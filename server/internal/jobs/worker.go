package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shreemangalam/stratum/server/internal/cache"
	"github.com/shreemangalam/stratum/server/internal/core"
	"github.com/shreemangalam/stratum/server/internal/parse"
	"github.com/shreemangalam/stratum/server/internal/store"
)

// SourceStore provides access to the submitted source texts.
// In v1 this is backed by an in-memory map; the source texts are
// ephemeral and not persisted beyond job completion.
type SourceStore struct {
	mu      sync.RWMutex
	sources map[string][]byte
}

// NewSourceStore creates a new source store.
func NewSourceStore() *SourceStore {
	return &SourceStore{sources: make(map[string][]byte)}
}

// Put stores source text by its content hash.
func (s *SourceStore) Put(hash string, source []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources[hash] = source
}

// Get retrieves source text by its content hash.
func (s *SourceStore) Get(hash string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src, ok := s.sources[hash]
	return src, ok
}

// Delete removes source text by its content hash.
func (s *SourceStore) Delete(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sources, hash)
}

// Size returns the number of stored sources.
func (s *SourceStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sources)
}

// Pool manages a bounded set of diff worker goroutines.
type Pool struct {
	store        store.Store
	cache        *cache.Cache
	registry     *parse.Registry
	sources      *SourceStore
	subscribers  *Subscribers
	workerCount  int
	pollInterval time.Duration
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewPool creates a worker pool.
func NewPool(
	workerCount int,
	s store.Store,
	c *cache.Cache,
	r *parse.Registry,
	sources *SourceStore,
	subs *Subscribers,
) *Pool {
	return &Pool{
		store:        s,
		cache:        c,
		registry:     r,
		sources:      sources,
		subscribers:  subs,
		workerCount:  workerCount,
		pollInterval: 500 * time.Millisecond,
	}
}

// Start launches worker goroutines. It first recovers any jobs that were
// left in "running" state by a previous crashed process.
func (p *Pool) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)

	recovered, err := p.store.RecoverStaleJobs(ctx, 2*time.Minute)
	if err != nil {
		slog.Error("recovering stale jobs", "error", err)
	} else if recovered > 0 {
		slog.Info("recovered stale jobs", "count", recovered)
	}

	for i := range p.workerCount {
		p.wg.Add(1)
		go func(id int) {
			defer p.wg.Done()
			p.runWorker(ctx, id)
		}(i)
	}

	slog.Info("worker pool started", "workers", p.workerCount)
}

// Stop signals all workers to stop and waits for them.
func (p *Pool) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	slog.Info("worker pool stopped")
}

func (p *Pool) runWorker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := p.store.ClaimPendingJob(ctx)
		if err != nil {
			slog.Error("claiming job", "worker", id, "error", err)
			time.Sleep(p.pollInterval)
			continue
		}

		if job == nil {
			time.Sleep(p.pollInterval)
			continue
		}

		slog.Info("processing job", "worker", id, "job_id", job.ID)
		p.subscribers.Send(job.ID, Event{Type: "status", Data: "running"})
		p.processJob(ctx, job)
		p.cleanupSources(job)
	}
}

func (p *Pool) processJob(ctx context.Context, job *store.Job) {
	if cached := p.cache.GetDiff(job.LeftHash, job.RightHash, job.Language); cached != nil {
		slog.Info("cache hit", "job_id", job.ID)
		if err := p.store.CompleteJob(ctx, job.ID, cached); err != nil {
			slog.Error("completing cached job", "job_id", job.ID, "error", err)
		}
		p.notifyComplete(job.ID, cached)
		return
	}

	leftSrc, ok := p.sources.Get(job.LeftHash)
	if !ok {
		p.failJob(ctx, job.ID, "left source not found")
		return
	}
	rightSrc, ok := p.sources.Get(job.RightHash)
	if !ok {
		p.failJob(ctx, job.ID, "right source not found")
		return
	}

	parser, err := p.registry.ForLanguage(job.Language)
	if err != nil {
		p.failJob(ctx, job.ID, fmt.Sprintf("parser not found: %v", err))
		return
	}

	p.subscribers.Send(job.ID, Event{Type: "progress", Data: "parsing left"})

	leftKey := cache.ParseKey(job.LeftHash, job.Language)
	leftTree := p.cache.GetParse(leftKey)
	if leftTree == nil {
		leftTree, err = parser.Parse(ctx, leftSrc)
		if err != nil {
			p.failJob(ctx, job.ID, fmt.Sprintf("parsing left: %v", err))
			return
		}
		p.cache.PutParse(leftKey, leftTree)
	}

	p.subscribers.Send(job.ID, Event{Type: "progress", Data: "parsing right"})

	rightKey := cache.ParseKey(job.RightHash, job.Language)
	rightTree := p.cache.GetParse(rightKey)
	if rightTree == nil {
		rightTree, err = parser.Parse(ctx, rightSrc)
		if err != nil {
			p.failJob(ctx, job.ID, fmt.Sprintf("parsing right: %v", err))
			return
		}
		p.cache.PutParse(rightKey, rightTree)
	}

	p.subscribers.Send(job.ID, Event{Type: "progress", Data: "matching"})

	matching := core.Match(leftTree, rightTree, core.DefaultMatchConfig())

	p.subscribers.Send(job.ID, Event{Type: "progress", Data: "generating edit script"})

	editScript := core.GenerateEditScript(leftTree, rightTree, matching)

	p.cache.PutDiff(job.LeftHash, job.RightHash, job.Language, editScript)

	if err := p.store.CompleteJob(ctx, job.ID, editScript); err != nil {
		slog.Error("completing job", "job_id", job.ID, "error", err)
		return
	}

	p.notifyComplete(job.ID, editScript)
	slog.Info("job completed", "job_id", job.ID, "operations", len(editScript.Operations))
}

func (p *Pool) cleanupSources(job *store.Job) {
	p.sources.Delete(job.LeftHash)
	p.sources.Delete(job.RightHash)
}

func (p *Pool) failJob(ctx context.Context, id, errMsg string) {
	slog.Error("job failed", "job_id", id, "error", errMsg)
	if err := p.store.FailJob(ctx, id, errMsg); err != nil {
		slog.Error("recording job failure", "job_id", id, "error", err)
	}
	// The error payload goes out before the terminal status: the SSE
	// handler closes the stream on a terminal status event.
	p.subscribers.Send(id, Event{Type: "error", Data: errMsg})
	p.subscribers.Send(id, Event{Type: "status", Data: "failed"})
}

func (p *Pool) notifyComplete(id string, es *core.EditScript) {
	// The result payload goes out before the terminal status: the SSE
	// handler closes the stream on a terminal status event.
	resultJSON, err := json.Marshal(es)
	if err == nil {
		p.subscribers.Send(id, Event{Type: "result", Data: string(resultJSON)})
	}
	p.subscribers.Send(id, Event{Type: "status", Data: "completed"})
}

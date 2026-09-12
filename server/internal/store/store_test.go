package store_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/shreemangalam/stratum/server/internal/core"
	"github.com/shreemangalam/stratum/server/internal/store"
)

func skipIfNoDocker(t *testing.T) {
	t.Helper()
	cmd := exec.Command("docker", "info")
	cmd.Env = append(cmd.Environ(), "DOCKER_HOST=npipe:////./pipe/docker_engine")
	if err := cmd.Run(); err != nil {
		t.Skip("Docker not available, skipping integration test")
	}
}

func setupDB(t *testing.T) *store.Postgres {
	t.Helper()
	skipIfNoDocker(t)
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("stratum_test"),
		postgres.WithUsername("stratum"),
		postgres.WithPassword("stratum"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("starting postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Errorf("terminating postgres container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := store.NewPostgres(connStr)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})

	if err := db.RunMigrations(ctx); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	return db
}

func mustCreateJob(
	ctx context.Context,
	t *testing.T,
	db *store.Postgres,
	leftHash, rightHash, leftSource, rightSource string,
) *store.Job {
	t.Helper()
	job, err := db.CreateJob(ctx, leftHash, rightHash, "go", leftSource, rightSource)
	if err != nil {
		t.Fatalf("creating job: %v", err)
	}
	return job
}

func mustGetJob(ctx context.Context, t *testing.T, db *store.Postgres, id string) *store.Job {
	t.Helper()
	job, err := db.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("getting job: %v", err)
	}
	if job == nil {
		t.Fatal("expected job, got nil")
	}
	return job
}

func mustExec(ctx context.Context, t *testing.T, db *store.Postgres, query string) {
	t.Helper()
	if _, err := db.Exec(ctx, query); err != nil {
		t.Fatalf("executing test SQL: %v", err)
	}
}

func mustJobStats(ctx context.Context, t *testing.T, db *store.Postgres) *store.JobStats {
	t.Helper()
	stats, err := db.JobStats(ctx)
	if err != nil {
		t.Fatalf("getting job stats: %v", err)
	}
	return stats
}

func TestPing(t *testing.T) {
	db := setupDB(t)
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestCreateJob(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	job, err := db.CreateJob(ctx, "left1", "right1", "go", "func a(){}", "func b(){}")
	if err != nil {
		t.Fatalf("creating job: %v", err)
	}

	if job.ID == "" {
		t.Error("expected non-empty ID")
	}
	if job.Status != store.StatusPending {
		t.Errorf("expected pending, got %s", job.Status)
	}
	if job.LeftHash != "left1" {
		t.Errorf("expected left_hash left1, got %s", job.LeftHash)
	}
	if job.Language != "go" {
		t.Errorf("expected language go, got %s", job.Language)
	}
	if job.LeftSource != "func a(){}" {
		t.Errorf("expected left source, got %q", job.LeftSource)
	}
	if job.RightSource != "func b(){}" {
		t.Errorf("expected right source, got %q", job.RightSource)
	}
}

func TestCreateJob_Idempotent(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	j1, err := db.CreateJob(ctx, "h1", "h2", "go", "a", "b")
	if err != nil {
		t.Fatal(err)
	}

	j2, err := db.CreateJob(ctx, "h1", "h2", "go", "a", "b")
	if err != nil {
		t.Fatal(err)
	}

	if j1.ID != j2.ID {
		t.Errorf("expected same ID for idempotent create, got %s and %s", j1.ID, j2.ID)
	}
}

func TestGetJob(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	created := mustCreateJob(ctx, t, db, "l", "r", "l", "r")

	got, err := db.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("getting job: %v", err)
	}
	if got == nil {
		t.Fatal("expected job, got nil")
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: %s vs %s", got.ID, created.ID)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	db := setupDB(t)
	got, err := db.GetJob(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent job")
	}
}

func TestFindJobByHashes(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	created := mustCreateJob(ctx, t, db, "findl", "findr", "", "")

	found, err := db.FindJobByHashes(ctx, "findl", "findr", "go")
	if err != nil {
		t.Fatalf("finding: %v", err)
	}
	if found == nil || found.ID != created.ID {
		t.Error("expected to find the created job")
	}

	notFound, err := db.FindJobByHashes(ctx, "findl", "findr", "python")
	if err != nil {
		t.Fatalf("finding: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for different language")
	}
}

func TestClaimPendingJob(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	_ = mustCreateJob(ctx, t, db, "cl", "cr", "", "")

	claimed, err := db.ClaimPendingJob(ctx)
	if err != nil {
		t.Fatalf("claiming: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected to claim a job")
	}
	if claimed.Status != store.StatusRunning {
		t.Errorf("expected running, got %s", claimed.Status)
	}

	second, err := db.ClaimPendingJob(ctx)
	if err != nil {
		t.Fatalf("claiming second: %v", err)
	}
	if second != nil {
		t.Error("expected no more pending jobs")
	}
}

func TestCompleteJob(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	created := mustCreateJob(ctx, t, db, "compl", "compr", "", "")

	es := &core.EditScript{
		Operations: []core.Operation{
			{Kind: core.OpInsert},
		},
		LeftRoot:  1,
		RightRoot: 1,
	}

	if err := db.CompleteJob(ctx, created.ID, es); err != nil {
		t.Fatalf("completing: %v", err)
	}

	got := mustGetJob(ctx, t, db, created.ID)
	if got.Status != store.StatusCompleted {
		t.Errorf("expected completed, got %s", got.Status)
	}
	if got.Result == nil {
		t.Fatal("expected result")
	}

	var parsed core.EditScript
	if err := json.Unmarshal(got.Result, &parsed); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if len(parsed.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(parsed.Operations))
	}
}

func TestFailJob(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	created := mustCreateJob(ctx, t, db, "fl", "fr", "", "")

	if err := db.FailJob(ctx, created.ID, "parse error"); err != nil {
		t.Fatalf("failing: %v", err)
	}

	got := mustGetJob(ctx, t, db, created.ID)
	if got.Status != store.StatusFailed {
		t.Errorf("expected failed, got %s", got.Status)
	}
	if got.Error != "parse error" {
		t.Errorf("expected error message, got %q", got.Error)
	}
}

func TestRecoverStaleJobs(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	mustExec(ctx, t, db, `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('stale1', 'stale2', 'go', 'running', now() - interval '10 minutes')
	`)

	recovered, err := db.RecoverStaleJobs(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("recovering: %v", err)
	}
	if recovered != 1 {
		t.Errorf("expected 1 recovered, got %d", recovered)
	}

	stats := mustJobStats(ctx, t, db)
	if stats.Running != 0 {
		t.Errorf("expected 0 running, got %d", stats.Running)
	}
	if stats.Pending != 1 {
		t.Errorf("expected 1 pending, got %d", stats.Pending)
	}
}

func TestRecoverStaleJobs_IgnoresRecent(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	_ = mustCreateJob(ctx, t, db, "recent_l", "recent_r", "", "")
	if _, err := db.ClaimPendingJob(ctx); err != nil {
		t.Fatalf("claiming pending job: %v", err)
	}

	recovered, err := db.RecoverStaleJobs(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("recovering: %v", err)
	}
	if recovered != 0 {
		t.Errorf("expected 0 recovered for recent job, got %d", recovered)
	}
}

func TestDeleteOldJobs(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	mustExec(ctx, t, db, `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('old1', 'old2', 'go', 'completed', now() - interval '8 days')
	`)
	mustExec(ctx, t, db, `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('old3', 'old4', 'go', 'failed', now() - interval '8 days')
	`)
	_ = mustCreateJob(ctx, t, db, "new1", "new2", "", "")

	deleted, err := db.DeleteOldJobs(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	stats := mustJobStats(ctx, t, db)
	total := stats.Pending + stats.Running + stats.Completed + stats.Failed
	if total != 1 {
		t.Errorf("expected 1 remaining job, got %d", total)
	}
}

func TestDeleteOldJobs_KeepsRunning(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	mustExec(ctx, t, db, `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('run1', 'run2', 'go', 'running', now() - interval '8 days')
	`)

	deleted, err := db.DeleteOldJobs(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (running jobs kept), got %d", deleted)
	}
}

func TestJobStats(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	_ = mustCreateJob(ctx, t, db, "s1", "s2", "", "")
	_ = mustCreateJob(ctx, t, db, "s3", "s4", "", "")
	mustExec(ctx, t, db, `
		INSERT INTO jobs (left_hash, right_hash, language, status)
		VALUES ('s5', 's6', 'go', 'completed')
	`)

	stats, err := db.JobStats(ctx)
	if err != nil {
		t.Fatalf("getting stats: %v", err)
	}

	if stats.Pending != 2 {
		t.Errorf("expected 2 pending, got %d", stats.Pending)
	}
	if stats.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", stats.Completed)
	}
}

func TestRequeueFailedJob(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	job := mustCreateJob(ctx, t, db, "rql", "rqr", "left src", "right src")

	claimed, err := db.ClaimPendingJob(ctx)
	if err != nil {
		t.Fatalf("claiming: %v", err)
	}
	if err := db.FailJob(ctx, claimed.ID, "parse error"); err != nil {
		t.Fatalf("failing: %v", err)
	}

	failed := mustGetJob(ctx, t, db, job.ID)
	if failed.Status != store.StatusFailed {
		t.Fatalf("expected failed, got %s", failed.Status)
	}

	requeued, err := db.RequeueFailedJob(ctx, job.ID, "new left", "new right")
	if err != nil {
		t.Fatalf("requeuing: %v", err)
	}
	if requeued == nil {
		t.Fatal("expected requeued job, got nil")
	}
	if requeued.Status != store.StatusPending {
		t.Errorf("expected pending, got %s", requeued.Status)
	}
	if requeued.Error != "" {
		t.Errorf("expected empty error, got %q", requeued.Error)
	}
	if requeued.LeftSource != "new left" {
		t.Errorf("expected refreshed left source")
	}
	if requeued.RightSource != "new right" {
		t.Errorf("expected refreshed right source")
	}
}

func TestRequeueFailedJob_NotFailed(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	job := mustCreateJob(ctx, t, db, "nfl", "nfr", "src", "src")

	requeued, err := db.RequeueFailedJob(ctx, job.ID, "src", "src")
	if err != nil {
		t.Fatalf("requeuing: %v", err)
	}
	if requeued != nil {
		t.Error("expected nil for non-failed job")
	}

	got := mustGetJob(ctx, t, db, job.ID)
	if got.Status != store.StatusPending {
		t.Errorf("job should still be pending, got %s", got.Status)
	}
}

func TestRequeueFailedJob_CompleteThenResubmit(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	job := mustCreateJob(ctx, t, db, "crl", "crr", "src", "src")
	claimed, _ := db.ClaimPendingJob(ctx)
	script := &core.EditScript{LeftRoot: 1, RightRoot: 2}
	if err := db.CompleteJob(ctx, claimed.ID, script); err != nil {
		t.Fatalf("completing: %v", err)
	}

	requeued, err := db.RequeueFailedJob(ctx, job.ID, "src", "src")
	if err != nil {
		t.Fatalf("requeuing completed: %v", err)
	}
	if requeued != nil {
		t.Error("should not requeue a completed job")
	}

	got := mustGetJob(ctx, t, db, job.ID)
	if got.Status != store.StatusCompleted {
		t.Errorf("completed job should stay completed, got %s", got.Status)
	}
}

func TestRequeueFailedJob_FullCycle(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	job := mustCreateJob(ctx, t, db, "fcl", "fcr", "left", "right")
	claimed, _ := db.ClaimPendingJob(ctx)
	_ = db.FailJob(ctx, claimed.ID, "transient error")

	_, err := db.RequeueFailedJob(ctx, job.ID, "left", "right")
	if err != nil {
		t.Fatalf("requeuing: %v", err)
	}

	reclaimed, err := db.ClaimPendingJob(ctx)
	if err != nil {
		t.Fatalf("reclaiming: %v", err)
	}
	if reclaimed == nil {
		t.Fatal("expected to reclaim requeued job")
	}
	if reclaimed.ID != job.ID {
		t.Errorf("expected same job ID %s, got %s", job.ID, reclaimed.ID)
	}

	script := &core.EditScript{LeftRoot: 1, RightRoot: 2}
	if err := db.CompleteJob(ctx, reclaimed.ID, script); err != nil {
		t.Fatalf("completing: %v", err)
	}

	final := mustGetJob(ctx, t, db, job.ID)
	if final.Status != store.StatusCompleted {
		t.Errorf("expected completed, got %s", final.Status)
	}
	if final.LeftSource == "" || final.RightSource == "" {
		t.Error("completed job should have sources")
	}

	var result core.EditScript
	if err := json.Unmarshal(final.Result, &result); err != nil {
		t.Fatalf("unmarshaling result: %v", err)
	}
}

func TestFindJobByHashes_ReturnsFailedJob(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	job := mustCreateJob(ctx, t, db, "fhl", "fhr", "src", "src")
	claimed, _ := db.ClaimPendingJob(ctx)
	_ = db.FailJob(ctx, claimed.ID, "bad input")

	found, err := db.FindJobByHashes(ctx, "fhl", "fhr", "go")
	if err != nil {
		t.Fatalf("finding: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find the failed job")
	}
	if found.ID != job.ID {
		t.Error("expected same job")
	}
	if found.Status != store.StatusFailed {
		t.Errorf("expected failed status, got %s", found.Status)
	}
}

func TestRequeueFailedJob_ConcurrentRetries(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	job := mustCreateJob(ctx, t, db, "ccl", "ccr", "src", "src")
	claimed, _ := db.ClaimPendingJob(ctx)
	_ = db.FailJob(ctx, claimed.ID, "crash")

	const goroutines = 5
	results := make([]*store.Job, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = db.RequeueFailedJob(ctx, job.ID, "src", "src")
		}(i)
	}
	wg.Wait()

	var successes int
	for i := range goroutines {
		if errs[i] != nil {
			t.Errorf("goroutine %d error: %v", i, errs[i])
		}
		if results[i] != nil {
			successes++
		}
	}

	if successes != 1 {
		t.Errorf("expected exactly 1 successful requeue, got %d", successes)
	}

	stats := mustJobStats(ctx, t, db)
	if stats.Pending != 1 {
		t.Errorf("expected 1 pending job after concurrent retries, got %d", stats.Pending)
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	if err := db.RunMigrations(ctx); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
}

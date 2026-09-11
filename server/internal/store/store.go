package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shreemangalam/stratum/server/internal/core"
)

// JobStatus represents the state of a diff job.
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
)

// Job represents a diff computation job.
type Job struct {
	ID          string          `json:"id"`
	Status      JobStatus       `json:"status"`
	LeftHash    string          `json:"left_hash"`
	RightHash   string          `json:"right_hash"`
	Language    string          `json:"language,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	LeftSource  string          `json:"left_source,omitempty"`
	RightSource string          `json:"right_source,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// JobStats holds aggregate job counts.
type JobStats struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// Store defines the persistence interface.
type Store interface {
	Ping(ctx context.Context) error
	CreateJob(ctx context.Context, leftHash, rightHash, language, leftSource, rightSource string) (*Job, error)
	GetJob(ctx context.Context, id string) (*Job, error)
	FindJobByHashes(ctx context.Context, leftHash, rightHash, language string) (*Job, error)
	ClaimPendingJob(ctx context.Context) (*Job, error)
	CompleteJob(ctx context.Context, id string, result *core.EditScript) error
	FailJob(ctx context.Context, id string, errMsg string) error
	RecoverStaleJobs(ctx context.Context, staleDuration time.Duration) (int, error)
	DeleteOldJobs(ctx context.Context, olderThan time.Duration) (int, error)
	JobStats(ctx context.Context) (*JobStats, error)
	RunMigrations(ctx context.Context) error
}

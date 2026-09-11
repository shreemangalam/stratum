package store_test

import (
	"context"
	"encoding/json"
	"os/exec"
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
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("starting postgres: %v", err)
	}
	t.Cleanup(func() { pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := store.NewPostgres(connStr)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.RunMigrations(ctx); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	return db
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

	created, _ := db.CreateJob(ctx, "l", "r", "go", "l", "r")

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

	created, _ := db.CreateJob(ctx, "findl", "findr", "go", "", "")

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

	db.CreateJob(ctx, "cl", "cr", "go", "", "")

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

	created, _ := db.CreateJob(ctx, "compl", "compr", "go", "", "")

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

	got, _ := db.GetJob(ctx, created.ID)
	if got.Status != store.StatusCompleted {
		t.Errorf("expected completed, got %s", got.Status)
	}
	if got.Result == nil {
		t.Fatal("expected result")
	}

	var parsed core.EditScript
	json.Unmarshal(got.Result, &parsed)
	if len(parsed.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(parsed.Operations))
	}
}

func TestFailJob(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	created, _ := db.CreateJob(ctx, "fl", "fr", "go", "", "")

	if err := db.FailJob(ctx, created.ID, "parse error"); err != nil {
		t.Fatalf("failing: %v", err)
	}

	got, _ := db.GetJob(ctx, created.ID)
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

	db.Exec(ctx, `
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

	stats, _ := db.JobStats(ctx)
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

	db.CreateJob(ctx, "recent_l", "recent_r", "go", "", "")
	db.ClaimPendingJob(ctx)

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

	db.Exec(ctx, `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('old1', 'old2', 'go', 'completed', now() - interval '8 days')
	`)
	db.Exec(ctx, `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('old3', 'old4', 'go', 'failed', now() - interval '8 days')
	`)
	db.CreateJob(ctx, "new1", "new2", "go", "", "")

	deleted, err := db.DeleteOldJobs(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	stats, _ := db.JobStats(ctx)
	total := stats.Pending + stats.Running + stats.Completed + stats.Failed
	if total != 1 {
		t.Errorf("expected 1 remaining job, got %d", total)
	}
}

func TestDeleteOldJobs_KeepsRunning(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	db.Exec(ctx, `
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

	db.CreateJob(ctx, "s1", "s2", "go", "", "")
	db.CreateJob(ctx, "s3", "s4", "go", "", "")
	db.Exec(ctx, `
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

func TestRunMigrations_Idempotent(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	if err := db.RunMigrations(ctx); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
}

package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/shreemangalam/stratum/server/internal/core"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Postgres implements Store using PostgreSQL.
type Postgres struct {
	db *sql.DB
}

// NewPostgres creates a new Postgres store.
func NewPostgres(databaseURL string) (*Postgres, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &Postgres{db: db}, nil
}

// Close closes the database connection.
func (p *Postgres) Close() error {
	return p.db.Close()
}

func (p *Postgres) RunMigrations(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		var exists bool
		err := p.db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)",
			entry.Name(),
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration %s: %w", entry.Name(), err)
		}
		if exists {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", entry.Name(), err)
		}

		tx, err := p.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning tx for %s: %w", entry.Name(), err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("executing migration %s: %w", entry.Name(), err)
		}

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)",
			entry.Name(),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", entry.Name(), err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func scanJob(row interface{ Scan(dest ...any) error }) (*Job, error) {
	job := &Job{}
	var result *json.RawMessage
	var errStr, leftSrc, rightSrc sql.NullString
	err := row.Scan(
		&job.ID, &job.Status, &job.LeftHash, &job.RightHash,
		&job.Language, &result, &errStr,
		&leftSrc, &rightSrc,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if result != nil {
		job.Result = *result
	}
	if errStr.Valid {
		job.Error = errStr.String
	}
	if leftSrc.Valid {
		job.LeftSource = leftSrc.String
	}
	if rightSrc.Valid {
		job.RightSource = rightSrc.String
	}
	return job, nil
}

func (p *Postgres) CreateJob(ctx context.Context, leftHash, rightHash, language, leftSource, rightSource string) (*Job, error) {
	job, err := scanJob(p.db.QueryRowContext(ctx, `
		INSERT INTO jobs (left_hash, right_hash, language, left_source, right_source)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (left_hash, right_hash, language) DO UPDATE SET updated_at = now()
		RETURNING id, status, left_hash, right_hash, language, result, error, left_source, right_source, created_at, updated_at
	`, leftHash, rightHash, language, leftSource, rightSource))
	if err != nil {
		return nil, fmt.Errorf("creating job: %w", err)
	}
	return job, nil
}

func (p *Postgres) GetJob(ctx context.Context, id string) (*Job, error) {
	job, err := scanJob(p.db.QueryRowContext(ctx, `
		SELECT id, status, left_hash, right_hash, language, result, error, left_source, right_source, created_at, updated_at
		FROM jobs WHERE id = $1
	`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting job: %w", err)
	}
	return job, nil
}

func (p *Postgres) FindJobByHashes(ctx context.Context, leftHash, rightHash, language string) (*Job, error) {
	job, err := scanJob(p.db.QueryRowContext(ctx, `
		SELECT id, status, left_hash, right_hash, language, result, error, left_source, right_source, created_at, updated_at
		FROM jobs WHERE left_hash = $1 AND right_hash = $2 AND language = $3
	`, leftHash, rightHash, language))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding job by hashes: %w", err)
	}
	return job, nil
}

func (p *Postgres) ClaimPendingJob(ctx context.Context) (*Job, error) {
	job, err := scanJob(p.db.QueryRowContext(ctx, `
		UPDATE jobs
		SET status = 'running', updated_at = now()
		WHERE id = (
			SELECT id FROM jobs
			WHERE status = 'pending'
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, status, left_hash, right_hash, language, result, error, left_source, right_source, created_at, updated_at
	`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claiming job: %w", err)
	}
	return job, nil
}

func (p *Postgres) CompleteJob(ctx context.Context, id string, result *core.EditScript) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}

	_, err = p.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'completed', result = $2, updated_at = now()
		WHERE id = $1
	`, id, resultJSON)
	if err != nil {
		return fmt.Errorf("completing job: %w", err)
	}
	return nil
}

func (p *Postgres) FailJob(ctx context.Context, id string, errMsg string) error {
	_, err := p.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'failed', error = $2, updated_at = now()
		WHERE id = $1
	`, id, errMsg)
	if err != nil {
		return fmt.Errorf("failing job: %w", err)
	}
	return nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func (p *Postgres) RecoverStaleJobs(ctx context.Context, staleDuration time.Duration) (int, error) {
	result, err := p.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'pending', updated_at = now()
		WHERE status = 'running' AND updated_at < now() - $1::interval
	`, fmt.Sprintf("%d seconds", int(staleDuration.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("recovering stale jobs: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (p *Postgres) DeleteOldJobs(ctx context.Context, olderThan time.Duration) (int, error) {
	result, err := p.db.ExecContext(ctx, `
		DELETE FROM jobs
		WHERE status IN ('completed', 'failed')
		  AND updated_at < now() - $1::interval
	`, fmt.Sprintf("%d seconds", int(olderThan.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("deleting old jobs: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// Exec runs a raw SQL statement. Intended for test setup only.
func (p *Postgres) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}

func (p *Postgres) JobStats(ctx context.Context) (*JobStats, error) {
	stats := &JobStats{}
	err := p.db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'pending'),
			count(*) FILTER (WHERE status = 'running'),
			count(*) FILTER (WHERE status = 'completed'),
			count(*) FILTER (WHERE status = 'failed')
		FROM jobs
	`).Scan(&stats.Pending, &stats.Running, &stats.Completed, &stats.Failed)
	if err != nil {
		return nil, fmt.Errorf("getting job stats: %w", err)
	}
	return stats, nil
}

package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/shreemangalam/stratum/server/internal/cache"
	stratumhttp "github.com/shreemangalam/stratum/server/internal/http"
	"github.com/shreemangalam/stratum/server/internal/jobs"
	"github.com/shreemangalam/stratum/server/internal/parse"
	"github.com/shreemangalam/stratum/server/internal/parse/treesitter"
	"github.com/shreemangalam/stratum/server/internal/parse/xslt"
	"github.com/shreemangalam/stratum/server/internal/store"
)

type testEnv struct {
	server *httptest.Server
	db     *store.Postgres
	pool   *jobs.Pool
	cancel context.CancelFunc
}

func (e *testEnv) close(t *testing.T) {
	t.Helper()
	e.server.Close()
	e.pool.Stop()
	e.cancel()
	if err := e.db.Close(); err != nil {
		t.Errorf("closing test database: %v", err)
	}
}

func closeBody(t *testing.T, body io.Closer) {
	t.Helper()
	if err := body.Close(); err != nil {
		t.Errorf("closing response body: %v", err)
	}
}

func decodeJSON(t *testing.T, r io.Reader, target any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(target); err != nil {
		t.Fatalf("decoding JSON response: %v", err)
	}
}

func unmarshalJSON(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decoding JSON result: %v", err)
	}
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available, skipping integration test")
	}
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
		t.Fatalf("starting postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Errorf("terminating postgres container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

	db, err := store.NewPostgres(connStr)
	if err != nil {
		t.Fatalf("connecting to postgres: %v", err)
	}

	if err := db.RunMigrations(ctx); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	appCache := cache.New(time.Hour)
	registry := parse.NewRegistry()
	registry.Register(treesitter.NewGo())
	registry.Register(xslt.New())
	for _, lang := range treesitter.SupportedLanguages() {
		if lang.Lang == "go" {
			continue
		}
		registry.Register(treesitter.NewGeneric(lang.Lang, lang.Exts))
	}

	sources := jobs.NewSourceStore()
	subscribers := jobs.NewSubscribers()

	workerCtx, cancel := context.WithCancel(ctx)
	pool := jobs.NewPool(2, db, appCache, registry, sources, subscribers)
	pool.Start(workerCtx)

	srv := stratumhttp.NewServer(db, appCache, registry, sources, subscribers, stratumhttp.Config{
		GitEnabled: true,
	})
	ts := httptest.NewServer(srv.Handler())

	return &testEnv{server: ts, db: db, pool: pool, cancel: cancel}
}

func TestHealthEndpoint(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	resp, err := http.Get(env.server.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	decodeJSON(t, resp.Body, &body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
	if body["database"] != "connected" {
		t.Errorf("expected database connected, got %q", body["database"])
	}
}

func TestLanguagesEndpoint(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	resp, err := http.Get(env.server.URL + "/api/v1/languages")
	if err != nil {
		t.Fatalf("languages request: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Languages []struct {
			ID string `json:"id"`
		} `json:"languages"`
	}
	decodeJSON(t, resp.Body, &body)

	ids := make(map[string]bool)
	for _, l := range body.Languages {
		ids[l.ID] = true
	}
	if !ids["go"] {
		t.Error("expected go in languages")
	}
	if !ids["xslt"] {
		t.Error("expected xslt in languages")
	}
}

func TestCreateDiff_GoParsesAndCompletes(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "package main\n\nfunc hello() {\n\treturn \"hello\"\n}\n"},
		"right": {"content": "package main\n\nfunc hello() {\n\treturn \"world\"\n}\n"},
		"language": "go"
	}`

	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 201 or 200, got %d", resp.StatusCode)
	}

	var job struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Language string `json:"language"`
	}
	decodeJSON(t, resp.Body, &job)

	if job.ID == "" {
		t.Fatal("expected job ID")
	}
	if job.Language != "go" {
		t.Errorf("expected language go, got %q", job.Language)
	}

	var result struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		if err != nil {
			t.Fatalf("polling diff: %v", err)
		}
		decodeJSON(t, pollResp.Body, &result)
		closeBody(t, pollResp.Body)

		if result.Status == "completed" || result.Status == "failed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if result.Status != "completed" {
		t.Fatalf("expected completed, got %q", result.Status)
	}
	if result.Result == nil {
		t.Fatal("expected result, got nil")
	}

	var editScript struct {
		Operations []struct {
			Kind string `json:"kind"`
		} `json:"operations"`
	}
	unmarshalJSON(t, result.Result, &editScript)

	if len(editScript.Operations) == 0 {
		t.Error("expected at least one operation")
	}
}

func TestCreateDiff_XSLTParsesAndCompletes(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "<?xml version=\"1.0\"?><xsl:stylesheet version=\"2.0\" xmlns:xsl=\"http://www.w3.org/1999/XSL/Transform\"><xsl:template match=\"/\"><out/></xsl:template></xsl:stylesheet>"},
		"right": {"content": "<?xml version=\"1.0\"?><xsl:stylesheet version=\"2.0\" xmlns:xsl=\"http://www.w3.org/1999/XSL/Transform\"><xsl:template match=\"/\"><result/></xsl:template></xsl:stylesheet>"},
		"language": "xslt"
	}`

	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 201 or 200, got %d", resp.StatusCode)
	}

	var job struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp.Body, &job)

	var result struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		if err != nil {
			t.Fatalf("polling diff: %v", err)
		}
		decodeJSON(t, pollResp.Body, &result)
		closeBody(t, pollResp.Body)

		if result.Status == "completed" || result.Status == "failed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if result.Status != "completed" {
		t.Fatalf("expected completed, got %q", result.Status)
	}

	var editScript struct {
		Operations []struct {
			Kind string `json:"kind"`
		} `json:"operations"`
	}
	unmarshalJSON(t, result.Result, &editScript)

	if len(editScript.Operations) == 0 {
		t.Error("expected at least one operation for XSLT structural diff")
	}
}

func TestCreateDiff_Idempotent(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "package main\n\nfunc a() {}"},
		"right": {"content": "package main\n\nfunc b() {}"},
		"language": "go"
	}`

	resp1, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	var job1 struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp1.Body, &job1)
	closeBody(t, resp1.Body)

	resp2, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	var job2 struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp2.Body, &job2)
	closeBody(t, resp2.Body)

	if job1.ID != job2.ID {
		t.Errorf("expected same job ID for idempotent request, got %q and %q", job1.ID, job2.ID)
	}
}

func TestCreateDiff_LanguageDifferentiates(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	content := `{"left":{"content":"<root>a</root>"},"right":{"content":"<root>b</root>"},"language":"%s"}`

	resp1, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json",
		strings.NewReader(fmt.Sprintf(content, "go")))
	if err != nil {
		t.Fatalf("go request: %v", err)
	}
	var job1 struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp1.Body, &job1)
	closeBody(t, resp1.Body)

	resp2, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json",
		strings.NewReader(fmt.Sprintf(content, "xslt")))
	if err != nil {
		t.Fatalf("xslt request: %v", err)
	}
	var job2 struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp2.Body, &job2)
	closeBody(t, resp2.Body)

	if job1.ID == job2.ID {
		t.Error("same content with different language should produce different jobs")
	}
}

func TestCreateDiff_ValidationErrors(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	tests := []struct {
		name    string
		payload string
		status  int
	}{
		{"empty body", `{}`, http.StatusBadRequest},
		{"missing right", `{"left":{"content":"x"}}`, http.StatusBadRequest},
		{"missing left", `{"right":{"content":"x"}}`, http.StatusBadRequest},
		{"invalid language", `{"left":{"content":"x"},"right":{"content":"y"},"language":"brainfuck"}`, http.StatusBadRequest},
		{"invalid json", `not json`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json",
				strings.NewReader(tt.payload))
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			closeBody(t, resp.Body)
			if resp.StatusCode != tt.status {
				t.Errorf("expected %d, got %d", tt.status, resp.StatusCode)
			}
		})
	}
}

func TestGetDiff_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	resp, err := http.Get(env.server.URL + "/api/v1/diffs/nonexistent-id")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRequestID_Propagated(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	req, _ := http.NewRequest("GET", env.server.URL+"/api/v1/health", nil)
	req.Header.Set("X-Request-ID", "test-123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	closeBody(t, resp.Body)

	if got := resp.Header.Get("X-Request-ID"); got != "test-123" {
		t.Errorf("expected X-Request-ID test-123, got %q", got)
	}
}

func TestRequestID_Generated(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	resp, err := http.Get(env.server.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	closeBody(t, resp.Body)

	if got := resp.Header.Get("X-Request-ID"); got == "" {
		t.Error("expected generated X-Request-ID")
	}
}

func TestStreamDiff_CompletedJob(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "package main\n\nfunc hello() {\n\treturn \"hello\"\n}\n"},
		"right": {"content": "package main\n\nfunc hello() {\n\treturn \"world\"\n}\n"},
		"language": "go"
	}`

	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	var job struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp.Body, &job)
	closeBody(t, resp.Body)

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		if err != nil {
			t.Fatalf("polling: %v", err)
		}
		var s struct {
			Status string `json:"status"`
		}
		decodeJSON(t, pollResp.Body, &s)
		closeBody(t, pollResp.Body)
		if s.Status == "completed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	sseResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s/stream", env.server.URL, job.ID))
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer closeBody(t, sseResp.Body)

	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", sseResp.StatusCode)
	}

	ct := sseResp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}

	buf := make([]byte, 64*1024)
	n, _ := sseResp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "event: status") {
		t.Error("expected status event in SSE response")
	}
	if !strings.Contains(body, "event: result") {
		t.Error("expected result event in SSE response")
	}
	if !strings.Contains(body, "\"operations\"") {
		t.Error("expected operations in result data")
	}
}

func TestStreamDiff_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	resp, err := http.Get(env.server.URL + "/api/v1/diffs/00000000-0000-0000-0000-000000000000/stream")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestStatsEndpoint(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "package main\n\nfunc a() {}"},
		"right": {"content": "package main\n\nfunc b() {}"},
		"language": "go"
	}`
	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	closeBody(t, resp.Body)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		statsResp, err := http.Get(env.server.URL + "/api/v1/stats")
		if err != nil {
			t.Fatalf("stats request: %v", err)
		}
		var body struct {
			Jobs struct {
				Completed int `json:"completed"`
			} `json:"jobs"`
			CacheSize int `json:"cache_size"`
		}
		decodeJSON(t, statsResp.Body, &body)
		closeBody(t, statsResp.Body)

		if body.Jobs.Completed >= 1 {
			if body.CacheSize == 0 {
				t.Error("expected cache_size > 0 after completed job")
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("timed out waiting for job to complete")
}

func TestStaleJobRecovery(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	_, err := env.db.Exec(context.Background(), `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('stale_left', 'stale_right', 'go', 'running', now() - interval '10 minutes')
	`)
	if err != nil {
		t.Fatalf("inserting stale job: %v", err)
	}

	recovered, err := env.db.RecoverStaleJobs(context.Background(), 2*time.Minute)
	if err != nil {
		t.Fatalf("recovering stale jobs: %v", err)
	}
	if recovered != 1 {
		t.Errorf("expected 1 recovered job, got %d", recovered)
	}

	stats, err := env.db.JobStats(context.Background())
	if err != nil {
		t.Fatalf("getting stats: %v", err)
	}
	if stats.Running != 0 {
		t.Errorf("expected 0 running jobs after recovery, got %d", stats.Running)
	}
}

func TestRecoveredJob_CompletesFromPersistedSource(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "package main\n\nfunc before() {\n\treturn 1\n}\n"},
		"right": {"content": "package main\n\nfunc after() {\n\treturn 2\n}\n"},
		"language": "go"
	}`

	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	var job struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp.Body, &job)
	closeBody(t, resp.Body)

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		if err != nil {
			t.Fatalf("polling: %v", err)
		}
		var s struct {
			Status string `json:"status"`
		}
		decodeJSON(t, pollResp.Body, &s)
		closeBody(t, pollResp.Body)
		if s.Status == "completed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	env.pool.Stop()

	_, err = env.db.Exec(context.Background(),
		"UPDATE jobs SET status = 'pending', result = NULL, updated_at = now() WHERE id = $1", job.ID)
	if err != nil {
		t.Fatalf("resetting job to pending: %v", err)
	}

	freshSources := jobs.NewSourceStore()
	freshCache := cache.New(time.Hour)

	registry := parse.NewRegistry()
	registry.Register(treesitter.NewGo())

	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	freshPool := jobs.NewPool(1, env.db, freshCache, registry, freshSources, jobs.NewSubscribers())
	freshPool.Start(workerCtx)
	defer freshPool.Stop()

	var result struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		if err != nil {
			t.Fatalf("polling: %v", err)
		}
		decodeJSON(t, pollResp.Body, &result)
		closeBody(t, pollResp.Body)
		if result.Status == "completed" || result.Status == "failed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if result.Status != "completed" {
		t.Fatalf("recovered job should complete, got status=%q error=%q", result.Status, result.Error)
	}
	if result.Result == nil {
		t.Fatal("expected non-nil result")
	}

	var es struct {
		Operations []struct {
			Kind string `json:"kind"`
		} `json:"operations"`
	}
	unmarshalJSON(t, result.Result, &es)
	if len(es.Operations) == 0 {
		t.Error("expected operations from recovered job")
	}
}

func TestCreateDiff_JavaScriptParsesAndCompletes(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "function hello() {\n  return 'hello';\n}\n"},
		"right": {"content": "function world() {\n  return 'world';\n}\n"},
		"language": "javascript"
	}`

	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 201 or 200, got %d", resp.StatusCode)
	}

	var job struct {
		ID       string `json:"id"`
		Language string `json:"language"`
	}
	decodeJSON(t, resp.Body, &job)
	if job.Language != "javascript" {
		t.Errorf("expected javascript, got %q", job.Language)
	}

	var result struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		if err != nil {
			t.Fatalf("polling: %v", err)
		}
		decodeJSON(t, pollResp.Body, &result)
		closeBody(t, pollResp.Body)
		if result.Status == "completed" || result.Status == "failed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if result.Status != "completed" {
		t.Fatalf("expected completed, got %q", result.Status)
	}

	var es struct {
		Operations []struct {
			Kind string `json:"kind"`
		} `json:"operations"`
	}
	unmarshalJSON(t, result.Result, &es)
	if len(es.Operations) == 0 {
		t.Error("expected operations for JS rename")
	}
}

func TestCreateDiff_PythonParsesAndCompletes(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "def greet(name):\n    return f'Hello, {name}'\n"},
		"right": {"content": "def greet(name):\n    return f'Hi, {name}'\n"},
		"language": "python"
	}`

	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 201/200, got %d", resp.StatusCode)
	}

	var job struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp.Body, &job)

	var result struct {
		Status string `json:"status"`
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, _ := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		decodeJSON(t, pollResp.Body, &result)
		closeBody(t, pollResp.Body)
		if result.Status == "completed" || result.Status == "failed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed, got %q", result.Status)
	}
}

func TestCreateDiff_JavaParsesAndCompletes(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	payload := `{
		"left": {"content": "package com.example;\n\npublic class Main {\n    public void run() {}\n}\n"},
		"right": {"content": "package com.example;\n\npublic class Main {\n    public void execute() {}\n}\n"},
		"language": "java"
	}`

	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 201/200, got %d", resp.StatusCode)
	}

	var job struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp.Body, &job)

	var result struct {
		Status string `json:"status"`
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, _ := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		decodeJSON(t, pollResp.Body, &result)
		closeBody(t, pollResp.Body)
		if result.Status == "completed" || result.Status == "failed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed, got %q", result.Status)
	}
}

func TestGetDiff_IncludesSource(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	leftContent := "package main\n\nfunc original() {}\n"
	rightContent := "package main\n\nfunc modified() {}\n"

	payload := fmt.Sprintf(`{
		"left": {"content": %q},
		"right": {"content": %q},
		"language": "go"
	}`, leftContent, rightContent)

	resp, err := http.Post(env.server.URL+"/api/v1/diffs", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	var job struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp.Body, &job)
	closeBody(t, resp.Body)

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, _ := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
		var s struct {
			Status string `json:"status"`
		}
		decodeJSON(t, pollResp.Body, &s)
		closeBody(t, pollResp.Body)
		if s.Status == "completed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	getResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", env.server.URL, job.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, getResp.Body)

	var result struct {
		LeftSource  string `json:"left_source"`
		RightSource string `json:"right_source"`
		Language    string `json:"language"`
	}
	decodeJSON(t, getResp.Body, &result)

	if result.LeftSource != leftContent {
		t.Errorf("left_source mismatch:\ngot:  %q\nwant: %q", result.LeftSource, leftContent)
	}
	if result.RightSource != rightContent {
		t.Errorf("right_source mismatch:\ngot:  %q\nwant: %q", result.RightSource, rightContent)
	}
	if result.Language != "go" {
		t.Errorf("expected language go, got %q", result.Language)
	}
}

func TestDeleteOldJobs(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	_, err := env.db.Exec(context.Background(), `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('old1', 'old2', 'go', 'completed', now() - interval '8 days')
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.db.Exec(context.Background(), `
		INSERT INTO jobs (left_hash, right_hash, language, status, updated_at)
		VALUES ('old3', 'old4', 'go', 'failed', now() - interval '8 days')
	`)
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := env.db.DeleteOldJobs(context.Background(), 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}
}

func TestCORS_Preflight(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	req, _ := http.NewRequest("OPTIONS", env.server.URL+"/api/v1/diffs", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected CORS header *, got %q", got)
	}
}

// --- Merge endpoint tests ---

func TestCreateMerge_AutoResolved(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	base := "package main\n\nfunc greet() string {\n\treturn \"hello\"\n}\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n"
	left := "package main\n\nfunc greet() string {\n\treturn \"hi\"\n}\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n"
	right := "package main\n\nfunc greet() string {\n\treturn \"hello\"\n}\n\nfunc add(a, b int) int {\n\treturn a + b + 1\n}\n"

	payload := fmt.Sprintf(`{
		"base": {"content": %q},
		"left": {"content": %q},
		"right": {"content": %q},
		"language": "go"
	}`, base, left, right)

	resp, err := http.Post(env.server.URL+"/api/v1/merges", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create merge: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Language string `json:"language"`
		Plan     struct {
			Entries       []json.RawMessage `json:"entries"`
			ConflictCount int               `json:"conflict_count"`
			HasConflicts  bool              `json:"has_conflicts"`
		} `json:"plan"`
	}
	decodeJSON(t, resp.Body, &result)

	if result.Language != "go" {
		t.Errorf("expected language go, got %q", result.Language)
	}
	if result.Plan.HasConflicts {
		t.Error("expected no conflicts for non-overlapping changes")
	}
	if len(result.Plan.Entries) == 0 {
		t.Error("expected merge entries")
	}
}

func TestCreateMerge_WithConflict(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	base := "package main\n\nfunc greet() string {\n\treturn \"hello\"\n}\n"
	left := "package main\n\nfunc greet() string {\n\treturn \"hi\"\n}\n"
	right := "package main\n\nfunc greet() string {\n\treturn \"hey\"\n}\n"

	payload := fmt.Sprintf(`{
		"base": {"content": %q},
		"left": {"content": %q},
		"right": {"content": %q},
		"language": "go"
	}`, base, left, right)

	resp, err := http.Post(env.server.URL+"/api/v1/merges", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create merge: %v", err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Plan struct {
			Entries []struct {
				Decision     string  `json:"decision"`
				ConflictKind *string `json:"conflict_kind"`
			} `json:"entries"`
			ConflictCount int  `json:"conflict_count"`
			HasConflicts  bool `json:"has_conflicts"`
		} `json:"plan"`
	}
	decodeJSON(t, resp.Body, &result)

	if !result.Plan.HasConflicts {
		t.Error("expected conflicts for modify-modify")
	}
	if result.Plan.ConflictCount < 1 {
		t.Errorf("expected at least 1 conflict, got %d", result.Plan.ConflictCount)
	}

	hasConflictEntry := false
	for _, e := range result.Plan.Entries {
		if e.Decision == "conflict" && e.ConflictKind != nil && *e.ConflictKind == "modify-modify" {
			hasConflictEntry = true
		}
	}
	if !hasConflictEntry {
		t.Error("expected a modify-modify conflict entry")
	}
}

func TestCreateMerge_DeleteModifyConflict(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	base := "package main\n\nfunc removed() int {\n\treturn 1\n}\n\nfunc kept() int {\n\treturn 2\n}\n"
	left := "package main\n\nfunc kept() int {\n\treturn 2\n}\n"
	right := "package main\n\nfunc removed() int {\n\treturn 99\n}\n\nfunc kept() int {\n\treturn 2\n}\n"

	payload := fmt.Sprintf(`{
		"base": {"content": %q},
		"left": {"content": %q},
		"right": {"content": %q},
		"language": "go"
	}`, base, left, right)

	resp, err := http.Post(env.server.URL+"/api/v1/merges", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Plan struct {
			Entries []struct {
				Decision     string  `json:"decision"`
				ConflictKind *string `json:"conflict_kind"`
			} `json:"entries"`
			HasConflicts bool `json:"has_conflicts"`
		} `json:"plan"`
	}
	decodeJSON(t, resp.Body, &result)

	if !result.Plan.HasConflicts {
		t.Error("expected delete-modify conflict")
	}

	hasDeleteModify := false
	for _, e := range result.Plan.Entries {
		if e.ConflictKind != nil && *e.ConflictKind == "delete-modify" {
			hasDeleteModify = true
		}
	}
	if !hasDeleteModify {
		t.Error("expected a delete-modify conflict entry")
	}
}

func TestCreateMerge_IncludesSources(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	base := "package main\n\nfunc f() {}\n"
	left := "package main\n\nfunc g() {}\n"
	right := "package main\n\nfunc h() {}\n"

	payload := fmt.Sprintf(`{
		"base": {"content": %q},
		"left": {"content": %q},
		"right": {"content": %q},
		"language": "go"
	}`, base, left, right)

	resp, err := http.Post(env.server.URL+"/api/v1/merges", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, resp.Body)

	var result struct {
		BaseSource  *string `json:"base_source"`
		LeftSource  *string `json:"left_source"`
		RightSource *string `json:"right_source"`
	}
	decodeJSON(t, resp.Body, &result)

	if result.BaseSource == nil || *result.BaseSource != base {
		t.Error("expected base_source in response")
	}
	if result.LeftSource == nil || *result.LeftSource != left {
		t.Error("expected left_source in response")
	}
	if result.RightSource == nil || *result.RightSource != right {
		t.Error("expected right_source in response")
	}
}

func TestCreateMerge_ValidationErrors(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	tests := []struct {
		name    string
		payload string
	}{
		{"missing base", `{"left":{"content":"x"},"right":{"content":"y"},"language":"go"}`},
		{"missing left", `{"base":{"content":"x"},"right":{"content":"y"},"language":"go"}`},
		{"missing right", `{"base":{"content":"x"},"left":{"content":"y"},"language":"go"}`},
		{"invalid json", `not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Post(env.server.URL+"/api/v1/merges", "application/json",
				strings.NewReader(tt.payload))
			if err != nil {
				t.Fatal(err)
			}
			closeBody(t, resp.Body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

// --- Changeset endpoint tests ---

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.name", "test")
	run("config", "user.email", "test@test.com")
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "-A")
	run("commit", "-m", msg)
}

func TestCreateChangeset_CrossFileMove(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	dir := initGitRepo(t)

	writeFile(t, dir, "utils.go", "package main\n\nfunc helper(x int) int {\n\ty := x * 2\n\tz := y + 1\n\treturn z\n}\n\nfunc other() {}\n")
	gitCommit(t, dir, "initial")

	writeFile(t, dir, "utils.go", "package main\n\nfunc other() {}\n")
	writeFile(t, dir, "helpers.go", "package main\n\nfunc helper(x int) int {\n\ty := x * 2\n\tz := y + 1\n\treturn z\n}\n")
	gitCommit(t, dir, "move helper to helpers.go")

	payload := fmt.Sprintf(`{
		"repo_path": %q,
		"left_ref": "HEAD~1",
		"right_ref": "HEAD"
	}`, dir)

	resp, err := http.Post(env.server.URL+"/api/v1/changesets", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Files []struct {
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"files"`
		CrossFileMatches []struct {
			Kind       string  `json:"kind"`
			Score      float64 `json:"score"`
			SourceFile string  `json:"source_file"`
			TargetFile string  `json:"target_file"`
			SourceNode struct {
				Label string `json:"label"`
			} `json:"source_node"`
			TargetNode struct {
				Label string `json:"label"`
			} `json:"target_node"`
		} `json:"cross_file_matches"`
	}
	decodeJSON(t, resp.Body, &result)

	if len(result.Files) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(result.Files))
	}

	if len(result.CrossFileMatches) != 1 {
		t.Fatalf("expected 1 cross-file match, got %d", len(result.CrossFileMatches))
	}

	m := result.CrossFileMatches[0]
	if m.Kind != "move" {
		t.Errorf("expected move, got %q", m.Kind)
	}
	if m.Score != 1.0 {
		t.Errorf("expected exact match score 1.0, got %f", m.Score)
	}
	if m.SourceNode.Label != "helper" || m.TargetNode.Label != "helper" {
		t.Errorf("expected helper -> helper, got %q -> %q", m.SourceNode.Label, m.TargetNode.Label)
	}
}

func TestCreateChangeset_RenameMove(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	dir := initGitRepo(t)

	writeFile(t, dir, "old.go", "package main\n\nfunc oldName(a, b int) int {\n\tx := a * 2\n\ty := b * 3\n\tresult := x + y\n\tif result < 0 {\n\t\treturn 0\n\t}\n\treturn result\n}\n\nfunc unrelatedOld() string {\n\treturn \"this function exists only in old.go\"\n}\n")
	gitCommit(t, dir, "initial")

	if err := os.Remove(filepath.Join(dir, "old.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "new.go", "package main\n\nfunc newName(a, b int) int {\n\tx := a * 2\n\ty := b * 3\n\tresult := x + y\n\tif result < 0 {\n\t\treturn 0\n\t}\n\treturn result\n}\n\nfunc unrelatedNew() string {\n\treturn \"this function exists only in new.go and is completely different\"\n}\n")
	gitCommit(t, dir, "delete old, add new with renamed func")

	payload := fmt.Sprintf(`{
		"repo_path": %q,
		"left_ref": "HEAD~1",
		"right_ref": "HEAD"
	}`, dir)

	resp, err := http.Post(env.server.URL+"/api/v1/changesets", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		CrossFileMatches []struct {
			Kind       string `json:"kind"`
			SourceNode struct {
				Label string `json:"label"`
			} `json:"source_node"`
			TargetNode struct {
				Label string `json:"label"`
			} `json:"target_node"`
		} `json:"cross_file_matches"`
	}
	decodeJSON(t, resp.Body, &result)

	hasRenameMove := false
	for _, m := range result.CrossFileMatches {
		if m.Kind == "rename-move" && m.SourceNode.Label == "oldName" && m.TargetNode.Label == "newName" {
			hasRenameMove = true
		}
	}
	if !hasRenameMove {
		t.Errorf("expected a rename-move match oldName -> newName, got %d matches: %+v",
			len(result.CrossFileMatches), result.CrossFileMatches)
	}
}

func TestCreateChangeset_NoMatches(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	dir := initGitRepo(t)

	writeFile(t, dir, "a.go", "package main\n\nfunc alpha() {}\n")
	gitCommit(t, dir, "initial")

	writeFile(t, dir, "a.go", "package main\n\nfunc alpha() { println(1) }\n")
	gitCommit(t, dir, "modify in place")

	payload := fmt.Sprintf(`{
		"repo_path": %q,
		"left_ref": "HEAD~1",
		"right_ref": "HEAD"
	}`, dir)

	resp, err := http.Post(env.server.URL+"/api/v1/changesets", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBody(t, resp.Body)

	var result struct {
		Files            []json.RawMessage `json:"files"`
		CrossFileMatches []json.RawMessage `json:"cross_file_matches"`
	}
	decodeJSON(t, resp.Body, &result)

	if len(result.Files) == 0 {
		t.Error("expected at least 1 file result")
	}
	if len(result.CrossFileMatches) != 0 {
		t.Errorf("expected 0 cross-file matches for in-place edit, got %d", len(result.CrossFileMatches))
	}
}

func TestCreateChangeset_ValidationErrors(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	tests := []struct {
		name    string
		payload string
	}{
		{"missing repo_path", `{"left_ref":"HEAD~1","right_ref":"HEAD"}`},
		{"missing left_ref", `{"repo_path":"/tmp","right_ref":"HEAD"}`},
		{"invalid json", `not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Post(env.server.URL+"/api/v1/changesets", "application/json",
				strings.NewReader(tt.payload))
			if err != nil {
				t.Fatal(err)
			}
			closeBody(t, resp.Body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

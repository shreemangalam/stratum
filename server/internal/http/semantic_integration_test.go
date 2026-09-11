package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestCreateDiff_ClassifiesReorderedGoStatements(t *testing.T) {
	env := setupTestEnv(t)
	defer env.close(t)

	tests := []struct {
		name        string
		left        string
		right       string
		wantVerdict string
		wantReason  string
	}{
		{
			name: "independent",
			left: `package main
func f() int {
	x := 11
	y := 22
	return x + y
}`,
			right: `package main
func f() int {
	y := 22
	x := 11
	return x + y
}`,
			wantVerdict: "behavior-preserving",
			wantReason:  "independent statements reordered",
		},
		{
			name: "dependent",
			left: `package main
func f() int {
	x := 31
	y := x + 41
	return y
}`,
			right: `package main
func f() int {
	y := x + 41
	x := 31
	return y
}`,
			wantVerdict: "behavior-changing",
			wantReason:  "dependent statements reordered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, reason := createDiffAndWaitForSemantic(t, env.server.URL, tt.left, tt.right)
			if verdict != tt.wantVerdict {
				t.Errorf("verdict = %q, want %q", verdict, tt.wantVerdict)
			}
			if reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func createDiffAndWaitForSemantic(t *testing.T, baseURL, left, right string) (string, string) {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"left":     map[string]string{"content": left},
		"right":    map[string]string{"content": right},
		"language": "go",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	resp, err := http.Post(baseURL+"/api/v1/diffs", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create diff: %v", err)
	}
	defer closeBody(t, resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("create diff status = %d, want 200 or 201", resp.StatusCode)
	}

	var job struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatalf("decode created job: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pollResp, err := http.Get(fmt.Sprintf("%s/api/v1/diffs/%s", baseURL, job.ID))
		if err != nil {
			t.Fatalf("poll diff: %v", err)
		}

		var result struct {
			Status string `json:"status"`
			Error  string `json:"error"`
			Result struct {
				Semantic []struct {
					Verdict string `json:"verdict"`
					Reason  string `json:"reason"`
				} `json:"semantic"`
			} `json:"result"`
		}
		decodeErr := json.NewDecoder(pollResp.Body).Decode(&result)
		closeBody(t, pollResp.Body)
		if decodeErr != nil {
			t.Fatalf("decode diff result: %v", decodeErr)
		}

		switch result.Status {
		case "completed":
			if len(result.Result.Semantic) != 1 {
				t.Fatalf("semantic changes = %d, want 1", len(result.Result.Semantic))
			}
			return result.Result.Semantic[0].Verdict, result.Result.Semantic[0].Reason
		case "failed":
			t.Fatalf("diff failed: %s", result.Error)
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatal("timed out waiting for semantic result")
	return "", ""
}

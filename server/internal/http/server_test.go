package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	stratumhttp "github.com/shreemangalam/stratum/server/internal/http"
)

func TestGitRoutesAreOptIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "create git diff", path: "/api/v1/diffs/git"},
		{name: "list git files", path: "/api/v1/git/files"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			disabled := stratumhttp.NewServer(nil, nil, nil, nil, nil, stratumhttp.Config{})
			disabledRecorder := httptest.NewRecorder()
			disabledRequest := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader("{}"))
			disabled.Handler().ServeHTTP(disabledRecorder, disabledRequest)
			if disabledRecorder.Code != http.StatusNotFound {
				t.Errorf("disabled route status = %d, want 404", disabledRecorder.Code)
			}

			enabled := stratumhttp.NewServer(nil, nil, nil, nil, nil, stratumhttp.Config{GitEnabled: true})
			enabledRecorder := httptest.NewRecorder()
			enabledRequest := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader("{}"))
			enabled.Handler().ServeHTTP(enabledRecorder, enabledRequest)
			if enabledRecorder.Code != http.StatusBadRequest {
				t.Errorf("enabled route status = %d, want 400", enabledRecorder.Code)
			}
		})
	}
}

package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/shreemangalam/stratum/server/internal/cache"
	"github.com/shreemangalam/stratum/server/internal/jobs"
	"github.com/shreemangalam/stratum/server/internal/parse"
	"github.com/shreemangalam/stratum/server/internal/store"
)

const maxRequestBody = 1 << 20 // 1 MB

// Server is the HTTP server for the Stratum API.
type Server struct {
	mux            *http.ServeMux
	store          store.Store
	cache          *cache.Cache
	registry       *parse.Registry
	sources        *jobs.SourceStore
	subscribers    *jobs.Subscribers
	allowedOrigins []string
}

// Config controls HTTP exposure. Git routes are opt-in because they read from
// the server's local filesystem.
type Config struct {
	AllowedOrigins []string
	GitEnabled     bool
}

// NewServer creates the HTTP server with all routes wired up.
// AllowedOrigins controls CORS; pass ["*"] to allow any origin (dev default),
// or specific origins like ["http://localhost:3000"] for production.
func NewServer(
	s store.Store,
	c *cache.Cache,
	r *parse.Registry,
	sources *jobs.SourceStore,
	subs *jobs.Subscribers,
	config Config,
) *Server {
	allowedOrigins := config.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}
	srv := &Server{
		mux:            http.NewServeMux(),
		store:          s,
		cache:          c,
		registry:       r,
		sources:        sources,
		subscribers:    subs,
		allowedOrigins: allowedOrigins,
	}

	srv.mux.HandleFunc("GET /api/v1/health", srv.handleHealth)
	srv.mux.HandleFunc("GET /api/v1/languages", srv.handleLanguages)
	srv.mux.HandleFunc("POST /api/v1/diffs", srv.handleCreateDiff)
	srv.mux.HandleFunc("POST /api/v1/merges", srv.handleCreateMerge)
	if config.GitEnabled {
		srv.mux.HandleFunc("POST /api/v1/diffs/git", srv.handleGitDiff)
		srv.mux.HandleFunc("POST /api/v1/git/files", srv.handleGitFiles)
	} else {
		notFound := func(w http.ResponseWriter, _ *http.Request) {
			http.NotFound(w, nil)
		}
		srv.mux.HandleFunc("POST /api/v1/diffs/git", notFound)
		srv.mux.HandleFunc("POST /api/v1/git/files", notFound)
	}
	srv.mux.HandleFunc("GET /api/v1/diffs/{id}", srv.handleGetDiff)
	srv.mux.HandleFunc("GET /api/v1/diffs/{id}/stream", srv.handleStreamDiff)
	srv.mux.HandleFunc("GET /api/v1/stats", srv.handleStats)

	return srv
}

const apiTimeout = 30 * time.Second

// Handler returns the HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	h := http.Handler(s.mux)
	h = s.withRequestLog(h)
	h = s.withCORS(h)
	h = withBodyLimit(h)
	h = withTimeout(h, apiTimeout)
	h = withRecover(h)
	return h
}

// responseCapture wraps http.ResponseWriter to capture the status code.
type responseCapture struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (rc *responseCapture) WriteHeader(code int) {
	if !rc.wrote {
		rc.status = code
		rc.wrote = true
	}
	rc.ResponseWriter.WriteHeader(code)
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	if !rc.wrote {
		rc.status = http.StatusOK
		rc.wrote = true
	}
	return rc.ResponseWriter.Write(b)
}

func (rc *responseCapture) Unwrap() http.ResponseWriter {
	return rc.ResponseWriter
}

func (rc *responseCapture) Flush() {
	if f, ok := rc.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func withTimeout(next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stream") {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"error", fmt.Sprint(rec),
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, `{"error":"internal server error"}`)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.ContentLength > maxRequestBody {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{
				Error: fmt.Sprintf("request body too large (max %d bytes)", maxRequestBody),
			})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	allowAll := len(s.allowedOrigins) == 1 && s.allowedOrigins[0] == "*"
	originSet := make(map[string]bool, len(s.allowedOrigins))
	for _, o := range s.allowedOrigins {
		originSet[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" && originSet[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = generateID()
		}
		w.Header().Set("X-Request-ID", reqID)

		rc := &responseCapture{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rc, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rc.status,
			"duration", time.Since(start),
			"request_id", reqID,
		)
	})
}

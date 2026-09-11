package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shreemangalam/stratum/server/internal/cache"
	"github.com/shreemangalam/stratum/server/internal/jobs"
	"github.com/shreemangalam/stratum/server/internal/parse"
	"github.com/shreemangalam/stratum/server/internal/parse/treesitter"
	"github.com/shreemangalam/stratum/server/internal/parse/xslt"
	"github.com/shreemangalam/stratum/server/internal/store"

	stratumhttp "github.com/shreemangalam/stratum/server/internal/http"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	port := envOrDefault("PORT", "8080")
	dbURL := envOrDefault("DATABASE_URL", "postgres://stratum:stratum@localhost:5433/stratum?sslmode=disable")
	workerCount := 4
	cacheTTL := 24 * time.Hour

	db, err := store.NewPostgres(dbURL)
	if err != nil {
		slog.Error("connecting to database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("closing database", "error", err)
		}
	}()

	if err := db.RunMigrations(context.Background()); err != nil {
		slog.Error("running migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	appCache := cache.New(cacheTTL)
	appCache.StartEviction(context.Background(), 10*time.Minute)

	registry := parse.NewRegistry()
	goParser := treesitter.NewGo()
	registry.Register(goParser)

	for _, lang := range treesitter.SupportedLanguages() {
		if lang.Lang == "go" {
			continue
		}
		registry.Register(treesitter.NewGeneric(lang.Lang, lang.Exts))
	}
	registry.Register(xslt.New())
	slog.Info("parsers registered", "languages", registry.Languages())

	sources := jobs.NewSourceStore()
	subscribers := jobs.NewSubscribers()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool := jobs.NewPool(workerCount, db, appCache, registry, sources, subscribers)
	pool.Start(ctx)
	defer pool.Stop()

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n, err := db.DeleteOldJobs(context.Background(), 7*24*time.Hour)
				if err != nil {
					slog.Error("deleting old jobs", "error", err)
				} else if n > 0 {
					slog.Info("deleted old jobs", "count", n)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	allowedOrigins := parseOrigins(envOrDefault("ALLOWED_ORIGINS", "http://localhost:3000"))
	gitEnabled := envEnabled("GIT_ENABLED")

	srv := stratumhttp.NewServer(db, appCache, registry, sources, subscribers, stratumhttp.Config{
		AllowedOrigins: allowedOrigins,
		GitEnabled:     gitEnabled,
	})

	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool.Stop()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envEnabled(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseOrigins(raw string) []string {
	if raw == "*" {
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			origins = append(origins, p)
		}
	}
	if len(origins) == 0 {
		return []string{"*"}
	}
	return origins
}

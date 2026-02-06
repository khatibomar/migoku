package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Application struct {
	logger  *slog.Logger
	cache   *Cache
	service *MigakuService

	headless      bool
	port          int
	loginWaitTime time.Duration
	cors          []string
	secretKey     string

	accounts map[string]*Browser
}

var _, longVersion, _ = FromBuildInfo()

func main() {
	logLevel := os.Getenv("LOG_LEVEL")
	var logLvl slog.Level
	if err := logLvl.UnmarshalText([]byte(logLevel)); err != nil {
		logLvl = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLvl}))
	slog.SetDefault(logger)

	logger.Info("Application starting", "version", longVersion, "log_level", logLvl.String())

	if err := realMain(logger); err != nil {
		logger.Error("Application error", "error", err)
		os.Exit(1)
	}
}

func realMain(logger *slog.Logger) error {
	headless := os.Getenv("HEADLESS") != "false"
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port number: %w", err)
	}
	corsOrigins := os.Getenv("CORS_ORIGINS")
	var cors []string
	if corsOrigins == "" {
		cors = []string{"*"}
	} else {
		cors = strings.Split(corsOrigins, ",")
		for i, origin := range cors {
			cors[i] = strings.TrimSpace(origin)
		}
	}
	cacheTTL := os.Getenv("CACHE_TTL")
	var cacheTTLDuration time.Duration
	if cacheTTL == "" {
		cacheTTLDuration = defaultCacheTTL
	} else {
		cacheTTLDuration, err = time.ParseDuration(cacheTTL)
		if err != nil {
			logger.Error("Invalid CACHE_TTL value", "value", cacheTTL)
			return fmt.Errorf("invalid CACHE_TTL value: %w", err)
		}
	}

	cache := NewCache(cacheTTLDuration)

	secretKey := os.Getenv("API_SECRET")
	if secretKey == "" {
		return errors.New("API_SECRET environment variable is required")
	}

	logger.Info("Initializing browser and logging in...")

	app := &Application{
		headless:      headless,
		port:          portInt,
		loginWaitTime: 30 * time.Second,
		cors:          cors,
		cache:         cache,
		logger:        logger,
		secretKey:     secretKey,
		accounts:      make(map[string]*Browser),
	}

	repo := NewRepository()
	app.service = NewMigakuService(repo, cache)

	logger.Info("Login complete, browser ready for queries")

	//--- Start HTTP server ---
	chainMiddlewares := func(handler http.HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
		return handler
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", chainMiddlewares(app.handleRoot, app.corsMiddleware))
	mux.HandleFunc("/docs", chainMiddlewares(app.handleDocs, app.corsMiddleware))
	mux.HandleFunc("/openapi.yaml", chainMiddlewares(app.handleOpenAPISpec, app.corsMiddleware))
	mux.HandleFunc("/auth/login", chainMiddlewares(app.handleLogin, app.corsMiddleware))
	mux.HandleFunc("/auth/logout", chainMiddlewares(app.handleLogout, app.corsMiddleware, app.authMiddleware))

	v1 := http.NewServeMux()
	v1.HandleFunc("GET /words", chainMiddlewares(app.handleWords, app.corsMiddleware, app.authMiddleware))
	v1.HandleFunc("POST /words/status", chainMiddlewares(app.handleSetWordStatus, app.corsMiddleware, app.authMiddleware))
	v1.HandleFunc("GET /decks", chainMiddlewares(app.handleDecks, app.corsMiddleware, app.authMiddleware))
	v1.HandleFunc("GET /status/counts", chainMiddlewares(app.handleStatusCounts, app.corsMiddleware, app.authMiddleware))
	v1.HandleFunc("GET /words/difficult", chainMiddlewares(app.handleDifficultWords, app.corsMiddleware, app.authMiddleware))
	v1.HandleFunc("GET /stats/words", chainMiddlewares(app.handleWordStats, app.corsMiddleware, app.authMiddleware))
	v1.HandleFunc("GET /stats/due", chainMiddlewares(app.handleDueStats, app.corsMiddleware, app.authMiddleware))
	v1.HandleFunc("GET /stats/intervals", chainMiddlewares(app.handleIntervalStats, app.corsMiddleware, app.authMiddleware))
	v1.HandleFunc("GET /stats/study", chainMiddlewares(app.handleStudyStats, app.corsMiddleware, app.authMiddleware))

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", v1))

	utility := http.NewServeMux()
	utility.HandleFunc("GET /status", chainMiddlewares(app.handleStatus, app.corsMiddleware, app.authMiddleware))
	utility.HandleFunc("GET /database/schema", chainMiddlewares(app.handleDatabaseSchema, app.corsMiddleware, app.authMiddleware))
	utility.HandleFunc("POST /cache/clear", chainMiddlewares(app.handleClearCache, app.corsMiddleware, app.authMiddleware))
	utility.HandleFunc("GET /tables", chainMiddlewares(app.handleTables, app.corsMiddleware, app.authMiddleware))

	mux.Handle("/dev/", http.StripPrefix("/dev", utility))

	logger.Info("Server starting", "url", "http://localhost:"+port)
	logger.Info("Cache TTL", "ttl", cache.ttl.String())

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("Server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Server failed", "error", err)
		}
	}()

	<-done
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, browser := range app.accounts {
		if browser != nil {
			browser.Close()
		}
	}

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	logger.Info("Server exited")
	return nil
}

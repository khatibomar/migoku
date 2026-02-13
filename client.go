package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type MigakuClient struct {
	mu      sync.RWMutex
	logger  *slog.Logger
	session *MigakuSession
	db      *sqlx.DB
	dbPath  string
	cleanUp func()
	key     string

	lastRefresh time.Time
	refreshTTL  time.Duration
	refreshWg   sync.WaitGroup
	refreshStop context.CancelFunc
}

// NewMigakuClient initializes an API session and downloads the Migaku SRS database.
// It returns an error if login fails or if the database cannot be fetched.
//
//nolint:contextcheck // background refresh loop not tied to request context
func NewMigakuClient(
	ctx context.Context,
	logger *slog.Logger,
	email, password string,
	ttl time.Duration,
) (c *MigakuClient, err error) {
	defer func() {
		if err != nil && c != nil {
			c.Close()
		}
	}()

	authToken, err := TryFromEmailPassword(ctx, email, password)
	if err != nil {
		return nil, err
	}
	if authToken == nil {
		return nil, errors.New("login failed: invalid credentials")
	}

	logger.Debug("Auth token acquired")

	session := NewMigakuSession(authToken)
	c = &MigakuClient{
		logger:     logger,
		session:    session,
		refreshTTL: ttl,
	}

	dbDir := filepath.Join(os.TempDir(), "migoku-db")
	if err = os.MkdirAll(dbDir, os.ModePerm); err != nil {
		c.logger.Error("failed to create temp db dir", "error", err)
		return nil, err
	}

	key := hashProfileDirKey(email)
	c.key = key
	c.dbPath = filepath.Join(dbDir, "migaku-"+key+".db")
	c.logger.Debug("Using local db path", "path", c.dbPath)
	if err = c.refreshDB(ctx); err != nil {
		return nil, err
	}

	if ttl > 0 {
		refreshCtx, refreshStop := context.WithCancel(context.Background())
		c.refreshStop = refreshStop
		c.refreshWg.Go(func() {
			ticker := time.NewTicker(ttl)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := c.refreshDBIfStale(refreshCtx, ttl); err != nil {
						c.logger.Error("failed to refresh db", "error", err)
					}
				case <-refreshCtx.Done():
					c.logger.Debug("Stopping refresh loop")
					return
				}
			}
		})
	}

	c.cleanUp = func() {
		c.closeDB()
	}

	c.logger.Info("Migaku session ready")
	return c, nil
}

func (c *MigakuClient) refreshDB(ctx context.Context) error {
	start := time.Now()
	c.logger.Debug("Refreshing local database")

	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil {
		return errors.New("missing migaku session")
	}

	data, err := session.ForceDownloadSRSDB(ctx)
	if err != nil {
		return err
	}
	c.logger.Debug("Downloaded database", "bytes", len(data))

	tmpPath := c.dbPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write db temp file: %w", err)
	}

	newDB, err := sqlx.Open("sqlite", tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to open new sqlite db: %w", err)
	}
	newDB.SetMaxOpenConns(1)
	newDB.SetMaxIdleConns(1)

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.Rename(tmpPath, c.dbPath); err != nil {
		_ = newDB.Close()
		return fmt.Errorf("failed to swap db file: %w", err)
	}

	if c.db != nil {
		_ = c.db.Close()
	}
	c.db = newDB
	c.lastRefresh = time.Now()
	c.logger.Debug("Local database refreshed", "duration_ms", time.Since(start).Milliseconds())
	return nil
}

func (c *MigakuClient) refreshDBLocked(ctx context.Context) error {
	start := time.Now()
	c.logger.Debug("Refreshing local database (locked)")

	if c.session == nil {
		return errors.New("missing migaku session")
	}

	// We already hold the lock, so we can't optimize this path
	// But this is only used for initial setup, not periodic refreshes
	data, err := c.session.ForceDownloadSRSDB(ctx)
	if err != nil {
		return err
	}
	c.logger.Debug("Downloaded database", "bytes", len(data))

	tmpPath := c.dbPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write db temp file: %w", err)
	}
	if err := os.Rename(tmpPath, c.dbPath); err != nil {
		return fmt.Errorf("failed to swap db file: %w", err)
	}

	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}

	db, err := sqlx.Open("sqlite", c.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	c.db = db
	c.lastRefresh = time.Now()
	c.logger.Debug("Local database refreshed", "duration_ms", time.Since(start).Milliseconds())
	return nil
}

func (c *MigakuClient) refreshDBIfStale(ctx context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		c.logger.Debug("Skipping db refresh; ttl disabled")
		return nil
	}
	buffer := 2 * time.Second
	threshold := max(ttl-buffer, 0)

	if !c.isRefreshStale(threshold) {
		c.logger.Debug("Skipping db refresh; not stale", "ttl", ttl.String())
		return nil
	}
	c.logger.Debug("Db refresh required", "ttl", ttl.String())

	return c.refreshDB(ctx)
}

func (c *MigakuClient) isRefreshStale(threshold time.Duration) bool {
	c.mu.RLock()
	last := c.lastRefresh
	c.mu.RUnlock()
	return time.Since(last) >= threshold
}

func (c *MigakuClient) ensureDBLocked(ctx context.Context) (*sqlx.DB, error) {
	if c.db != nil {
		return c.db, nil
	}

	if _, err := os.Stat(c.dbPath); err == nil {
		c.logger.Debug("Opening existing db file", "path", c.dbPath)
		db, err := sqlx.Open("sqlite", c.dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite db: %w", err)
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		c.db = db
		return c.db, nil
	}

	c.logger.Debug("Db file missing; downloading fresh db")
	if err := c.refreshDBLocked(ctx); err != nil {
		return nil, err
	}
	return c.db, nil
}

func (c *MigakuClient) closeDB() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
	}
}

func (c *MigakuClient) Close() {
	if c.refreshStop != nil {
		c.refreshStop()
		c.refreshStop = nil
	}
	c.refreshWg.Wait()
	if c.cleanUp != nil {
		c.cleanUp()
	}
}

func hashProfileDirKey(email string) string {
	key := email
	hash := 0
	for _, r := range key {
		hash = int(r) + ((hash << 5) - hash)
	}
	return fmt.Sprintf("%x", hash)
}

func runQuery[T any](ctx context.Context, client *MigakuClient, query string, params ...any) ([]T, error) {
	if client == nil {
		return nil, errors.New("missing authenticated session")
	}
	return runReadQuery[T](ctx, client, query, params...)
}

func runReadQuery[T any](ctx context.Context, client *MigakuClient, query string, params ...any) ([]T, error) {
	if client == nil {
		return nil, errors.New("missing authenticated session")
	}

	client.logger.Info("Running read query", "query", query, "params", params)

	client.mu.RLock()
	if client.db != nil {
		db := client.db
		defer client.mu.RUnlock()
		var result []T
		if err := db.SelectContext(ctx, &result, query, params...); err != nil {
			client.logger.Error("Read query failed", "error", err)
			return nil, fmt.Errorf("failed to execute read query: %w", err)
		}
		client.logger.Info("Read query completed", "rows", len(result))
		return result, nil
	}
	client.mu.RUnlock()

	client.mu.Lock()
	defer client.mu.Unlock()
	db, err := client.ensureDBLocked(ctx)
	if err != nil {
		return nil, err
	}

	var result []T
	if err := db.SelectContext(ctx, &result, query, params...); err != nil {
		client.logger.Error("Read query failed", "error", err)
		return nil, fmt.Errorf("failed to execute read query: %w", err)
	}

	client.logger.Info("Read query completed", "rows", len(result))
	return result, nil
}

func runReadRow(ctx context.Context, client *MigakuClient, query string, params ...any) (map[string]any, error) {
	if client == nil {
		return nil, errors.New("missing authenticated session")
	}

	client.logger.Info("Running read row query", "query", query, "params", params)

	client.mu.RLock()
	if client.db != nil {
		db := client.db
		defer client.mu.RUnlock()
		row := db.QueryRowxContext(ctx, query, params...)
		raw := map[string]any{}
		if err := row.MapScan(raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
	client.mu.RUnlock()

	client.mu.Lock()
	defer client.mu.Unlock()
	db, err := client.ensureDBLocked(ctx)
	if err != nil {
		return nil, err
	}
	row := db.QueryRowxContext(ctx, query, params...)
	raw := map[string]any{}
	if err := row.MapScan(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func runWriteQuery(ctx context.Context, client *MigakuClient, query string, params ...any) (sql.Result, error) {
	if client == nil {
		return nil, errors.New("missing authenticated session")
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	client.logger.Info("Running write query", "query", query, "params", params)

	db, err := client.ensureDBLocked(ctx)
	if err != nil {
		return nil, err
	}

	result, err := db.ExecContext(ctx, query, params...)
	if err != nil {
		client.logger.Error("Write query failed", "error", err)
		return nil, fmt.Errorf("failed to execute write query: %w", err)
	}

	return result, nil
}

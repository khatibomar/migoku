package main

import (
	"context"
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
	refreshMu   sync.Mutex
	dbUseMu     sync.RWMutex
}

// NewMigakuClient initializes an API session and downloads the Migaku SRS database.
// It returns an error if login fails or if the database cannot be fetched.
func NewMigakuClient(
	ctx context.Context,
	logger *slog.Logger,
	email, password string,
) (c *MigakuClient, err error) {
	defer func() {
		if err != nil && c != nil && c.cleanUp != nil {
			c.cleanUp()
		}
	}()

	authToken, err := TryFromEmailPassword(ctx, email, password)
	if err != nil {
		return nil, err
	}
	if authToken == nil {
		return nil, errors.New("login failed: invalid credentials")
	}

	session := NewMigakuSession(authToken)
	c = &MigakuClient{
		logger:  logger,
		session: session,
	}

	dbDir := filepath.Join(os.TempDir(), "migoku-db")
	if err = os.MkdirAll(dbDir, os.ModePerm); err != nil {
		c.logger.Error("failed to create temp db dir", "error", err)
		return nil, err
	}

	key := hashProfileDirKey(email)
	c.key = key
	c.dbPath = filepath.Join(dbDir, "migaku-"+key+".db")
	if err = c.refreshDB(ctx); err != nil {
		return nil, err
	}

	c.cleanUp = func() {
		c.closeDB()
	}

	c.logger.Info("Migaku session ready")
	return c, nil
}

func (c *MigakuClient) refreshDB(ctx context.Context) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.dbUseMu.Lock()
	defer c.dbUseMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session == nil {
		return errors.New("missing migaku session")
	}

	data, err := c.session.ForceDownloadSRSDB(ctx)
	if err != nil {
		return err
	}

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
	return nil
}

func (c *MigakuClient) refreshDBIfStale(ctx context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	buffer := 2 * time.Second
	threshold := max(ttl-buffer, 0)

	if !c.isRefreshStale(threshold) {
		return nil
	}

	return c.refreshDB(ctx)
}

func (c *MigakuClient) isRefreshStale(threshold time.Duration) bool {
	c.mu.RLock()
	last := c.lastRefresh
	c.mu.RUnlock()
	return time.Since(last) >= threshold
}

func (c *MigakuClient) ensureDB(ctx context.Context) (*sqlx.DB, error) {
	c.mu.RLock()
	if c.db != nil {
		db := c.db
		c.mu.RUnlock()
		return db, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db != nil {
		return c.db, nil
	}

	if _, err := os.Stat(c.dbPath); err == nil {
		db, err := sqlx.Open("sqlite", c.dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite db: %w", err)
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		c.db = db
		return c.db, nil
	}

	if err := c.refreshDB(ctx); err != nil {
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

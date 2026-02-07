package main

import (
	"context"
	"errors"
	"fmt"
)

func runQuery[T any](ctx context.Context, client *MigakuClient, query string, params ...any) ([]T, error) {
	if client == nil {
		return nil, errors.New("missing authenticated session")
	}

	client.logger.Info("Running query", "query", query, "params", params)

	db, err := client.ensureDB(ctx)
	if err != nil {
		return nil, err
	}

	client.dbUseMu.RLock()
	defer client.dbUseMu.RUnlock()

	var result []T
	if err := db.SelectContext(ctx, &result, query, params...); err != nil {
		client.logger.Error("Query execution failed", "error", err)
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	client.logger.Info("Query completed", "rows", len(result))
	return result, nil
}

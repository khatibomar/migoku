package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

//go:embed snippets/query_runner.js
var queryRunnerScript string

func runQuery[T any](ctx context.Context, browser *Browser, query string, params ...any) ([]T, error) {
	browser.logger.Info("Running query", "query", query, "params", params)

	paramsJSON := "[]"
	if len(params) > 0 {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		paramsJSON = string(paramsBytes)
	}

	script := strings.ReplaceAll(queryRunnerScript, "__QUERY__", "`"+query+"`")
	script = strings.ReplaceAll(script, "__PARAMS__", paramsJSON)

	awaitPromise := func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	}

	var result []T
	eval := chromedp.Evaluate(script, &result, awaitPromise)
	runCtx, cancel := browserRunContext(ctx, browser)
	defer cancel()
	if err := chromedp.Run(runCtx, eval); err != nil {
		browser.logger.Error("Query execution failed", "error", err)
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	browser.logger.Info("Query completed", "rows", len(result))
	return result, nil
}

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const wordBrowserURL = "https://study.migaku.com/word-browser"

//go:embed snippets/word_status.js
var wordStatusScript string

var wordStatusActionLabels = map[string]string{
	"known":    "Change to known",
	"learning": "Change to learning",
	"tracked":  "Change to tracked & unknown",
	"ignored":  "Change to ignored",
}

type WordStatusResult struct {
	Ok      bool                   `json:"ok"`
	Reason  string                 `json:"reason,omitempty"`
	Results []WordStatusItemResult `json:"results,omitempty"`
}

type WordStatusItem struct {
	WordText  string `json:"wordText"`
	Secondary string `json:"secondary,omitempty"`
}

type WordStatusItemResult struct {
	WordText  string `json:"wordText"`
	Secondary string `json:"secondary,omitempty"`
	Ok        bool   `json:"ok"`
	Reason    string `json:"reason,omitempty"`
}

type wordStatusPayload struct {
	Items       []WordStatusItem `json:"items,omitempty"`
	WordText    string           `json:"wordText,omitempty"`
	Secondary   string           `json:"secondary,omitempty"`
	ActionLabel string           `json:"actionLabel"`
}

func actionLabelFromStatus(status string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	actionLabel, ok := wordStatusActionLabels[normalized]
	return actionLabel, ok
}

func (s *MigakuService) SetWordStatus(
	ctx context.Context,
	browser *Browser,
	wordText, secondary, status string,
) (*WordStatusResult, error) {
	wordText = strings.TrimSpace(wordText)
	secondary = strings.TrimSpace(secondary)
	browser.logger.Info(
		"Updating word status",
		slog.String("status", status),
		slog.String("wordText", wordText),
		slog.String("secondary", secondary),
	)

	return s.setWordStatusItems(ctx, browser, []WordStatusItem{
		{
			WordText:  wordText,
			Secondary: secondary,
		},
	}, status)
}

func (s *MigakuService) SetWordStatusBatch(
	ctx context.Context,
	browser *Browser,
	items []WordStatusItem,
	status string,
) (*WordStatusResult, error) {
	browser.logger.Info(
		"Updating word status batch",
		slog.String("status", status),
		slog.Int("count", len(items)),
	)
	return s.setWordStatusItems(ctx, browser, items, status)
}

func (s *MigakuService) setWordStatusItems(
	ctx context.Context,
	browser *Browser,
	items []WordStatusItem,
	status string,
) (*WordStatusResult, error) {
	if browser == nil {
		return nil, errors.New("browser not authenticated")
	}

	actionLabel, ok := actionLabelFromStatus(status)
	if !ok {
		return nil, errors.New("invalid status: must be one of: known, learning, tracked, ignored")
	}

	if len(items) == 0 {
		return nil, errors.New("wordText is required")
	}

	normalizedItems := make([]WordStatusItem, 0, len(items))
	for _, item := range items {
		wordText := strings.TrimSpace(item.WordText)
		secondary := strings.TrimSpace(item.Secondary)
		if wordText == "" {
			return nil, errors.New("wordText is required")
		}
		normalizedItems = append(normalizedItems, WordStatusItem{
			WordText:  wordText,
			Secondary: secondary,
		})
	}

	payload := wordStatusPayload{
		Items:       normalizedItems,
		ActionLabel: actionLabel,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal status payload: %w", err)
	}

	script := strings.Replace(wordStatusScript, "__PAYLOAD__", string(payloadJSON), 1)

	awaitPromise := func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	}

	var result WordStatusResult
	eval := chromedp.Evaluate(script, &result, awaitPromise)
	runCtx, cancel := browserRunContext(ctx, browser)
	defer cancel()
	if err := chromedp.Run(runCtx,
		chromedp.Navigate(wordBrowserURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		eval,
	); err != nil {
		return nil, fmt.Errorf("failed to update word status: %w", err)
	}

	if result.Ok {
		s.cache.Clear()
	}

	return &result, nil
}

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
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
	Ok     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

type wordStatusPayload struct {
	WordText    string `json:"wordText"`
	Secondary   string `json:"secondary"`
	ActionLabel string `json:"actionLabel"`
}

func normalizeWordStatus(status string) (string, string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	actionLabel, ok := wordStatusActionLabels[normalized]
	return normalized, actionLabel, ok
}

func (s *MigakuService) SetWordStatus(
	ctx context.Context,
	browser *Browser,
	wordText, secondary, status string,
) (*WordStatusResult, error) {
	if browser == nil {
		return nil, errors.New("browser not authenticated")
	}

	status, actionLabel, ok := normalizeWordStatus(status)
	if !ok {
		return nil, errors.New("invalid status: must be one of: known, learning, tracked, ignored")
	}

	wordText = strings.TrimSpace(wordText)
	secondary = strings.TrimSpace(secondary)
	if wordText == "" && secondary == "" {
		return nil, errors.New("wordText or secondary is required")
	}

	payload := wordStatusPayload{
		WordText:    wordText,
		Secondary:   secondary,
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

	browser.logger.Info("Updating word status", "status", status, "wordText", wordText, "secondary", secondary)

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

package main

import (
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

func normalizeWordStatus(status string) (string, string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	actionLabel, ok := wordStatusActionLabels[normalized]
	return normalized, actionLabel, ok
}

func (s *MigakuService) SetWordStatus(wordText, secondary, status string) (*WordStatusResult, error) {
	app := s.repo.app
	wordText = strings.TrimSpace(wordText)
	secondary = strings.TrimSpace(secondary)
	app.logger.Info("Updating word status", "status", status, "wordText", wordText, "secondary", secondary)

	return s.setWordStatusItems([]WordStatusItem{
		{
			WordText:  wordText,
			Secondary: secondary,
		},
	}, status)
}

func (s *MigakuService) SetWordStatusBatch(items []WordStatusItem, status string) (*WordStatusResult, error) {
	s.repo.app.logger.Info("Updating word status batch", "status", status, "count", len(items))
	return s.setWordStatusItems(items, status)
}

func (s *MigakuService) setWordStatusItems(items []WordStatusItem, status string) (*WordStatusResult, error) {
	app := s.repo.app
	if !app.isAuthenticated.Load() {
		return nil, errors.New("browser not authenticated")
	}

	_, actionLabel, ok := normalizeWordStatus(status)
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
	if err := chromedp.Run(app.browserCtx,
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

package main

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

//go:embed api.html
var apiHTML []byte

func (app *Application) respondJSON(w http.ResponseWriter, data any) {
	if err := encode(w, nil, http.StatusOK, data); err != nil {
		app.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (app *Application) requireBrowser(w http.ResponseWriter, r *http.Request) (*Browser, bool) {
	browser, ok := browserFromContext(r.Context())
	if ok {
		return browser, true
	}

	app.writeJSONError(w, r, http.StatusUnauthorized, "Unauthorized")
	return nil, false
}

type wordStatusRequest struct {
	Status    string           `json:"status"`
	WordText  string           `json:"wordText"`
	Secondary string           `json:"secondary"`
	Items     []WordStatusItem `json:"items"`
}

func (app *Application) handleWords(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	status := r.URL.Query().Get("status")

	words, err := app.service.GetWords(r.Context(), browser, lang, status)
	if err != nil {
		if err.Error() == "invalid status: must be one of: known, learning, unknown, ignored" {
			app.writeJSONError(w, r, http.StatusBadRequest, "Status must be one of: known, learning, unknown, ignored")
			return
		}
		app.logger.Error("Failed to get words", "error", err, "status", status)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	app.respondJSON(w, words)
}

func (app *Application) handleSetWordStatus(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	var req wordStatusRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		app.writeJSONError(w, r, http.StatusBadRequest, "Request body must be valid JSON")
		return
	}

	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	req.WordText = strings.TrimSpace(req.WordText)
	req.Secondary = strings.TrimSpace(req.Secondary)

	if req.Status == "" {
		app.writeJSONError(w, r, http.StatusBadRequest, "Status is required")
		return
	}

	if _, ok := actionLabelFromStatus(req.Status); !ok {
		app.writeJSONError(w, r, http.StatusBadRequest, "Status must be one of: known, learning, tracked, ignored")
		return
	}

	if len(req.Items) == 0 {
		if req.WordText == "" {
			app.writeJSONError(w, r, http.StatusBadRequest, "WordText is required")
			return
		}
	}

	if len(req.Items) > 0 {
		items := make([]WordStatusItem, 0, len(req.Items))
		for _, item := range req.Items {
			wordText := strings.TrimSpace(item.WordText)
			secondary := strings.TrimSpace(item.Secondary)
			if wordText == "" {
				app.writeJSONError(w, r, http.StatusBadRequest, "WordText is required for each item")
				return
			}
			items = append(items, WordStatusItem{
				WordText:  wordText,
				Secondary: secondary,
			})
		}

		result, err := app.service.SetWordStatusBatch(r.Context(), browser, items, req.Status)
		if err != nil {
			app.logger.Error("Failed to update word status batch", "error", err, "status", req.Status, "count", len(items))
			app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		app.respondJSON(w, result)
		return
	}

	result, err := app.service.SetWordStatus(r.Context(), browser, req.WordText, req.Secondary, req.Status)
	if err != nil {
		app.logger.Error(
			"Failed to update word status",
			"error",
			err,
			"status",
			req.Status,
			"wordText",
			req.WordText,
			"secondary",
			req.Secondary,
		)
		app.writeJSONError(w, r, http.StatusInternalServerError, "Internal server error")
		return
	}

	app.respondJSON(w, result)
}

func (app *Application) handleDecks(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	decks, err := app.service.GetDecks(r.Context(), browser)
	if err != nil {
		app.logger.Error("Failed to get decks", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	app.respondJSON(w, decks)
}

func (app *Application) handleStatusCounts(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	deckID := r.URL.Query().Get("deckId")

	counts, err := app.service.GetStatusCounts(r.Context(), browser, lang, deckID)
	if err != nil {
		app.logger.Error("Failed to get status counts", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Return as array for backward compatibility with frontend
	app.respondJSON(w, []StatusCounts{*counts})
}

func (app *Application) handleTables(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	tables, err := app.service.GetTables(r.Context(), browser)
	if err != nil {
		app.logger.Error("Failed to get tables", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	app.respondJSON(w, tables)
}

func (app *Application) handleDatabaseSchema(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	schema, err := app.service.GetDatabaseSchema(r.Context(), browser)
	if err != nil {
		app.logger.Error("Failed to get database schema", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	app.respondJSON(w, schema)
}

func (app *Application) handleDifficultWords(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if lang == "" {
		app.writeJSONError(w, r, http.StatusBadRequest, "lang is required")
		return
	}

	deckID := r.URL.Query().Get("deckId")

	words, err := app.service.GetDifficultWords(r.Context(), browser, lang, limit, deckID)
	if err != nil {
		app.logger.Error("Failed to get difficult words", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, words)
}

func (app *Application) handleWordStats(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		app.writeJSONError(w, r, http.StatusBadRequest, "lang is required")
		return
	}

	deckID := r.URL.Query().Get("deckId")

	stats, err := app.service.GetWordStats(r.Context(), browser, lang, deckID)
	if err != nil {
		app.logger.Error("Failed to get word stats", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, stats)
}

func (app *Application) handleDueStats(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		app.writeJSONError(w, r, http.StatusBadRequest, "lang is required")
		return
	}

	deckID := r.URL.Query().Get("deckId")
	periodID := r.URL.Query().Get("periodId")

	stats, err := app.service.GetDueStats(r.Context(), browser, lang, deckID, periodID)
	if err != nil {
		app.logger.Error("Failed to get due stats", slog.String("error", err.Error()))
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, stats)
}

func (app *Application) handleIntervalStats(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		app.writeJSONError(w, r, http.StatusBadRequest, "lang is required")
		return
	}

	deckID := r.URL.Query().Get("deckId")
	percentileID := r.URL.Query().Get("percentileId")

	stats, err := app.service.GetIntervalStats(r.Context(), browser, lang, deckID, percentileID)
	if err != nil {
		app.logger.Error("Failed to get interval stats", slog.String("error", err.Error()))
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, stats)
}

func (app *Application) handleStudyStats(w http.ResponseWriter, r *http.Request) {
	browser, ok := app.requireBrowser(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		app.writeJSONError(w, r, http.StatusBadRequest, "lang is required")
		return
	}

	deckID := r.URL.Query().Get("deckId")
	periodID := r.URL.Query().Get("periodId")

	stats, err := app.service.GetStudyStats(r.Context(), browser, lang, deckID, periodID)
	if err != nil {
		app.logger.Error("Failed to get study stats", slog.String("error", err.Error()))
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, stats)
}

func (app *Application) handleStatus(w http.ResponseWriter, r *http.Request) {
	app.respondJSON(w, map[string]any{
		"status":    "running",
		"cache_ttl": app.cache.ttl.String(),
		"headless":  app.headless,
	})
}

func (app *Application) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.writeJSONError(w, r, http.StatusNotFound, "The requested endpoint does not exist")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write(apiHTML)
	if err != nil {
		app.logger.Error("Failed to write root response", slog.String("error", err.Error()))
	}
}

func (app *Application) handleClearCache(w http.ResponseWriter, r *http.Request) {
	app.cache.Clear()
	app.logger.Info("Cache cleared")
	app.respondJSON(w, map[string]string{
		"status":  "success",
		"message": "Cache cleared successfully",
	})
}

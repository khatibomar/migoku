package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

//go:embed docs.html
var docsHTML []byte

//go:embed openapi.yaml
var openAPISpec []byte

func (app *Application) respondJSON(w http.ResponseWriter, r *http.Request, data any) {
	if err := encode(w, r, http.StatusOK, data); err != nil {
		app.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (app *Application) requireClient(w http.ResponseWriter, r *http.Request) (*MigakuClient, bool) {
	client, ok := clientFromContext(r.Context())
	if ok {
		return client, true
	}

	app.writeJSONError(w, r, http.StatusUnauthorized, "Unauthorized")
	return nil, false
}

type wordStatusRequest struct {
	Status    string           `json:"status"`
	WordText  string           `json:"wordText"`
	Secondary string           `json:"secondary"`
	Items     []WordStatusItem `json:"items"`
	Language  string           `json:"language"`
}

func (app *Application) handleWords(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	status := r.URL.Query().Get("status")
	deckID := r.URL.Query().Get("deckId")
	form := r.URL.Query().Get("form")
	formExactStr := r.URL.Query().Get("formExact")
	formExact := false
	if formExactStr != "" {
		parsedExact, err := strconv.ParseBool(formExactStr)
		if err != nil {
			app.writeJSONError(w, r, http.StatusBadRequest, "formExact must be a boolean")
			return
		}
		formExact = parsedExact
	}

	words, err := app.service.GetWords(r.Context(), client, lang, status, deckID, form, formExact)
	if err != nil {
		if err.Error() == "invalid status: must be one of: known, learning, unknown, ignored" {
			app.writeJSONError(w, r, http.StatusBadRequest, "Status must be one of: known, learning, unknown, ignored")
			return
		}
		app.logger.Error("Failed to get words", "error", err, "status", status)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	app.respondJSON(w, r, words)
}

func (app *Application) handleSetWordStatus(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
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

	if _, ok := statusToUpdate(req.Status); !ok {
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

		err := app.service.SetWordStatusBatch(r.Context(), client, items, req.Status, req.Language)
		if err != nil {
			status := http.StatusInternalServerError
			message := "Internal server error"

			switch {
			case errors.Is(err, ErrWordNotFound):
				status = http.StatusNotFound
				message = err.Error()
			case errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrWordTextRequired):
				status = http.StatusBadRequest
				message = err.Error()
			case errors.Is(err, ErrClientNotAuth):
				status = http.StatusUnauthorized
				message = err.Error()
			default:
				app.logger.Error("Failed to update word status batch", "error", err, "status", req.Status, "count", len(items))
			}

			app.writeJSONError(w, r, status, message)
			return
		}

		app.respondJSON(w, r, map[string]any{
			"message": "Word status updated successfully",
			"count":   len(items),
		})
		return
	}

	err := app.service.SetWordStatus(r.Context(), client, req.WordText, req.Secondary, req.Status, req.Language)
	if err != nil {
		status := http.StatusInternalServerError
		message := "Internal server error"

		// Determine appropriate status code based on error type
		switch {
		case errors.Is(err, ErrWordNotFound):
			status = http.StatusNotFound
			message = err.Error()
		case errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrWordTextRequired):
			status = http.StatusBadRequest
			message = err.Error()
		case errors.Is(err, ErrClientNotAuth):
			status = http.StatusUnauthorized
			message = err.Error()
		default:
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
		}

		app.writeJSONError(w, r, status, message)
		return
	}

	app.respondJSON(w, r, map[string]string{
		"message": "Word status updated successfully",
	})
}

func (app *Application) handleDecks(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
	if !ok {
		return
	}

	decks, err := app.service.GetDecks(r.Context(), client)
	if err != nil {
		app.logger.Error("Failed to get decks", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	app.respondJSON(w, r, decks)
}

func (app *Application) handleStatusCounts(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	deckID := r.URL.Query().Get("deckId")

	counts, err := app.service.GetStatusCounts(r.Context(), client, lang, deckID)
	if err != nil {
		app.logger.Error("Failed to get status counts", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Return as array for backward compatibility with frontend
	app.respondJSON(w, r, []StatusCounts{*counts})
}

func (app *Application) handleTables(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
	if !ok {
		return
	}

	tables, err := app.service.GetTables(r.Context(), client)
	if err != nil {
		app.logger.Error("Failed to get tables", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	app.respondJSON(w, r, tables)
}

func (app *Application) handleDatabaseSchema(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
	if !ok {
		return
	}

	schema, err := app.service.GetDatabaseSchema(r.Context(), client)
	if err != nil {
		app.logger.Error("Failed to get database schema", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	app.respondJSON(w, r, schema)
}

func (app *Application) handleDifficultWords(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
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

	words, err := app.service.GetDifficultWords(r.Context(), client, lang, limit, deckID)
	if err != nil {
		app.logger.Error("Failed to get difficult words", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, r, words)
}

func (app *Application) handleWordStats(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
	if !ok {
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		app.writeJSONError(w, r, http.StatusBadRequest, "lang is required")
		return
	}

	deckID := r.URL.Query().Get("deckId")

	stats, err := app.service.GetWordStats(r.Context(), client, lang, deckID)
	if err != nil {
		app.logger.Error("Failed to get word stats", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, r, stats)
}

func (app *Application) handleDueStats(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
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

	stats, err := app.service.GetDueStats(r.Context(), client, lang, deckID, periodID)
	if err != nil {
		app.logger.Error("Failed to get due stats", slog.String("error", err.Error()))
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, r, stats)
}

func (app *Application) handleIntervalStats(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
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

	stats, err := app.service.GetIntervalStats(r.Context(), client, lang, deckID, percentileID)
	if err != nil {
		app.logger.Error("Failed to get interval stats", slog.String("error", err.Error()))
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, r, stats)
}

func (app *Application) handleStudyStats(w http.ResponseWriter, r *http.Request) {
	client, ok := app.requireClient(w, r)
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

	stats, err := app.service.GetStudyStats(r.Context(), client, lang, deckID, periodID)
	if err != nil {
		app.logger.Error("Failed to get study stats", slog.String("error", err.Error()))
		app.writeJSONError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	app.respondJSON(w, r, stats)
}

func (app *Application) handleStatus(w http.ResponseWriter, r *http.Request) {
	app.respondJSON(w, r, map[string]any{
		"status":    "running",
		"cache_ttl": app.cache.ttl.String(),
	})
}

func (app *Application) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.writeJSONError(w, r, http.StatusNotFound, "The requested endpoint does not exist")
		return
	}

	http.Redirect(w, r, "/docs", http.StatusFound)
}

func (app *Application) handleDocs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/docs" {
		app.writeJSONError(w, r, http.StatusNotFound, "The requested endpoint does not exist")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(docsHTML); err != nil {
		app.logger.Error("Failed to write docs response", slog.String("error", err.Error()))
	}
}

func (app *Application) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/openapi.yaml" {
		app.writeJSONError(w, r, http.StatusNotFound, "The requested endpoint does not exist")
		return
	}

	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	if _, err := w.Write(openAPISpec); err != nil {
		app.logger.Error("Failed to write OpenAPI spec", slog.String("error", err.Error()))
	}
}

func (app *Application) handleClearCache(w http.ResponseWriter, r *http.Request) {
	app.cache.Clear()
	app.logger.Info("Cache cleared")
	app.respondJSON(w, r, map[string]string{
		"status":  "success",
		"message": "Cache cleared successfully",
	})
}

package main

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

//go:embed api.html
var apiHTML []byte

func (app *Application) respondJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		app.logger.Error("Failed to encode JSON response", "error", err)
	}
}

type wordStatusRequest struct {
	Status    string           `json:"status"`
	WordText  string           `json:"wordText"`
	Secondary string           `json:"secondary"`
	Items     []WordStatusItem `json:"items"`
}

func (app *Application) handleWords(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	status := r.URL.Query().Get("status")

	words, err := app.service.GetWords(lang, status)
	if err != nil {
		if err.Error() == "invalid status: must be one of: known, learning, unknown, ignored" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, writeErr := w.Write([]byte(`{"error": "invalid status", "message": "Status must be one of: known, learning, unknown, ignored"}`))
			if writeErr != nil {
				app.logger.Error("Failed to write error response", "error", writeErr)
			}
			return
		}
		app.logger.Error("Failed to get words", "error", err, "status", status)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.respondJSON(w, words)
}

func (app *Application) handleSetWordStatus(w http.ResponseWriter, r *http.Request) {
	var req wordStatusRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		app.respondJSON(w, map[string]string{
			"error":   "invalid json",
			"message": "Request body must be valid JSON",
		})
		return
	}

	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	req.WordText = strings.TrimSpace(req.WordText)
	req.Secondary = strings.TrimSpace(req.Secondary)

	if req.Status == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		app.respondJSON(w, map[string]string{
			"error":   "missing parameters",
			"message": "status is required",
		})
		return
	}

	if _, _, ok := normalizeWordStatus(req.Status); !ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		app.respondJSON(w, map[string]string{
			"error":   "invalid status",
			"message": "Status must be one of: known, learning, tracked, ignored",
		})
		return
	}

	if len(req.Items) == 0 {
		if req.WordText == "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			app.respondJSON(w, map[string]string{
				"error":   "missing parameters",
				"message": "wordText is required",
			})
			return
		}
	}

	if len(req.Items) > 0 {
		items := make([]WordStatusItem, 0, len(req.Items))
		for _, item := range req.Items {
			wordText := strings.TrimSpace(item.WordText)
			secondary := strings.TrimSpace(item.Secondary)
			if wordText == "" {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				app.respondJSON(w, map[string]string{
					"error":   "missing parameters",
					"message": "wordText is required for each item",
				})
				return
			}
			items = append(items, WordStatusItem{
				WordText:  wordText,
				Secondary: secondary,
			})
		}

		result, err := app.service.SetWordStatusBatch(items, req.Status)
		if err != nil {
			app.logger.Error("Failed to update word status batch", "error", err, "status", req.Status, "count", len(items))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		app.respondJSON(w, result)
		return
	}

	result, err := app.service.SetWordStatus(req.WordText, req.Secondary, req.Status)
	if err != nil {
		app.logger.Error("Failed to update word status", "error", err, "status", req.Status, "wordText", req.WordText, "secondary", req.Secondary)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.respondJSON(w, result)
}

func (app *Application) handleDecks(w http.ResponseWriter, r *http.Request) {
	decks, err := app.service.GetDecks()
	if err != nil {
		app.logger.Error("Failed to get decks", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.respondJSON(w, decks)
}

func (app *Application) handleStatusCounts(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	deckID := r.URL.Query().Get("deckId")

	counts, err := app.service.GetStatusCounts(lang, deckID)
	if err != nil {
		app.logger.Error("Failed to get status counts", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return as array for backward compatibility with frontend
	app.respondJSON(w, []StatusCounts{*counts})
}

func (app *Application) handleTables(w http.ResponseWriter, r *http.Request) {
	tables, err := app.service.GetTables()
	if err != nil {
		app.logger.Error("Failed to get tables", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.respondJSON(w, tables)
}

func (app *Application) handleDatabaseSchema(w http.ResponseWriter, r *http.Request) {
	schema, err := app.service.GetDatabaseSchema()
	if err != nil {
		app.logger.Error("Failed to get database schema", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.respondJSON(w, schema)
}

func (app *Application) handleDifficultWords(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if lang == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error": "missing parameters", "message": "lang is required"}`)); err != nil {
			app.logger.Error("Failed to write error response", "error", err)
		}
		return
	}

	deckID := r.URL.Query().Get("deckId")

	words, err := app.service.GetDifficultWords(lang, limit, deckID)
	if err != nil {
		app.logger.Error("Failed to get difficult words", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	app.respondJSON(w, words)
}

func (app *Application) handleWordStats(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error": "missing parameters", "message": "lang is required"}`)); err != nil {
			app.logger.Error("Failed to write error response", "error", err)
		}
		return
	}

	deckID := r.URL.Query().Get("deckId")

	stats, err := app.service.GetWordStats(lang, deckID)
	if err != nil {
		app.logger.Error("Failed to get word stats", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	app.respondJSON(w, stats)
}

func (app *Application) handleDueStats(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error": "missing parameters", "message": "lang is required"}`)); err != nil {
			app.logger.Error("Failed to write error response", "error", err)
		}
		return
	}

	deckID := r.URL.Query().Get("deckId")
	periodId := r.URL.Query().Get("periodId")

	stats, err := app.service.GetDueStats(lang, deckID, periodId)
	if err != nil {
		app.logger.Error("Failed to get due stats", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	app.respondJSON(w, stats)
}

func (app *Application) handleIntervalStats(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error": "missing parameters", "message": "lang is required"}`)); err != nil {
			app.logger.Error("Failed to write error response", "error", err)
		}
		return
	}

	deckID := r.URL.Query().Get("deckId")
	percentileId := r.URL.Query().Get("percentileId")

	stats, err := app.service.GetIntervalStats(lang, deckID, percentileId)
	if err != nil {
		app.logger.Error("Failed to get interval stats", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	app.respondJSON(w, stats)
}

func (app *Application) handleStudyStats(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error": "missing parameters", "message": "lang is required"}`)); err != nil {
			app.logger.Error("Failed to write error response", "error", err)
		}
		return
	}

	deckID := r.URL.Query().Get("deckId")
	periodId := r.URL.Query().Get("periodId")

	stats, err := app.service.GetStudyStats(lang, deckID, periodId)
	if err != nil {
		app.logger.Error("Failed to get study stats", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	app.respondJSON(w, stats)
}

func (app *Application) handleStatus(w http.ResponseWriter, r *http.Request) {
	isAuth := app.isAuthenticated.Load()

	app.respondJSON(w, map[string]any{
		"status":        "running",
		"authenticated": isAuth,
		"cache_ttl":     app.cache.ttl.String(),
		"headless":      app.headless,
	})
}

func (app *Application) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, err := w.Write([]byte(`{"error": "endpoint not found", "message": "The requested endpoint does not exist"}`))
		if err != nil {
			app.logger.Error("Failed to write 404 response", "error", err)
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write(apiHTML)
	if err != nil {
		app.logger.Error("Failed to write root response", "error", err)
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

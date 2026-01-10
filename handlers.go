package main

import (
	"encoding/json"
	"net/http"
	"slices"
)

func (app *Application) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(app.cors) == 0 || (len(app.cors) == 1 && app.cors[0] == "*") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			origin := r.Header.Get("Origin")
			if slices.Contains(app.cors, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func (app *Application) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If no secret key is configured, allow all requests
		if app.secretKey == "" {
			next(w, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			auth := r.Header.Get("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				apiKey = auth[7:]
			}
		}

		if apiKey != app.secretKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte(`{"error": "unauthorized", "message": "Invalid or missing API key"}`))
			if err != nil {
				app.logger.Error("Failed to write unauthorized response", "error", err)
			}
			return
		}

		next(w, r)
	}
}

func (app *Application) respondJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		app.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (app *Application) handleWordsAll(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")

	words, err := app.service.GetAllWords(lang)
	if err != nil {
		app.logger.Error("Failed to get all words", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.respondJSON(w, words)
}

func (app *Application) handleWordsKnown(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")

	words, err := app.service.GetKnownWords(lang)
	if err != nil {
		app.logger.Error("Failed to get known words", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.respondJSON(w, words)
}

func (app *Application) handleWordsLearning(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")

	words, err := app.service.GetLearningWords(lang)
	if err != nil {
		app.logger.Error("Failed to get learning words", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.respondJSON(w, words)
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

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Migoku API</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 900px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        h2 { color: #555; margin-top: 30px; }
        .endpoint { background: #f4f4f4; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .method { color: #007bff; font-weight: bold; }
        code { background: #e9ecef; padding: 2px 5px; border-radius: 3px; font-size: 0.9em; }
        .response { background: #2d2d2d; color: #f8f8f2; padding: 10px; border-radius: 5px; margin-top: 5px; overflow-x: auto; }
        .desc { color: #666; margin: 5px 0; }
        .auth-info { background: #fff3cd; padding: 10px; border-radius: 5px; margin-bottom: 20px; border-left: 4px solid #ffc107; }
    </style>
</head>
<body>
    <h1>Migaku Stats API</h1>
    <p>Access your Migaku database through REST API with in-memory caching and parameterized queries for security.</p>
    
    <div class="auth-info">
        <strong>Authentication:</strong> If API_SECRET is configured, include it in requests:<br>
        <code>X-API-Key: your-secret</code> or <code>Authorization: Bearer your-secret</code>
    </div>
    
    <h2>Available Endpoints:</h2>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/words/all</code><br>
        <div class="desc">Get all words with their status (limit: 10,000)</div>
        Query: <code>?lang=ja</code> (optional, filter by language)<br>
        <div class="response">[{"dictForm":"本","secondary":"ほん","knownStatus":"KNOWN"}]</div>
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/words/known</code><br>
        <div class="desc">Get all known words</div>
        Query: <code>?lang=ja</code> (optional, filter by language)<br>
        <div class="response">[{"dictForm":"本","secondary":"ほん"}]</div>
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/words/learning</code><br>
        <div class="desc">Get all learning words</div>
        Query: <code>?lang=ja</code> (optional, filter by language)<br>
        <div class="response">[{"dictForm":"食べる","secondary":"たべる"}]</div>
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/decks</code><br>
        <div class="desc">Get all active decks</div>
        <div class="response">[{"id":1,"name":"Core 2k"}]</div>
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/status/counts</code><br>
        <div class="desc">Get aggregated word status counts</div>
        Query: <code>?deckId=123&lang=ja</code> (both optional)<br>
        <div class="response">[{"known_count":606,"learning_count":79,"unknown_count":2551,"ignored_count":4}]</div>
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/tables</code><br>
        <div class="desc">List all database tables</div>
        <div class="response">[{"name":"WordList"},{"name":"deck"}]</div>
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/status</code><br>
        <div class="desc">Get server status and configuration</div>
        <div class="response">{"status":"running","authenticated":true,"cache_ttl":"5m0s","headless":true}</div>
    </div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write([]byte(html))
	if err != nil {
		app.logger.Error("Failed to write root response", "error", err)
	}
}

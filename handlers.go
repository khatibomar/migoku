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
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		app.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (app *Application) handleWordsAll(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	cacheKey := "words:all:"
	if lang == "" {
		cacheKey += "all"
	} else {
		cacheKey += lang
	}

	if cached, ok := app.cache.Get(cacheKey); ok {
		app.logger.Info("Serving from cache", "key", cacheKey)
		app.respondJSON(w, cached)
		return
	}

	var data []map[string]any
	var err error

	if lang != "" {
		data, err = app.runQuery("SELECT dictForm, secondary, knownStatus FROM WordList WHERE language = ? AND del = 0 LIMIT 10000;", lang)
	} else {
		data, err = app.runQuery("SELECT dictForm, secondary, knownStatus FROM WordList WHERE del = 0 LIMIT 10000;")
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.cache.Set(cacheKey, data)
	app.respondJSON(w, data)
}

func (app *Application) handleWordsKnown(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	cacheKey := "words:known:"
	if lang == "" {
		cacheKey += "all"
	} else {
		cacheKey += lang
	}

	if cached, ok := app.cache.Get(cacheKey); ok {
		app.logger.Info("Serving from cache", "key", cacheKey)
		app.respondJSON(w, cached)
		return
	}

	var data []map[string]any
	var err error

	if lang != "" {
		data, err = app.runQuery("SELECT dictForm, secondary FROM WordList WHERE knownStatus = 'KNOWN' AND language = ? AND del = 0;", lang)
	} else {
		data, err = app.runQuery("SELECT dictForm, secondary FROM WordList WHERE knownStatus = 'KNOWN' AND del = 0;")
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.cache.Set(cacheKey, data)
	app.respondJSON(w, data)
}

func (app *Application) handleWordsLearning(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	cacheKey := "words:learning:"
	if lang == "" {
		cacheKey += "all"
	} else {
		cacheKey += lang
	}

	if cached, ok := app.cache.Get(cacheKey); ok {
		app.logger.Info("Serving from cache", "key", cacheKey)
		app.respondJSON(w, cached)
		return
	}

	var data []map[string]any
	var err error

	if lang != "" {
		data, err = app.runQuery("SELECT dictForm, secondary FROM WordList WHERE knownStatus = 'LEARNING' AND language = ? AND del = 0;", lang)
	} else {
		data, err = app.runQuery("SELECT dictForm, secondary FROM WordList WHERE knownStatus = 'LEARNING' AND del = 0;")
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.cache.Set(cacheKey, data)
	app.respondJSON(w, data)
}

func (app *Application) handleDecks(w http.ResponseWriter, r *http.Request) {
	cacheKey := "decks"
	if cached, ok := app.cache.Get(cacheKey); ok {
		app.logger.Info("Serving from cache", "key", cacheKey)
		app.respondJSON(w, cached)
		return
	}

	query := "SELECT id, name FROM deck WHERE del = 0 ORDER BY name;"
	data, err := app.runQuery(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.cache.Set(cacheKey, data)
	app.respondJSON(w, data)
}

func (app *Application) handleStatusCounts(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	deckID := r.URL.Query().Get("deckId")

	var cacheKey string

	if deckID == "" {
		cacheKey = "status:counts:all:"
		if lang == "" {
			cacheKey += "all"
		} else {
			cacheKey += lang
		}
	} else {
		cacheKey = "status:counts:deck:" + deckID + ":"
		if lang == "" {
			cacheKey += "all"
		} else {
			cacheKey += lang
		}
	}

	if cached, ok := app.cache.Get(cacheKey); ok {
		app.logger.Info("Serving from cache", "key", cacheKey)
		app.respondJSON(w, cached)
		return
	}

	var data []map[string]any
	var err error

	if deckID == "" {
		if lang != "" {
			data, err = app.runQuery(`SELECT
				SUM(CASE WHEN knownStatus = 'KNOWN' THEN 1 ELSE 0 END) as known_count,
				SUM(CASE WHEN knownStatus = 'LEARNING' THEN 1 ELSE 0 END) as learning_count,
				SUM(CASE WHEN knownStatus = 'UNKNOWN' THEN 1 ELSE 0 END) as unknown_count,
				SUM(CASE WHEN knownStatus = 'IGNORED' THEN 1 ELSE 0 END) as ignored_count
			FROM WordList
			WHERE language = ? AND del = 0;`, lang)
		} else {
			data, err = app.runQuery(`SELECT
				SUM(CASE WHEN knownStatus = 'KNOWN' THEN 1 ELSE 0 END) as known_count,
				SUM(CASE WHEN knownStatus = 'LEARNING' THEN 1 ELSE 0 END) as learning_count,
				SUM(CASE WHEN knownStatus = 'UNKNOWN' THEN 1 ELSE 0 END) as unknown_count,
				SUM(CASE WHEN knownStatus = 'IGNORED' THEN 1 ELSE 0 END) as ignored_count
			FROM WordList
			WHERE del = 0;`)
		}
	} else {
		if lang != "" {
			data, err = app.runQuery(`SELECT
				SUM(CASE WHEN knownStatus = 'KNOWN' THEN 1 ELSE 0 END) as known_count,
				SUM(CASE WHEN knownStatus = 'LEARNING' THEN 1 ELSE 0 END) as learning_count,
				SUM(CASE WHEN knownStatus = 'UNKNOWN' THEN 1 ELSE 0 END) as unknown_count,
				SUM(CASE WHEN knownStatus = 'IGNORED' THEN 1 ELSE 0 END) as ignored_count
			FROM WordList
			WHERE deckId = ? AND language = ? AND del = 0;`, deckID, lang)
		} else {
			data, err = app.runQuery(`SELECT
				SUM(CASE WHEN knownStatus = 'KNOWN' THEN 1 ELSE 0 END) as known_count,
				SUM(CASE WHEN knownStatus = 'LEARNING' THEN 1 ELSE 0 END) as learning_count,
				SUM(CASE WHEN knownStatus = 'UNKNOWN' THEN 1 ELSE 0 END) as unknown_count,
				SUM(CASE WHEN knownStatus = 'IGNORED' THEN 1 ELSE 0 END) as ignored_count
			FROM WordList
			WHERE deckId = ? AND del = 0;`, deckID)
		}
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.cache.Set(cacheKey, data)
	app.respondJSON(w, data)
}

func (app *Application) handleTables(w http.ResponseWriter, r *http.Request) {
	cacheKey := "tables"
	if cached, ok := app.cache.Get(cacheKey); ok {
		app.logger.Info("Serving from cache", "key", cacheKey)
		app.respondJSON(w, cached)
		return
	}

	query := "SELECT name FROM sqlite_master WHERE type='table';"
	data, err := app.runQuery(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.cache.Set(cacheKey, data)
	app.respondJSON(w, data)
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
    <title>Migaku Stats API</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        .endpoint { background: #f4f4f4; padding: 10px; margin: 10px 0; border-radius: 5px; }
        .method { color: #007bff; font-weight: bold; }
        code { background: #e9ecef; padding: 2px 5px; border-radius: 3px; }
    </style>
</head>
<body>
    <h1>Migaku Stats API</h1>
    <p>Access Migaku database through REST API with in-memory caching</p>
    
    <h2>Available Endpoints:</h2>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/words/all</code><br>
        Get all words with their status<br>
        Query: <code>?lang=ja</code> (optional, filter by language)
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/words/known</code><br>
        Get all known words<br>
        Query: <code>?lang=ja</code> (optional, filter by language)
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/words/learning</code><br>
        Get all learning words<br>
        Query: <code>?lang=ja</code> (optional, filter by language)
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/decks</code><br>
        Get all decks
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/status/counts</code><br>
        Get status count breakdown (all decks or filtered by deck and language)<br>
        Query: <code>?deckId=123&lang=ja</code> (both optional)
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/v1/tables</code><br>
        List all database tables
    </div>
    
    <div class="endpoint">
        <span class="method">GET</span> <code>/api/status</code><br>
        Get server status and configuration
    </div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	_, err := w.Write([]byte(html))
	if err != nil {
		app.logger.Error("Failed to write root response", "error", err)
	}
}

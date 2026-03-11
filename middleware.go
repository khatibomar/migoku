package main

import (
	"net/http"
	"slices"
)

// corsHandler wraps the top-level mux so that OPTIONS preflight requests
// are answered with the correct headers before Go 1.22's method-constrained
// routes ("GET /foo") can reject them with 405.
func (app *Application) corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(app.cors) == 0 || (len(app.cors) == 1 && app.cors[0] == "*") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			origin := r.Header.Get("Origin")
			if slices.Contains(app.cors, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Key, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

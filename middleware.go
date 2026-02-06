package main

import (
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
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

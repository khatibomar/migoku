package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type clientContextKey int

const requestClientKey clientContextKey = iota

func clientFromContext(ctx context.Context) (*MigakuClient, bool) {
	client, ok := ctx.Value(requestClientKey).(*MigakuClient)
	if !ok || client == nil {
		return nil, false
	}
	return client, true
}

func (app *Application) deriveAPIKey(email, password string) (string, error) {
	if app.secretKey == "" {
		return "", errors.New("API_SECRET not configured")
	}

	mac := hmac.New(sha256.New, []byte(app.secretKey))
	_, _ = io.WriteString(mac, email)
	_, _ = mac.Write([]byte{0})
	_, _ = io.WriteString(mac, password)

	return hex.EncodeToString(mac.Sum(nil)), nil
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (app *Application) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req loginRequest
	if err := decoder.Decode(&req); err != nil {
		app.writeJSONError(w, r, http.StatusBadRequest, "Invalid JSON")
		return
	}

	email := strings.TrimSpace(req.Email)
	password := strings.TrimSpace(req.Password)
	if email == "" || password == "" {
		app.writeJSONError(w, r, http.StatusBadRequest, "Missing required fields")
		return
	}

	apiKey, err := app.deriveAPIKey(email, password)
	if err != nil {
		app.logger.Error("API key derivation failed", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, "Server misconfigured")
		return
	}
	if _, exists := app.accounts[apiKey]; exists {
		if err := encode(w, r, http.StatusOK, map[string]string{
			"api_key": apiKey,
			"message": "Already, logged in",
		}); err != nil {
			app.logger.Error("Failed to encode JSON response", "error", err)
		}
		return
	}

	db, err := NewMigakuClient(
		r.Context(),
		app.logger,
		email,
		password,
		app.cache.ttl,
	)
	if err != nil {
		app.logger.Error("Failed to initialize client", "error", err)
		app.writeJSONError(w, r, http.StatusInternalServerError, "Failed to initialize client")
		return
	}

	app.accounts[apiKey] = db
	if err := encode(w, r, http.StatusOK, map[string]string{
		"api_key": apiKey,
		"message": "Login successful",
	}); err != nil {
		app.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (app *Application) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	apiKey := r.Header.Get("X-Api-Key")
	if apiKey == "" {
		app.writeJSONError(w, r, http.StatusUnauthorized, "Missing API key")
		return
	}

	db, exists := app.accounts[apiKey]
	if !exists || db == nil {
		app.writeJSONError(w, r, http.StatusUnauthorized, "Not logged in")
		return
	}

	if db.cleanUp != nil {
		db.cleanUp()
	}

	delete(app.accounts, apiKey)
	if err := encode(w, r, http.StatusOK, map[string]string{
		"message": "Logout successful",
	}); err != nil {
		app.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (app *Application) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-Api-Key")
		if apiKey == "" {
			app.writeJSONError(w, r, http.StatusUnauthorized, "Missing API key")
			return
		}

		client, exists := app.accounts[apiKey]
		if !exists || client == nil {
			app.writeJSONError(w, r, http.StatusUnauthorized, "Invalid or expired API key")
			return
		}

		ctx := context.WithValue(r.Context(), requestClientKey, client)
		next(w, r.WithContext(ctx))
	}
}

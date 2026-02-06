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

type browserContextKey int

const requestBrowserKey browserContextKey = iota

func browserFromContext(ctx context.Context) (*Browser, bool) {
	browser, ok := ctx.Value(requestBrowserKey).(*Browser)
	if !ok || browser == nil {
		return nil, false
	}
	return browser, true
}

func (app *Application) deriveAPIKey(email, password, language string) (string, error) {
	if app.secretKey == "" {
		return "", errors.New("API_SECRET not configured")
	}

	mac := hmac.New(sha256.New, []byte(app.secretKey))
	_, _ = io.WriteString(mac, email)
	_, _ = mac.Write([]byte{0})
	_, _ = io.WriteString(mac, password)
	_, _ = mac.Write([]byte{0})
	_, _ = io.WriteString(mac, language)

	return hex.EncodeToString(mac.Sum(nil)), nil
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Language string `json:"language"`
}

func (app *Application) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req loginRequest
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(req.Email)
	password := strings.TrimSpace(req.Password)
	language := strings.TrimSpace(req.Language)
	if email == "" || password == "" || language == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	apiKey, err := app.deriveAPIKey(email, password, language)
	if err != nil {
		app.logger.Error("API key derivation failed", "error", err)
		http.Error(w, "Server misconfigured", http.StatusInternalServerError)
		return
	}
	if _, exists := app.accounts[apiKey]; exists {
		app.respondJSON(w, map[string]string{
			"api_key": apiKey,
			"message": "Already, logged in",
		})
		return
	}

	browser, err := NewBrowser(app.logger, email, password, language, app.headless)
	if err != nil {
		app.logger.Error("Failed to initialize browser", "error", err)
		http.Error(w, "Failed to initialize browser", http.StatusInternalServerError)
		return
	}

	app.accounts[apiKey] = browser

	w.WriteHeader(http.StatusOK)
	app.respondJSON(w, map[string]string{
		"api_key": apiKey,
		"message": "Login successful",
	})
}

func (app *Application) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	apiKey := r.Header.Get("X-Api-Key")
	if apiKey == "" {
		http.Error(w, "Missing API key", http.StatusUnauthorized)
		return
	}

	browser, exists := app.accounts[apiKey]
	if !exists || browser == nil {
		http.Error(w, "Not logged in", http.StatusUnauthorized)
		return
	}

	if browser.cleanUp != nil {
		browser.cleanUp()
	}

	delete(app.accounts, apiKey)

	w.WriteHeader(http.StatusOK)
	app.respondJSON(w, map[string]string{
		"message": "Logout successful",
	})
}

func (app *Application) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-Api-Key")
		if apiKey == "" {
			http.Error(w, "Missing API key", http.StatusUnauthorized)
			return
		}

		browser, exists := app.accounts[apiKey]
		if !exists || browser == nil {
			http.Error(w, "Invalid or expired API key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), requestBrowserKey, browser)
		next(w, r.WithContext(ctx))
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// Validator is an object that can be validated.
type Validator interface {
	// Valid checks the object and returns any
	// problems. If len(problems) == 0 then
	// the object is valid.
	Valid(ctx context.Context) (problems map[string]string)
}

func encode[T any](w http.ResponseWriter, _ *http.Request, status int, v T) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func (app *Application) writeJSONError(w http.ResponseWriter, r *http.Request, status int, message string) {
	errText := http.StatusText(status)
	app.logger.Error("HTTP error", slog.Int("status", status), slog.String("error", errText), slog.String("message", message))
	if status == http.StatusInternalServerError {
		message = "Internal server error"
	}
	if err := encode(w, r, status, errorResponse{Error: errText, Message: message}); err != nil {
		app.logger.Error("Failed to encode JSON error response", slog.String("error", err.Error()))
	}
}

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	//nolint:gosec // Public API key used by Migaku clients.
	migakuAPIKey              = "AIzaSyDZvwYKYTsQoZkf3oKsfIQ4ykuy2GZAiH8"
	migakuSyncServerURL       = "https://core-server-mohegkboza-uc.a.run.app"
	migakuPresignedURLService = "https://srs-db-presigned-url-service-api.migaku.com/db-force-sync-download-url"
)

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

type FirebaseAuthToken struct {
	mu           sync.Mutex
	refreshToken string
	authToken    string
	expiresAt    time.Time
}

type MigakuSession struct {
	auth *FirebaseAuthToken
}

type MigakuWord struct {
	DictForm         string `json:"dictForm"`
	Secondary        string `json:"secondary"`
	PartOfSpeech     string `json:"partOfSpeech"`
	Language         string `json:"language"`
	Mod              int64  `json:"mod"`
	ServerMod        int64  `json:"serverMod"`
	Del              int    `json:"del"`
	KnownStatus      string `json:"knownStatus"`
	HasCard          bool   `json:"hasCard"`
	Tracked          bool   `json:"tracked"`
	Created          *int64 `json:"created,omitempty"`
	IsModern         *int   `json:"isModern,omitempty"`
	ServerVersion    *int   `json:"serverVersion,omitempty"`
	IsPendingEnqueue *int   `json:"isPendingEnqueue,omitempty"`
	IsPendingApply   *int   `json:"isPendingApply,omitempty"`
}

type migakuSyncPayload struct {
	Decks             []any            `json:"decks"`
	CardTypes         []any            `json:"cardTypes"`
	Cards             []any            `json:"cards"`
	CardWordRelations []any            `json:"cardWordRelations"`
	Vacations         []any            `json:"vacations"`
	Reviews           []any            `json:"reviews"`
	Words             []map[string]any `json:"words"`
	Config            any              `json:"config"`
	KeyValue          []any            `json:"keyValue"`
	LearningMaterials []any            `json:"learningMaterials"`
	Lessons           []any            `json:"lessons"`
	ReviewHistory     []any            `json:"reviewHistory"`
}

func NewMigakuSession(auth *FirebaseAuthToken) *MigakuSession {
	return &MigakuSession{
		auth: auth,
	}
}

func TryFromEmailPassword(ctx context.Context, email, password string) (*FirebaseAuthToken, error) {
	if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
		return nil, errors.New("email and password are required")
	}

	url := fmt.Sprintf("https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=%s", migakuAPIKey)
	payload := map[string]any{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	}

	respBody, status, err := doJSONRequest(ctx, http.MethodPost, url, payload, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("login failed (%d): %s", status, string(respBody))
	}

	var res struct {
		RefreshToken string `json:"refreshToken"`
		IDToken      string `json:"idToken"`
		ExpiresIn    string `json:"expiresIn"`
	}
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("failed to parse login response: %w", err)
	}

	expiresInSec, _ := strconv.Atoi(res.ExpiresIn)
	if expiresInSec <= 0 {
		expiresInSec = 3600
	}

	return &FirebaseAuthToken{
		refreshToken: res.RefreshToken,
		authToken:    res.IDToken,
		expiresAt:    time.Now().Add(time.Duration(expiresInSec-5) * time.Second),
	}, nil
}

func (t *FirebaseAuthToken) get(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if time.Now().Before(t.expiresAt) && t.authToken != "" {
		return t.authToken, nil
	}

	return t.refreshLocked(ctx)
}

func (t *FirebaseAuthToken) refresh(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.refreshLocked(ctx)
}

func (t *FirebaseAuthToken) refreshLocked(ctx context.Context) (string, error) {
	url := fmt.Sprintf("https://securetoken.googleapis.com/v1/token?key=%s", migakuAPIKey)
	payload := map[string]any{
		"grant_type":    "refresh_token",
		"refresh_token": t.refreshToken,
	}

	respBody, status, err := doJSONRequest(ctx, http.MethodPost, url, payload, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("failed to refresh token (%d): %s", status, string(respBody))
	}

	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &res); err != nil {
		return "", fmt.Errorf("failed to parse refresh response: %w", err)
	}

	expiresInSec, _ := strconv.Atoi(res.ExpiresIn)
	if expiresInSec <= 0 {
		expiresInSec = 3600
	}

	t.authToken = res.AccessToken
	t.expiresAt = time.Now().Add(time.Duration(expiresInSec-5) * time.Second)
	return t.authToken, nil
}

func (s *MigakuSession) ForceDownloadSRSDB(ctx context.Context) ([]byte, error) {
	if s.auth == nil {
		return nil, errors.New("missing auth token")
	}

	slog.Default().Debug("Requesting SRS database download URL")

	respBody, status, err := s.doAuthorizedJSONRequest(ctx, http.MethodGet, migakuPresignedURLService, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch download url (%d): %s", status, string(respBody))
	}

	downloadURL := strings.TrimSpace(string(respBody))
	if downloadURL == "" {
		return nil, errors.New("empty download url")
	}

	slog.Default().Debug("Downloading SRS database")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to download database (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	compressed, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	slog.Default().Debug("Downloaded compressed database", "bytes", len(compressed))

	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	slog.Default().Debug("Decompressed database", "bytes", len(data))
	return data, nil
}

func (s *MigakuSession) PushSync(ctx context.Context, words []map[string]any) error {
	if s.auth == nil {
		return errors.New("missing auth token")
	}

	if len(words) == 0 {
		return errors.New("no words to sync")
	}

	slog.Default().Debug("Pushing word status updates", "count", len(words))

	payload := migakuSyncPayload{
		Decks:             []any{},
		CardTypes:         []any{},
		Cards:             []any{},
		CardWordRelations: []any{},
		Vacations:         []any{},
		Reviews:           []any{},
		Words:             words,
		Config:            nil,
		KeyValue:          []any{},
		LearningMaterials: []any{},
		Lessons:           []any{},
		ReviewHistory:     []any{},
	}

	url := fmt.Sprintf("%s/sync?clientSessionId=%d", migakuSyncServerURL, time.Now().UnixMilli())

	respBody, status, err := s.doAuthorizedJSONRequest(ctx, http.MethodPut, url, payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("push failed (%d): %s", status, string(respBody))
	}

	slog.Default().Debug("Push sync completed", "status", status)
	return nil
}

func (s *MigakuSession) doAuthorizedJSONRequest(ctx context.Context, method, url string, payload any) ([]byte, int, error) {
	authToken, err := s.auth.get(ctx)
	if err != nil {
		return nil, 0, err
	}

	headers := map[string]string{
		"Authorization": "Bearer " + authToken,
	}

	respBody, status, err := doJSONRequest(ctx, method, url, payload, headers)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return respBody, status, nil
	}

	slog.Default().Debug("Auth token expired, refreshing")

	authToken, err = s.auth.refresh(ctx)
	if err != nil {
		return respBody, status, err
	}
	headers["Authorization"] = "Bearer " + authToken

	return doJSONRequest(ctx, method, url, payload, headers)
}

func doJSONRequest(
	ctx context.Context,
	method, url string,
	payload any,
	headers map[string]string,
) ([]byte, int, error) {
	start := time.Now()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, 0, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		slog.Default().Debug("HTTP request failed", "method", method, "url", url, "error", err)
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	slog.Default().Debug(
		"HTTP request completed",
		"method", method,
		"url", url,
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return respBody, resp.StatusCode, nil
}

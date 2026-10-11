package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	counts := map[string]langCount{
		"ja": {known: 1200, learning: 300},
		"en": {known: 5500, learning: 50},
	}

	tests := []struct {
		name        string
		tmpl        string
		userName    string
		defaultLang string
		expected    string
	}{
		{
			name:        "explicit lang token",
			tmpl:        "{name} | {ja}{emoji}{lang} {known}",
			userName:    "Ayn",
			defaultLang: "ja",
			expected:    "Ayn | 🇯🇵JA 1.2k",
		},
		{
			name:        "implicit default lang",
			tmpl:        "{name} | {emoji}{lang} {known}",
			userName:    "Ayn",
			defaultLang: "ja",
			expected:    "Ayn | 🇯🇵JA 1.2k",
		},
		{
			name:        "multi language tokens",
			tmpl:        "{ja}{emoji} {known} | {en}{emoji} {known}",
			userName:    "Ayn",
			defaultLang: "ja",
			expected:    "🇯🇵 1.2k | 🇺🇸 5.5k",
		},
		{
			name:        "learning and total",
			tmpl:        "{ja}Total: {total} ({learning} learning)",
			userName:    "Ayn",
			defaultLang: "ja",
			expected:    "Total: 1.5k (300 learning)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := renderTemplate(tc.tmpl, tc.userName, tc.defaultLang, counts)
			if res != tc.expected {
				t.Errorf("got %q, want %q", res, tc.expected)
			}
		})
	}
}

func TestGetEffectiveLangs(t *testing.T) {
	langs := getEffectiveLangs("{name} | {emoji}{lang} {known}", "ja")
	if len(langs) != 1 || langs[0] != "ja" {
		t.Errorf("got %v, want ['ja']", langs)
	}

	langs2 := getEffectiveLangs("{name} | {zh}{emoji} {known} {en}{emoji} {known}", "ja")
	if len(langs2) != 2 || langs2[0] != "zh" || langs2[1] != "en" {
		t.Errorf("got %v, want ['zh', 'en']", langs2)
	}
}

func TestLoadConfigAndEnv(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "test-token-123")
	t.Setenv("MIGOKU_API_KEY", "api-key-xyz")
	t.Setenv("GUILD_ID", "987654321")
	t.Setenv("INTERVAL", "30m")

	cfg, _ := loadConfigAndEnv("/non/existent/path/config.json")
	if cfg.DiscordToken != "test-token-123" {
		t.Errorf("expected DiscordToken test-token-123, got %q", cfg.DiscordToken)
	}
	if cfg.MigokuKey != "api-key-xyz" {
		t.Errorf("expected MigokuKey api-key-xyz, got %q", cfg.MigokuKey)
	}
	if cfg.GuildID != "987654321" {
		t.Errorf("expected GuildID 987654321, got %q", cfg.GuildID)
	}
	if cfg.Interval != "30m" {
		t.Errorf("expected Interval 30m, got %q", cfg.Interval)
	}
}

func TestFetchWordCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "my-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v1/status/counts" || r.URL.Query().Get("lang") != "ja" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]StatusCounts{
			{KnownCount: 1500, LearningCount: 250},
		})
	}))
	defer server.Close()

	known, learning, err := fetchWordCounts(server.URL, "my-key", "ja")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if known != 1500 || learning != 250 {
		t.Errorf("got known=%d, learning=%d; want 1500, 250", known, learning)
	}
}

func TestValidateConfigMissingCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &Config{
		MigokuURL: "http://localhost:8080",
	}
	err := validateAndResolveConfig(cfg)
	if err == nil {
		t.Fatal("expected error when DISCORD_TOKEN is missing")
	}

	cfg.DiscordToken = "token"
	err = validateAndResolveConfig(cfg)
	if err == nil {
		t.Fatal("expected error when MIGOKU_API_KEY is missing")
	}
}

func TestRuneTruncation(t *testing.T) {
	longTemplate := "{name} | 🇯🇵 123456789012345678901234567890"
	counts := map[string]langCount{
		"ja": {known: 100, learning: 20},
	}
	rendered := renderTemplate(longTemplate, "SuperLongDiscordUsername", "ja", counts)
	runes := []rune(rendered)
	if len(runes) > 32 {
		truncated := string(runes[:32])
		if len([]rune(truncated)) != 32 {
			t.Errorf("expected 32 runes, got %d", len([]rune(truncated)))
		}
	}
}

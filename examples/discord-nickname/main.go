package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var langEmoji = map[string]string{
	"ja": "\U0001F1EF\U0001F1F5",
	"zh": "\U0001F1E8\U0001F1F3",
	"es": "\U0001F1EA\U0001F1F8",
	"fr": "\U0001F1EB\U0001F1F7",
	"de": "\U0001F1E9\U0001F1EA",
	"ko": "\U0001F1F0\U0001F1F7",
	"it": "\U0001F1EE\U0001F1F9",
	"pt": "\U0001F1E7\U0001F1F7",
	"ru": "\U0001F1F7\U0001F1FA",
	"ar": "\U0001F1E6\U0001F1F7",
	"vi": "\U0001F1FB\U0001F1F3",
	"th": "\U0001F1F9\U0001F1ED",
	"nl": "\U0001F1F3\U0001F1F1",
	"pl": "\U0001F1F5\U0001F1F1",
	"tr": "\U0001F1F9\U0001F1F7",
}

type Config struct {
	MigokuURL    string `json:"migoku_url"`
	MigokuKey    string `json:"migoku_api_key"`
	DiscordToken string `json:"discord_token"`
	DiscordName  string `json:"discord_name"`
	Lang         string `json:"lang"`
	GuildID      string `json:"guild_id"`
	NickTemplate string `json:"nick_template"`
	Interval     string `json:"interval"`
}

type StatusCounts struct {
	KnownCount    int `json:"known_count"`
	LearningCount int `json:"learning_count"`
	UnknownCount  int `json:"unknown_count"`
	IgnoredCount  int `json:"ignored_count"`
}

type DiscordGuild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DiscordUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
}

func main() {
	configPath := flag.String("config", "", "path to config file")
	interactive := flag.Bool("i", false, "run interactively to set up")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil && !errors.Is(err, errConfigNotFound) {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *interactive || cfg == nil {
		cfg = runSetup(*configPath)
		if cfg == nil {
			return
		}
	}

	if cfg == nil {
		log.Fatal("No configuration found. Run with -i to set up interactively.")
	}

	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		interval = 1 * time.Hour
	}

	log.Printf("Starting Discord nickname updater")
	log.Printf("  Migoku:    %s", cfg.MigokuURL)
	log.Printf("  Language:  %s", cfg.Lang)
	log.Printf("  Guild:     %s", cfg.GuildID)
	log.Printf("  Interval:  %s", interval)

	if err := updateNickname(cfg); err != nil {
		log.Printf("Initial update failed: %v", err)
	}

	ticker := time.NewTicker(interval)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			log.Println("Refreshing nickname...")
			if err := updateNickname(cfg); err != nil {
				log.Printf("Update failed: %v", err)
			}
		case <-sigCh:
			log.Println("Shutting down...")
			ticker.Stop()
			return
		}
	}
}

func runSetup(configPath string) *Config {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== Discord Nickname Updater Setup ===")

	if configPath == "" {
		configPath = defaultConfigPath()
	}

	cfg := &Config{
		MigokuURL:    "http://localhost:8080",
		NickTemplate: "{name} | {emoji}{lang} {known}",
		Interval:     "1h",
	}

	fmt.Printf("Migoku URL [%s]: ", cfg.MigokuURL)
	if input := readLine(reader); input != "" {
		cfg.MigokuURL = strings.TrimRight(input, "/")
	}

	fmt.Print("Migoku API Key: ")
	cfg.MigokuKey = readLine(reader)
	if cfg.MigokuKey == "" {
		log.Fatal("API key is required")
	}

	fmt.Print("Discord User Token: ")
	cfg.DiscordToken = readLine(reader)
	if cfg.DiscordToken == "" {
		log.Fatal("Discord token is required")
	}

	user, err := getCurrentUser(cfg.DiscordToken)
	if err != nil {
		log.Fatalf("Failed to verify Discord token: %v", err)
	}
	cfg.DiscordName = user.GlobalName
	if cfg.DiscordName == "" {
		cfg.DiscordName = user.Username
	}
	fmt.Printf("Authenticated as: %s (%s)\n", cfg.DiscordName, user.ID)

	guilds, err := listUserGuilds(cfg.DiscordToken)
	if err != nil {
		log.Fatalf("Failed to list Discord guilds: %v", err)
	}
	if len(guilds) == 0 {
		log.Fatal("No Discord servers found.")
	}

	fmt.Println("\nAvailable Discord servers:")
	for i, g := range guilds {
		fmt.Printf("  [%d] %s (%s)\n", i+1, g.Name, g.ID)
	}
	fmt.Print("Pick a server [1]: ")
	choiceStr := readLine(reader)
	choice := 1
	if choiceStr != "" {
		if c, err := strconv.Atoi(choiceStr); err == nil && c >= 1 && c <= len(guilds) {
			choice = c
		}
	}
	cfg.GuildID = guilds[choice-1].ID
	fmt.Printf("Selected: %s\n", guilds[choice-1].Name)

	fmt.Print("Language code (ja, zh, es, fr, de, ko, etc.) [ja]: ")
	lang := readLine(reader)
	if lang == "" {
		lang = "ja"
	}
	cfg.Lang = strings.ToLower(strings.TrimSpace(lang))

	fmt.Printf("Nickname template [%s]:\n", cfg.NickTemplate)
	fmt.Println("  Available placeholders: {name} {emoji} {lang} {known} {learning} {total}")
	if input := readLine(reader); input != "" {
		cfg.NickTemplate = input
	}

	fmt.Printf("Update interval (e.g. 30m, 1h, 6h) [%s]: ", cfg.Interval)
	if input := readLine(reader); input != "" {
		cfg.Interval = input
	}

	saveConfig(cfg, configPath)
	fmt.Printf("\nConfiguration saved to %s\n", configPath)
	fmt.Println("Starting updater...")

	return cfg
}

func updateNickname(cfg *Config) error {
	known, learning, err := fetchWordCounts(cfg.MigokuURL, cfg.MigokuKey, cfg.Lang)
	if err != nil {
		return fmt.Errorf("fetch word counts: %w", err)
	}

	currentNick := cfg.DiscordName

	total := known + learning
	emoji := langEmoji[cfg.Lang]

	newNick := cfg.NickTemplate
	newNick = strings.ReplaceAll(newNick, "{name}", currentNick)
	newNick = strings.ReplaceAll(newNick, "{emoji}", emoji)
	newNick = strings.ReplaceAll(newNick, "{lang}", strings.ToUpper(cfg.Lang))
	newNick = strings.ReplaceAll(newNick, "{known}", formatInt(known))
	newNick = strings.ReplaceAll(newNick, "{learning}", formatInt(learning))
	newNick = strings.ReplaceAll(newNick, "{total}", formatInt(total))

	if err := setDiscordNickname(cfg.DiscordToken, cfg.GuildID, newNick); err != nil {
		return fmt.Errorf("set discord nickname: %w", err)
	}

	log.Printf("Nickname updated: %s (known=%d learning=%d total=%d)", newNick, known, learning, total)
	return nil
}

var errConfigNotFound = errors.New("config not found")

func fetchWordCounts(migokuURL, apiKey, lang string) (known, learning int, err error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, migokuURL+"/api/v1/status/counts?lang="+lang, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("X-Api-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("migoku returned HTTP %d", resp.StatusCode)
	}

	var counts []StatusCounts
	if err := json.NewDecoder(resp.Body).Decode(&counts); err != nil {
		return 0, 0, err
	}

	if len(counts) > 0 {
		return counts[0].KnownCount, counts[0].LearningCount, nil
	}
	return 0, 0, nil
}

func getCurrentUser(token string) (*DiscordUser, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord API returned HTTP %d", resp.StatusCode)
	}

	var user DiscordUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func listUserGuilds(token string) ([]DiscordGuild, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://discord.com/api/v10/users/@me/guilds", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			return nil, fmt.Errorf("discord API returned HTTP %d: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("discord API returned HTTP %d: %s", resp.StatusCode, buf.String())
	}

	var guilds []DiscordGuild
	if err := json.NewDecoder(resp.Body).Decode(&guilds); err != nil {
		return nil, err
	}
	return guilds, nil
}

func setDiscordNickname(token, guildID, nick string) error {
	endpoint := fmt.Sprintf("https://discord.com/api/v10/guilds/%s/members/@me", guildID)

	body := map[string]string{"nick": nick}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return fmt.Errorf("discord API returned HTTP %d: %w", resp.StatusCode, err)
	}
	return fmt.Errorf("discord API returned HTTP %d: %s", resp.StatusCode, buf.String())
}

func formatInt(n int) string {
	s := strconv.Itoa(n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func readLine(reader *bufio.Reader) string {
	text, _ := reader.ReadString('\n')
	return strings.TrimRight(text, "\r\n")
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.Getenv("HOME")
	}
	return filepath.Join(dir, "migoku-discord-nickname.json")
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errConfigNotFound
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfig(cfg *Config, path string) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatalf("Failed to create config directory: %v", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		log.Fatalf("Failed to write config: %v", err)
	}
}

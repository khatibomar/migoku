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
	"regexp"
	"runtime"
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
	"en": "\U0001F1FA\U0001F1F8",
}

type Config struct {
	MigokuURL    string `json:"migoku_url"`
	MigokuKey    string `json:"migoku_api_key"`
	DiscordToken string `json:"discord_token"`
	DiscordName  string `json:"discord_name"`
	GuildID      string `json:"guild_id"`
	GuildName    string `json:"guild_name,omitempty"`
	NickTemplate string `json:"nick_template"`
	DefaultLang  string `json:"default_lang,omitempty"`
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

var errConfigNotFound = errors.New("config not found")

func main() {
	configPath := flag.String("config", "", "path to config file")
	interactive := flag.Bool("i", false, "run interactively to set up")
	flag.Parse()

	cfg, _ := loadConfigAndEnv(*configPath)

	if *interactive {
		cfg = runSetup(cfg, *configPath)
		if cfg == nil {
			return
		}
	} else {
		if err := validateAndResolveConfig(cfg); err != nil {
			log.Fatalf("Configuration error: %v", err)
		}
	}

	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		log.Printf("Invalid interval %q, defaulting to 1h: %v", cfg.Interval, err)
		interval = 1 * time.Hour
	}

	log.Printf("Starting Discord nickname updater")
	log.Printf("  Migoku:    %s", cfg.MigokuURL)
	log.Printf("  Guild:     %s", cfg.GuildID)
	log.Printf("  Template:  %s", cfg.NickTemplate)
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

func getEnvAny(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func loadConfigAndEnv(configPath string) (*Config, bool) {
	var cfg *Config
	loadedFromFile := false

	targetPath := configPath
	if targetPath == "" {
		targetPath = defaultConfigPath()
	}

	c, err := loadConfig(targetPath)
	if err == nil {
		cfg = c
		loadedFromFile = true
	} else if configPath != "" && !errors.Is(err, errConfigNotFound) {
		log.Printf("Warning: failed to read config file %s: %v", configPath, err)
	}

	if cfg == nil {
		cfg = &Config{
			MigokuURL:    "http://localhost:8080",
			NickTemplate: "{name} | {ja}{emoji}{lang} {known}",
			DefaultLang:  "ja",
			Interval:     "1h",
		}
	}

	// Environment variable overrides
	if v := getEnvAny("MIGOKU_URL"); v != "" {
		cfg.MigokuURL = strings.TrimRight(v, "/")
	}
	if v := getEnvAny("MIGOKU_API_KEY", "API_KEY"); v != "" {
		cfg.MigokuKey = v
	}
	if v := getEnvAny("DISCORD_TOKEN", "TOKEN"); v != "" {
		cfg.DiscordToken = v
	}
	if v := getEnvAny("DISCORD_NAME", "NICKNAME"); v != "" {
		cfg.DiscordName = v
	}
	if v := getEnvAny("DISCORD_GUILD_ID", "GUILD_ID"); v != "" {
		cfg.GuildID = v
	}
	if v := getEnvAny("DISCORD_GUILD_NAME", "GUILD_NAME"); v != "" {
		cfg.GuildName = v
	}
	if v := getEnvAny("NICK_TEMPLATE", "TEMPLATE", "DISCORD_NICK_TEMPLATE"); v != "" {
		cfg.NickTemplate = v
	}
	if v := getEnvAny("MIGOKU_LANG", "DEFAULT_LANG", "LANG"); v != "" {
		cfg.DefaultLang = v
	}
	if v := getEnvAny("INTERVAL", "UPDATE_INTERVAL"); v != "" {
		cfg.Interval = v
	}

	return cfg, loadedFromFile
}

func validateAndResolveConfig(cfg *Config) error {
	if cfg.DiscordToken == "" {
		if token, err := detectLocalDiscordToken(cfg.DiscordName); err == nil && token != "" {
			cfg.DiscordToken = token
			log.Println("Auto-detected active Discord token from local Discord client")
			tryPersistConfigToken(cfg)
		} else {
			return errors.New("DISCORD_TOKEN is required (set via DISCORD_TOKEN environment variable, config file, or have Discord app logged in)")
		}
	}

	if cfg.MigokuKey == "" {
		return errors.New("MIGOKU_API_KEY is required (set via MIGOKU_API_KEY environment variable or config file)")
	}

	user, err := getCurrentUser(cfg.DiscordToken)
	if err != nil && isDiscord401(err) {
		log.Println("Configured Discord token is invalid or expired (401); attempting auto-detection from local Discord client...")
		if token, dErr := detectLocalDiscordToken(cfg.DiscordName); dErr == nil && token != "" {
			cfg.DiscordToken = token
			user, err = getCurrentUser(cfg.DiscordToken)
			if err == nil {
				log.Println("Successfully refreshed Discord token from local client")
				tryPersistConfigToken(cfg)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("verify Discord token: %w", err)
	}

	if cfg.DiscordName == "" {
		cfg.DiscordName = user.GlobalName
		if cfg.DiscordName == "" {
			cfg.DiscordName = user.Username
		}
	}
	log.Printf("Authenticated as Discord user: %s (%s)", cfg.DiscordName, user.ID)

	if cfg.GuildID == "" {
		guilds, err := listUserGuilds(cfg.DiscordToken)
		if err != nil {
			return fmt.Errorf("list Discord servers: %w", err)
		}
		if len(guilds) == 0 {
			return errors.New("no Discord servers found for this token")
		}

		if cfg.GuildName != "" {
			target := strings.ToLower(cfg.GuildName)
			for _, g := range guilds {
				if strings.ToLower(g.Name) == target || strings.Contains(strings.ToLower(g.Name), target) {
					cfg.GuildID = g.ID
					log.Printf("Matched Discord server %q: %s (%s)", cfg.GuildName, g.Name, g.ID)
					break
				}
			}
			if cfg.GuildID == "" {
				log.Println("Available Discord servers:")
				for _, g := range guilds {
					log.Printf("  - %s (%s)", g.Name, g.ID)
				}
				return fmt.Errorf("no server matched GUILD_NAME %q", cfg.GuildName)
			}
		} else if len(guilds) == 1 {
			cfg.GuildID = guilds[0].ID
			log.Printf("Auto-selected Discord server: %s (%s)", guilds[0].Name, guilds[0].ID)
		} else {
			log.Println("Multiple Discord servers found:")
			for _, g := range guilds {
				log.Printf("  - %s (ID: %s)", g.Name, g.ID)
			}
			return errors.New("DISCORD_GUILD_ID is required when user belongs to multiple servers (or set DISCORD_GUILD_NAME)")
		}
	}

	if cfg.NickTemplate == "" {
		cfg.NickTemplate = "{name} | {ja}{emoji}{lang} {known}"
	}

	return nil
}

func runSetup(initialCfg *Config, configPath string) *Config {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== Discord Nickname Updater Setup ===")

	if configPath == "" {
		configPath = defaultConfigPath()
	}

	cfg := initialCfg
	if cfg == nil {
		cfg = &Config{
			MigokuURL:    "http://localhost:8080",
			NickTemplate: "{name} | {ja}{emoji}{lang} {known}",
			DefaultLang:  "ja",
			Interval:     "1h",
		}
	}

	fmt.Printf("Migoku URL [%s]: ", cfg.MigokuURL)
	if input := readLine(reader); input != "" {
		cfg.MigokuURL = strings.TrimRight(input, "/")
	}

	fmt.Print("Migoku API Key")
	if cfg.MigokuKey != "" {
		fmt.Printf(" [%s]", maskSecret(cfg.MigokuKey))
	}
	fmt.Print(": ")
	if input := readLine(reader); input != "" {
		cfg.MigokuKey = input
	}
	if cfg.MigokuKey == "" {
		log.Fatal("API key is required")
	}

	fmt.Print("Discord User Token")
	if cfg.DiscordToken != "" {
		fmt.Printf(" [%s]", maskSecret(cfg.DiscordToken))
	}
	fmt.Print(": ")
	if input := readLine(reader); input != "" {
		cfg.DiscordToken = input
	}
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
	selectedIdx := 1
	for i, g := range guilds {
		fmt.Printf("  [%d] %s (%s)\n", i+1, g.Name, g.ID)
		if cfg.GuildID == g.ID {
			selectedIdx = i + 1
		}
	}
	fmt.Printf("Pick a server [%d]: ", selectedIdx)
	choiceStr := readLine(reader)
	choice := selectedIdx
	if choiceStr != "" {
		if c, err := strconv.Atoi(choiceStr); err == nil && c >= 1 && c <= len(guilds) {
			choice = c
		}
	}
	cfg.GuildID = guilds[choice-1].ID
	fmt.Printf("Selected: %s\n", guilds[choice-1].Name)

	for {
		fmt.Printf("Nickname template [%s]:\n", cfg.NickTemplate)
		fmt.Println("  Placeholders: {name} {emoji} {lang} {known} {learning} {total}")
		fmt.Println("  Use language codes (e.g. {ja} {en}) in the template to switch languages")
		if input := readLine(reader); input != "" {
			cfg.NickTemplate = input
		}
		if len(getEffectiveLangs(cfg.NickTemplate, cfg.DefaultLang)) > 0 {
			break
		}
		fmt.Println("  Template must include at least one placeholder")
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

func maskSecret(s string) string {
	if len(s) <= 8 {
		return "********"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

type langCount struct {
	known    int
	learning int
}

func updateNickname(cfg *Config) error {
	langCodes := getEffectiveLangs(cfg.NickTemplate, cfg.DefaultLang)

	langCounts := make(map[string]langCount, len(langCodes))
	seen := make(map[string]bool)
	for _, lc := range langCodes {
		if seen[lc] {
			continue
		}
		seen[lc] = true

		known, learning, err := fetchWordCounts(cfg.MigokuURL, cfg.MigokuKey, lc)
		if err != nil {
			return fmt.Errorf("fetch word counts for %s: %w", lc, err)
		}
		langCounts[lc] = langCount{known, learning}
	}

	newNick := renderTemplate(cfg.NickTemplate, cfg.DiscordName, cfg.DefaultLang, langCounts)
	runes := []rune(newNick)
	if len(runes) > 32 {
		log.Printf("Warning: nickname %q exceeds Discord 32 char limit (%d chars); truncating", newNick, len(runes))
		newNick = string(runes[:32])
	}

	err := setDiscordNickname(cfg.DiscordToken, cfg.GuildID, newNick)
	if err != nil && isDiscord401(err) {
		log.Println("Discord returned 401 Unauthorized; attempting to refresh token from local Discord client...")
		if refreshedToken, rErr := detectLocalDiscordToken(cfg.DiscordName); rErr == nil && refreshedToken != "" && refreshedToken != cfg.DiscordToken {
			cfg.DiscordToken = refreshedToken
			tryPersistConfigToken(cfg)
			log.Println("Refreshed Discord token from local client; retrying nickname update...")
			err = setDiscordNickname(cfg.DiscordToken, cfg.GuildID, newNick)
		}
	}
	if err != nil {
		return fmt.Errorf("set discord nickname: %w", err)
	}

	log.Printf("Nickname updated: %s", newNick)
	return nil
}

func getEffectiveLangs(tmpl, defaultLang string) []string {
	langs := extractLangCodes(tmpl)
	if len(langs) == 0 {
		if defaultLang == "" {
			defaultLang = "ja"
		}
		return []string{defaultLang}
	}
	return langs
}

func extractLangCodes(tmpl string) []string {
	knownPH := map[string]bool{
		"name": true, "emoji": true, "lang": true,
		"known": true, "learning": true, "total": true,
	}
	var langs []string
	seen := map[string]bool{}

	for i := 0; i < len(tmpl); i++ {
		if tmpl[i] != '{' {
			continue
		}
		end := strings.IndexByte(tmpl[i+1:], '}')
		if end == -1 {
			break
		}
		token := tmpl[i+1 : i+1+end]
		if !knownPH[token] && !seen[token] {
			langs = append(langs, token)
			seen[token] = true
		}
		i += end
	}
	return langs
}

func renderTemplate(tmpl, name, defaultLang string, langCounts map[string]langCount) string {
	var out strings.Builder
	currentLang := defaultLang
	if currentLang == "" {
		currentLang = "ja"
	}

	i := 0
	for i < len(tmpl) {
		bs := strings.IndexByte(tmpl[i:], '{')
		if bs == -1 {
			out.WriteString(tmpl[i:])
			break
		}
		out.WriteString(tmpl[i : i+bs])
		be := strings.IndexByte(tmpl[i+bs+1:], '}')
		if be == -1 {
			out.WriteString(tmpl[i+bs:])
			break
		}
		token := tmpl[i+bs+1 : i+bs+1+be]

		switch token {
		case "name":
			out.WriteString(name)
		case "emoji":
			if e, ok := langEmoji[currentLang]; ok && currentLang != "" {
				out.WriteString(e)
			}
		case "lang":
			out.WriteString(strings.ToUpper(currentLang))
		case "known":
			if c, ok := langCounts[currentLang]; ok {
				out.WriteString(formatInt(c.known))
			}
		case "learning":
			if c, ok := langCounts[currentLang]; ok {
				out.WriteString(formatInt(c.learning))
			}
		case "total":
			if c, ok := langCounts[currentLang]; ok {
				out.WriteString(formatInt(c.known + c.learning))
			}
		default:
			if _, ok := langCounts[token]; ok {
				currentLang = token
			} else {
				out.WriteByte('{')
				out.WriteString(token)
				out.WriteByte('}')
			}
		}

		i = i + bs + be + 2
	}

	return out.String()
}

func fetchWordCounts(migokuURL, apiKey, lang string) (known, learning int, err error) {
	endpoint := fmt.Sprintf("%s/api/v1/status/counts?lang=%s", strings.TrimRight(migokuURL, "/"), lang)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
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

func setDiscordHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/119.0")
	req.Header.Set("Accept", "*/*")
}

func isDiscord401(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "401")
}

func tryPersistConfigToken(cfg *Config) {
	p := defaultConfigPath()
	if _, err := os.Stat(p); err == nil {
		if diskCfg, err := loadConfig(p); err == nil && diskCfg.DiscordToken != cfg.DiscordToken {
			diskCfg.DiscordToken = cfg.DiscordToken
			saveConfig(diskCfg, p)
			log.Printf("Updated Discord token in config file: %s", p)
		}
	}
}

func detectLocalDiscordToken(preferredName string) (string, error) {
	var searchDirs []string
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "linux":
		searchDirs = []string{
			filepath.Join(home, ".config", "discord", "Local Storage", "leveldb"),
			filepath.Join(home, ".config", "discordcanary", "Local Storage", "leveldb"),
			filepath.Join(home, ".config", "discordptb", "Local Storage", "leveldb"),
			filepath.Join(home, ".config", "vesktop", "Local Storage", "leveldb"),
			filepath.Join(home, ".config", "google-chrome", "Default", "Local Storage", "leveldb"),
			filepath.Join(home, ".config", "chromium", "Default", "Local Storage", "leveldb"),
		}
	case "darwin":
		searchDirs = []string{
			filepath.Join(home, "Library", "Application Support", "discord", "Local Storage", "leveldb"),
			filepath.Join(home, "Library", "Application Support", "discordcanary", "Local Storage", "leveldb"),
			filepath.Join(home, "Library", "Application Support", "vesktop", "Local Storage", "leveldb"),
		}
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			searchDirs = []string{
				filepath.Join(appData, "discord", "Local Storage", "leveldb"),
				filepath.Join(appData, "discordcanary", "Local Storage", "leveldb"),
				filepath.Join(appData, "discordptb", "Local Storage", "leveldb"),
				filepath.Join(appData, "vesktop", "Local Storage", "leveldb"),
			}
		}
	}

	tokenRe := regexp.MustCompile(`[\w-]{24,26}\.[\w-]{6}\.[\w-]{27,38}|mfa\.[\w-]{84}`)
	seen := make(map[string]bool)
	var candidates []string

	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".ldb") && !strings.HasSuffix(name, ".log") {
				continue
			}
			fpath := filepath.Join(dir, name)
			content, err := os.ReadFile(fpath)
			if err != nil {
				continue
			}
			matches := tokenRe.FindAll(content, -1)
			for _, m := range matches {
				t := string(m)
				if !seen[t] {
					seen[t] = true
					candidates = append(candidates, t)
				}
			}
		}
	}

	if len(candidates) == 0 {
		return "", errors.New("no Discord session files found in local storage")
	}

	var firstValid string
	for _, token := range candidates {
		user, err := getCurrentUser(token)
		if err == nil && user != nil && user.ID != "" {
			if preferredName != "" && (strings.EqualFold(user.Username, preferredName) || strings.EqualFold(user.GlobalName, preferredName)) {
				return token, nil
			}
			if firstValid == "" {
				firstValid = token
			}
		}
	}

	if firstValid != "" {
		return firstValid, nil
	}

	return "", errors.New("no active valid Discord token found in local Discord storage")
}

func getCurrentUser(token string) (*DiscordUser, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return nil, err
	}
	setDiscordHeaders(req, token)

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
	setDiscordHeaders(req, token)

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
	setDiscordHeaders(req, token)
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
	const M = 1_000_000
	const K = 1_000
	if n >= M {
		if n%M == 0 {
			return strconv.Itoa(n/M) + "M"
		}
		return strconv.FormatFloat(float64(n)/M, 'f', 1, 64) + "M"
	}
	if n >= K {
		if n%K == 0 {
			return strconv.Itoa(n/K) + "k"
		}
		return strconv.FormatFloat(float64(n)/K, 'f', 1, 64) + "k"
	}
	return strconv.Itoa(n)
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

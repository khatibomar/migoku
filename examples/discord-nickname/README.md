# Discord Nickname Updater

Periodically updates your Discord nickname with your Migaku word counts.

## Usage

```bash
# Interactive setup (recommended)
go run main.go -i

# Or with a pre-existing config file
go run main.go -config /path/to/config.json
```

The interactive setup will ask for:
1. **Migoku URL** — defaults to `http://localhost:8080`
2. **Migoku API Key** — get one from `POST /auth/login`
3. **Discord User Token** — your Discord account token (see below)
4. **Discord Server** — pick a server where the nickname will be updated
5. **Language code** — e.g. `ja`, `zh`, `es`, `fr`, `de`
6. **Nickname template** — placeholders: `{name}` `{emoji}` `{lang}` `{known}` `{learning}` `{total}`
7. **Update interval** — e.g. `30m`, `1h`, `6h`

Config is saved to `~/.config/migoku-discord-nickname.json`.

### Getting your Discord token

1. Open Discord in your browser
2. Open DevTools (`F12`) → Application → Local Storage → `https://discord.com`
3. Find `token` key and copy its value
4. Alternatively, press `Ctrl+Shift+I` → Network tab → look for any request to `discord.com/api` and copy the `Authorization` header

> ⚠️ Never share your Discord token. It grants full access to your account.

## Template examples

The template supports these placeholders:

| Placeholder | Description |
|---|---|
| `{name}` | Your Discord display name |
| `{emoji}` | Flag emoji for the current language |
| `{lang}` | Uppercase language code (e.g., `JA`, `ZH`, `EN`) |
| `{known}` | Known word count (comma-formatted) |
| `{learning}` | Learning word count |
| `{total}` | Known + Learning total |
| `{ja}` `{zh}` `{en}` | Language code as a placeholder — switches the current language for subsequent tokens |

| Template | Result |
|---|---|
| `{name} \| {emoji}{lang} {known}` | `Ayn 🇯🇵JA 1,234` |
| `{emoji}{lang} 🎓 {known}/{total}` | `🇯🇵JA 🎓 1,234/1,500` |
| `📖 {learning} ✅ {known}` | `📖 56 ✅ 1,234` |
| `{ja}{emoji}{lang} {known} {en}{emoji}{lang} {known}` | `🇯🇵JA 1,234 🇺🇸EN 5,678` |
| `{name} \| {de}{emoji}{lang} {total} · {ja}{emoji}{lang} {total}` | `Ayn 🇩🇪DE 890 · 🇯🇵JA 1,500` |

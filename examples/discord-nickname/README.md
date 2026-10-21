# Discord Nickname Updater

Periodically updates your Discord nickname with your Migaku word counts. Runs headless out-of-the-box on Unikraft Cloud, Docker, or locally.

---

## Deploy on Unikraft Cloud

You can deploy `discord-nickname` directly to [Unikraft Cloud](https://console.unikraft.cloud) with zero manual prompts. It reads your credentials and settings from environment variables and starts updating immediately.

### Option 1: Unikraft Web Dashboard

1. In the [Unikraft Cloud Console](https://console.unikraft.cloud), deploy an instance or connect your repository.
2. If building from git, set the project directory to `examples/discord-nickname` (where `Kraftfile` and `Dockerfile` reside).
3. Set the following **Environment Variables** in the dashboard:

| Variable | Required | Description | Example |
|---|---|---|---|
| `DISCORD_TOKEN` | **Yes** | Your Discord user or bot token | `mfa.xxxx...` |
| `MIGOKU_API_KEY` | **Yes** | Migoku API Key | `a1b2c3d4...` |
| `DISCORD_GUILD_ID` | Recommended | Discord Server (Guild) ID *(auto-selected if account is in only 1 server)* | `123456789012345678` |
| `MIGOKU_URL` | Optional | Migoku API URL (defaults to `http://localhost:8080`) | `https://migoku-xxxx.fra.unikraft.app` |
| `NICK_TEMPLATE` | Optional | Nickname format (defaults to `{name} \| {ja}{emoji}{lang} {known}`) | `{name} \| {emoji}{lang} {known}` |
| `MIGOKU_LANG` | Optional | Default language code (defaults to `ja`) | `ja`, `zh`, `de`, `es`, `fr` |
| `INTERVAL` | Optional | Refresh frequency (defaults to `1h`) | `30m`, `1h`, `6h` |
| `DISCORD_GUILD_NAME`| Optional | Server name to match if Guild ID is not known | `Japanese Study Club` |
| `DISCORD_NAME` | Optional | Override display name (auto-detected via `@me` if unset) | `Ayn` |

4. Deploy the instance. It will fetch your stats from Migoku, update your Discord nickname, and keep it updated on schedule.

---

### Option 2: Unikraft CLI (`kraft`)

Build and deploy with the `Makefile` shortcuts:

```bash
# 1. Build the Unikraft image
UNIKRAFT_ORG=<my-org> make unikraft-discord-build

# 2. Run the instance with environment variables
UNIKRAFT_ORG=<my-org> \
DISCORD_TOKEN=<token> \
MIGOKU_API_KEY=<api-key> \
GUILD_ID=<server-id> \
MIGOKU_URL=https://migoku-xxxx.fra.unikraft.app \
make unikraft-discord-run
```

To update an existing instance with new changes:
```bash
UNIKRAFT_ORG=<my-org> INSTANCE_NAME=<instance-name> make unikraft-discord-update
```

To sync your latest credentials and tokens from `~/.config/migoku-discord-nickname.json` to your running Unikraft Cloud instance:
```bash
make unikraft-discord-sync
```

---

## Local Usage

### Zero-Input / Headless Run

When running locally, `discord-nickname` **automatically detects your active Discord token** from your installed Discord app (`~/.config/discord`, Discord Canary, PTB, Vesktop, Chrome). If a token expires or rotates, it automatically auto-refreshes from local storage without interruption:

```bash
# Runs immediately with auto-detected token and ~/.config/migoku-discord-nickname.json
go run ./examples/discord-nickname
```

Or pass environment variables explicitly:

```bash
MIGOKU_API_KEY="<api-key>" \
DISCORD_GUILD_ID="<server-id>" \
MIGOKU_URL="http://localhost:8080" \
go run ./examples/discord-nickname
```

### Interactive Setup

If you prefer an interactive wizard that saves your settings to `~/.config/migoku-discord-nickname.json`:

```bash
go run ./examples/discord-nickname -i
```

Or with a config file:
```bash
go run ./examples/discord-nickname -config /path/to/config.json
```

---

## Discord Token

`discord-nickname` automatically auto-detects and refreshes your Discord token from your local Discord desktop client on Linux, macOS, and Windows.

If configuring manually (e.g. for a headless remote server):
1. Open Discord in your browser
2. Open DevTools (`F12`) → Network tab
3. Look for any request to `discord.com/api` and copy the `Authorization` header

> ⚠️ Never share your Discord token publicly. It grants access to your Discord account.

---

## Template Placeholders

| Placeholder | Description |
|---|---|
| `{name}` | Discord display name (falls back to username) |
| `{emoji}` | Flag emoji for current language (e.g. 🇯🇵, 🇺🇸, 🇪🇸, 🇩🇪) |
| `{lang}` | Uppercase language code (e.g., `JA`, `ZH`, `EN`) |
| `{known}` | Known word count (formatted as `1.2k` or `1M`) |
| `{learning}` | Learning word count |
| `{total}` | Known + Learning total |
| `{streak}` | Current study streak in days (from `/api/v1/stats/streak`; fetched only when the template uses it) |
| `{ja}`, `{zh}`, `{en}`, etc. | Switches the current language for subsequent tokens |

### Examples

| Template | Result |
|---|---|
| `{name} \| {emoji}{lang} {known}` | `Ayn \| 🇯🇵JA 1.2k` |
| `{emoji}{lang} 🎓 {known}/{total}` | `🇯🇵JA 🎓 1.2k/1.5k` |
| `📖 {learning} ✅ {known}` | `📖 56 ✅ 1.2k` |
| `{name} \| 🔥 {streak}d` | `Ayn \| 🔥 42d` |
| `{ja}{emoji}{lang} {known} 🔥{streak}` | `🇯🇵JA 1.2k 🔥42` |
| `{ja}{emoji}{lang} {known} {en}{emoji}{lang} {known}` | `🇯🇵JA 1.2k 🇺🇸EN 5.7k` |
| `{name} \| {de}{emoji}{lang} {total} · {ja}{emoji}{lang} {total}` | `Ayn \| 🇩🇪DE 890 · 🇯🇵JA 1.5k` |

> **Note:** Nicknames exceeding Discord's 32-character limit are automatically truncated safely so API calls never fail.

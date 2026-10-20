.PHONY: run build clean docker-run lint test examples android-widget-apk unikraft-run unikraft-update unikraft-discord-build unikraft-discord-run unikraft-discord-update unikraft-discord-sync

# Run the API server
run: build
	@echo "Starting Migoku API Server..."
	LOG_LEVEL=debug API_SECRET="top-secret" go run .

# Build the server binary
build:
	@echo "Building migoku..."
	go build -o bin/migoku
	@echo "Binary created: migoku"

# Run with Docker Compose
docker-run:
	@echo "Starting Migoku API Server with Docker..."
	docker compose up --build

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f migoku

# Run linting
lint:
	golangci-lint run --config .golangci.yml

# Run tests with the race detector
test:
	go test -race ./...

# Serve static HTML examples from ./examples
# Open in browser:
#   http://localhost:4173/stats-dashboard/
#   http://localhost:4173/ai-study-coach/
#   http://localhost:4173/android-widget/
examples:
	@echo "Serving static examples on http://localhost:4173"
	@echo "Open /stats-dashboard/, /ai-study-coach/, or /android-widget/"
	cd examples && python3 -m http.server 4173

# Build the Android widget debug APK locally
android-widget-apk:
	@if [ -z "$(ANDROID_HOME)" ]; then echo "ERROR: ANDROID_HOME is not set (Android SDK required)"; exit 1; fi
	@if [ -z "$(JAVA_HOME)" ]; then echo "ERROR: JAVA_HOME is not set (JDK 17 required)"; exit 1; fi
	cd examples/android-widget && ./gradlew assembleDebug --no-daemon
	@echo "APK ready: examples/android-widget/app/build/outputs/apk/debug/app-debug.apk"

# Unikraft deployment

unikraft-run:
	@if ! ps -a | grep -q buildkitd; then echo "ERROR: buildkitd is not running — start it with 'sudo buildkitd'"; exit 1; fi
	@if [ -z "$(UNIKRAFT_ORG)" ]; then echo "ERROR: UNIKRAFT_ORG is not set"; exit 1; fi
	@if [ -z "$(API_SECRET)" ]; then echo "ERROR: API_SECRET is not set"; exit 1; fi
	@echo "Deploying Unikraft image..."
	unikraft run --metro fra -p 443:8080/tls+http -m 256M -e CACHE_TTL=12h -e API_SECRET=$(API_SECRET) --image $(UNIKRAFT_ORG)/migoku:latest

unikraft-update:
	@if ! ps -a | grep -q buildkitd; then echo "ERROR: buildkitd is not running — start it with 'sudo buildkitd'"; exit 1; fi
	@if [ -z "$(INSTANCE_NAME)" ]; then echo "ERROR: INSTANCE_NAME is not set"; exit 1; fi
	@if [ -z "$(UNIKRAFT_ORG)" ]; then echo "ERROR: UNIKRAFT_ORG is not set"; exit 1; fi
	@echo "Building Unikraft image..."
	unikraft build . --output $(UNIKRAFT_ORG)/migoku:latest
	@echo "Stopping Unikraft instance '$(INSTANCE_NAME)'..."
	unikraft instances stop $(INSTANCE_NAME)
	@echo "Updating Unikraft instance '$(INSTANCE_NAME)' with latest image..."
	unikraft instances edit $(INSTANCE_NAME) --image $(UNIKRAFT_ORG)/migoku:latest
	@echo "Starting Unikraft instance '$(INSTANCE_NAME)'..."
	unikraft instances start $(INSTANCE_NAME)

# Discord Nickname Unikraft deployment

unikraft-discord-build:
	@if ! pgrep -a buildkitd >/dev/null 2>&1 && ! docker ps >/dev/null 2>&1; then echo "ERROR: buildkitd or docker daemon is not running — start it with 'sudo buildkitd' or start Docker"; exit 1; fi
	@if [ -z "$(UNIKRAFT_ORG)" ]; then echo "ERROR: UNIKRAFT_ORG is not set"; exit 1; fi
	@echo "Building Unikraft Discord nickname image..."
	cd examples/discord-nickname && unikraft build . --output $(UNIKRAFT_ORG)/migoku-discord-nickname:latest

unikraft-discord-run:
	@if ! pgrep -a buildkitd >/dev/null 2>&1 && ! docker ps >/dev/null 2>&1; then echo "ERROR: buildkitd or docker daemon is not running — start it with 'sudo buildkitd' or start Docker"; exit 1; fi
	@if [ -z "$(UNIKRAFT_ORG)" ]; then echo "ERROR: UNIKRAFT_ORG is not set"; exit 1; fi
	@if [ -z "$(DISCORD_TOKEN)" ]; then echo "ERROR: DISCORD_TOKEN is not set"; exit 1; fi
	@if [ -z "$(MIGOKU_API_KEY)" ] && [ -z "$(API_KEY)" ]; then echo "ERROR: MIGOKU_API_KEY is not set"; exit 1; fi
	@echo "Deploying Unikraft Discord nickname instance..."
	unikraft run --metro fra -m 128M \
		-e DISCORD_TOKEN=$(DISCORD_TOKEN) \
		-e MIGOKU_API_KEY=$(if $(MIGOKU_API_KEY),$(MIGOKU_API_KEY),$(API_KEY)) \
		$(if $(MIGOKU_URL),-e MIGOKU_URL=$(MIGOKU_URL),) \
		$(if $(GUILD_ID),-e GUILD_ID=$(GUILD_ID),) \
		$(if $(DISCORD_GUILD_ID),-e DISCORD_GUILD_ID=$(DISCORD_GUILD_ID),) \
		$(if $(DISCORD_GUILD_NAME),-e DISCORD_GUILD_NAME='$(DISCORD_GUILD_NAME)',) \
		$(if $(TEMPLATE),-e TEMPLATE='$(TEMPLATE)',) \
		$(if $(INTERVAL),-e INTERVAL=$(INTERVAL),) \
		--image $(UNIKRAFT_ORG)/migoku-discord-nickname:latest

unikraft-discord-update:
	@if ! pgrep -a buildkitd >/dev/null 2>&1 && ! docker ps >/dev/null 2>&1; then echo "ERROR: buildkitd or docker daemon is not running — start it with 'sudo buildkitd' or start Docker"; exit 1; fi
	@if [ -z "$(INSTANCE_NAME)" ]; then echo "ERROR: INSTANCE_NAME is not set"; exit 1; fi
	@if [ -z "$(UNIKRAFT_ORG)" ]; then echo "ERROR: UNIKRAFT_ORG is not set"; exit 1; fi
	@echo "Building Unikraft Discord nickname image..."
	cd examples/discord-nickname && unikraft build . --output $(UNIKRAFT_ORG)/migoku-discord-nickname:latest
	@echo "Stopping Unikraft instance '$(INSTANCE_NAME)'..."
	unikraft instances stop $(INSTANCE_NAME)
	@echo "Updating Unikraft instance '$(INSTANCE_NAME)' with latest image..."
	unikraft instances edit $(INSTANCE_NAME) --image $(UNIKRAFT_ORG)/migoku-discord-nickname:latest
	@echo "Starting Unikraft instance '$(INSTANCE_NAME)'..."
	unikraft instances start $(INSTANCE_NAME)

unikraft-discord-sync:
	@INSTANCE=$${INSTANCE_NAME:-migoku-discord-nickname}; \
	echo "Syncing latest configuration from ~/.config/migoku-discord-nickname.json to '$$INSTANCE'..."; \
	python3 -c 'import json, os, subprocess; p = os.path.expanduser("~/.config/migoku-discord-nickname.json"); f = open(p); cfg = json.load(f); f.close(); cmd = ["unikraft", "instances", "edit", "'$$INSTANCE'"]; [cmd.extend(["-e", f"{k}={v}"]) for k, v in [("DISCORD_TOKEN", cfg.get("discord_token")), ("MIGOKU_API_KEY", cfg.get("migoku_api_key")), ("MIGOKU_URL", cfg.get("migoku_url")), ("GUILD_ID", cfg.get("guild_id")), ("DISCORD_NAME", cfg.get("discord_name")), ("NICK_TEMPLATE", cfg.get("nick_template")), ("INTERVAL", cfg.get("interval"))] if v]; subprocess.check_call(cmd); print("Sync complete!")'


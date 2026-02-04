PLUGIN_ID ?= com.fambear.ai-limits-monitor
PLUGIN_VERSION ?= 0.1.0
BUNDLE_NAME ?= $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz

# Build targets
PLATFORMS = linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

GO ?= $(shell command -v go 2> /dev/null)
NPM ?= $(shell command -v npm 2> /dev/null)

.PHONY: all build server webapp bundle clean

all: build

build: server webapp bundle

## Server build — all platforms
server:
	@echo "Building server..."
	@mkdir -p server/dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "  Building $$os/$$arch..."; \
		cd server && \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -o dist/plugin-$$os-$$arch$$ext . && \
		cd ..; \
	done
	@echo "Server build complete"

## Webapp build
webapp:
	@echo "Building webapp..."
	cd webapp && $(NPM) install --legacy-peer-deps && $(NPM) run build
	@echo "Webapp build complete"

## Bundle into tar.gz
bundle:
	@echo "Creating plugin bundle..."
	@rm -rf dist
	@mkdir -p dist/$(PLUGIN_ID)/server/dist dist/$(PLUGIN_ID)/webapp/dist dist/$(PLUGIN_ID)/assets
	@cp plugin.json dist/$(PLUGIN_ID)/
	@cp -r server/dist/* dist/$(PLUGIN_ID)/server/dist/
	@cp webapp/dist/main.js dist/$(PLUGIN_ID)/webapp/dist/
	@cp assets/icon.svg dist/$(PLUGIN_ID)/assets/
	@cd dist && tar -czf $(BUNDLE_NAME) $(PLUGIN_ID)
	@echo "Bundle created: dist/$(BUNDLE_NAME)"

clean:
	@rm -rf dist server/dist webapp/dist webapp/node_modules

## Build single platform (for dev)
server-local:
	@mkdir -p server/dist
	cd server && CGO_ENABLED=0 $(GO) build -trimpath -o dist/plugin-$(shell go env GOOS)-$(shell go env GOARCH) .

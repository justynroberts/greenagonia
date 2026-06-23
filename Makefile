.PHONY: build build-all clean sync-site generate-scenarios fmt vet

BIN      := bin
PKG_DIR  := ./cli
NAME     := greenagonia
SITE_SRC := ./site
SITE_DST := $(PKG_DIR)/site

# Generate scenarios.json from Go source (single source of truth).
# Bootstraps cli/site/ with a stub if it doesn't exist yet (fresh clone),
# so go:embed compiles; sync-site replaces the stub with real content.
generate-scenarios:
	@if [ ! -d $(SITE_DST) ]; then mkdir -p $(SITE_DST) && echo "" > $(SITE_DST)/placeholder; fi
	cd $(PKG_DIR) && go run . scenarios dump-json > $(abspath $(SITE_SRC))/scenarios.json

# Copy shared-site (including freshly generated scenarios.json) into the CLI package.
sync-site: generate-scenarios
	rm -rf $(SITE_DST)
	mkdir -p $(SITE_DST)
	cp -r $(SITE_SRC)/. $(SITE_DST)/

# Native build for the host platform.
build: sync-site
	mkdir -p $(BIN)
	cd $(PKG_DIR) && go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME) .

# Cross-compile for the usual desktop targets.
build-all: sync-site
	mkdir -p $(BIN)
	cd $(PKG_DIR) && GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-darwin-arm64 .
	cd $(PKG_DIR) && GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-darwin-amd64 .
	cd $(PKG_DIR) && GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-linux-amd64 .
	cd $(PKG_DIR) && GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-linux-arm64 .
	cd $(PKG_DIR) && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-windows-amd64.exe .

clean:
	rm -rf $(BIN) $(SITE_DST)

fmt:
	cd $(PKG_DIR) && go fmt ./...

vet:
	cd $(PKG_DIR) && go vet ./...

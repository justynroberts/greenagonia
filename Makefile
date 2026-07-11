.PHONY: build build-all clean generate-scenarios fmt vet

PKG_DIR := ./cli
NAME    := greenagonia

# Generate scenarios.json in site/ from Go source (single source of truth).
generate-scenarios:
	cd $(PKG_DIR) && go run . scenarios dump-json > ../site/scenarios.json

# Native build for the host platform.
build: generate-scenarios
	cd $(PKG_DIR) && go build -trimpath -ldflags "-s -w" -o ../$(NAME) .

# Cross-compile for the usual desktop targets.
build-all: generate-scenarios
	cd $(PKG_DIR) && GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ../$(NAME)-darwin-arm64 .
	cd $(PKG_DIR) && GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(NAME)-darwin-amd64 .
	cd $(PKG_DIR) && GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(NAME)-linux-amd64 .
	cd $(PKG_DIR) && GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ../$(NAME)-linux-arm64 .
	cd $(PKG_DIR) && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(NAME)-windows-amd64.exe .

clean:
	rm -f $(NAME) $(NAME)-darwin-* $(NAME)-linux-* $(NAME)-windows-*

fmt:
	cd $(PKG_DIR) && go fmt ./...

vet:
	cd $(PKG_DIR) && go vet ./...

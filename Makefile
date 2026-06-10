.PHONY: build build-all clean fmt vet

BIN     := bin
PKG_DIR := ./cli
NAME    := greenagonia

# Native build for the host platform.
build:
	mkdir -p $(BIN)
	cd $(PKG_DIR) && go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME) .

# Cross-compile for the usual desktop targets.
build-all: clean
	mkdir -p $(BIN)
	cd $(PKG_DIR) && GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-darwin-arm64 .
	cd $(PKG_DIR) && GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-darwin-amd64 .
	cd $(PKG_DIR) && GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-linux-amd64 .
	cd $(PKG_DIR) && GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-linux-arm64 .
	cd $(PKG_DIR) && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../$(BIN)/$(NAME)-windows-amd64.exe .

clean:
	rm -rf $(BIN)

fmt:
	cd $(PKG_DIR) && go fmt ./...

vet:
	cd $(PKG_DIR) && go vet ./...

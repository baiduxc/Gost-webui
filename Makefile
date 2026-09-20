GO ?= go
BIN_DIR ?= bin
VERSION ?= 1.0.0

.PHONY: all build gost test vet clean run

all: build

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN_DIR)/gost-webui .

# 编译 gost（面板需要它；也可直接使用官方发行版二进制）
gost:
	@mkdir -p $(BIN_DIR)
	@if [ ! -d .gost-src ]; then \
		git clone --depth 1 https://github.com/go-gost/gost.git .gost-src; \
	fi
	cd .gost-src && CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o ../$(BIN_DIR)/gost ./cmd/gost

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

run: build
	$(BIN_DIR)/gost-webui -c ./panel.dev.yml -listen :8787

clean:
	rm -rf $(BIN_DIR) .gost-src

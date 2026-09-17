# Fera Games — atalhos de desenvolvimento.
# `make` ou `make help` lista os alvos.

GO        ?= go
BIN       ?= fera
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    = -s -w -X main.version=$(VERSION)

# golangci-lint v2 compilado com o Go do go.mod; o v1 do PATH não roda com go 1.26.
GOLANGCI  ?= $(shell $(GO) env GOPATH)/bin/golangci-lint
GOLANGCI_VERSION ?= latest
# goreleaser exige Go mais novo que o do go.mod; GOTOOLCHAIN=auto baixa o que precisar.
GORELEASER ?= $(shell $(GO) env GOPATH)/bin/goreleaser

.DEFAULT_GOAL := help

.PHONY: help
help: ## lista os alvos
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: check-audio-deps
check-audio-deps: ## garante libasound2-dev (Linux) para compilar o oto/v3
ifeq ($(shell uname -s),Linux)
	@pkg-config --exists alsa || { \
		echo "==> libasound2-dev (pkg-config: alsa) não encontrado."; \
		if command -v apt-get >/dev/null 2>&1; then \
			echo "==> instalando via apt-get (vai pedir senha do sudo)..."; \
			sudo apt-get update && sudo apt-get install -y libasound2-dev; \
		else \
			echo "Instale o pacote de desenvolvimento do ALSA para sua distro (ex.: alsa-lib-devel no Fedora, alsa-lib no Arch) e rode 'make run' de novo."; \
			exit 1; \
		fi; \
	}
endif

.PHONY: run
run: check-audio-deps ## abre o menu (go run ./cmd/fera)
	$(GO) run ./cmd/fera $(ARGS)

.PHONY: build
build: ## compila ./$(BIN)
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/fera

.PHONY: test
test: ## go test ./...
	$(GO) test ./...

.PHONY: race
race: ## go test -race nos pacotes da TUI
	$(GO) test -race ./internal/app/... ./internal/tui/...

.PHONY: vet
vet: ## go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## gofmt -l (falha se houver arquivo fora do padrão)
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

.PHONY: lint
lint: $(GOLANGCI) ## golangci-lint run
	$(GOLANGCI) run ./...

$(GOLANGCI):
	GOTOOLCHAIN=$$($(GO) env GOVERSION) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

.PHONY: check
check: fmt vet test lint ## tudo que a CI roda

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: probe
probe: ## puzzle de hoje sem spoiler (go run ./cmd/probe -file today)
	$(GO) run ./cmd/probe -file today

.PHONY: play
play: check-audio-deps ## toca a faixa 1 de hoje
	$(GO) run ./cmd/probe -file stem:1 -play

.PHONY: walk
walk: check-audio-deps ## toca os estágios 1..5 de hoje trocando a cada 4 s (teste de ouvido da Fase 3)
	$(GO) run ./cmd/probe -walk 4s

.PHONY: snapshot
snapshot: $(GORELEASER) ## binários para todas as plataformas em dist/ (FERA_VERSION=v0.1.0 nomeia a versão)
	FERA_VERSION=$(VERSION) $(GORELEASER) build --snapshot --clean

$(GORELEASER):
	GOTOOLCHAIN=auto $(GO) install github.com/goreleaser/goreleaser/v2@latest

.PHONY: clean
clean: ## remove binário e dist/
	rm -rf $(BIN) dist/

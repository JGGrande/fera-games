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

.PHONY: check-clipboard
check-clipboard: ## garante a existência do xclip (Linux) 
ifeq ($(shell uname -s),Linux)
	@pkg-config --exists xclip || { \
		echo "==> xclip não encontrado."; \
		if command -v apt-get >/dev/null 2>&1; then \
			echo "==> instalando via apt-get (vai pedir senha do sudo)..."; \
			sudo apt-get update && sudo apt-get install -y xclip; \
		else \
			echo "Instale o pacote de desenvolvimento do xclip para sua distro e rode 'make run' de novo."; \
			exit 1; \
		fi; \
	}
endif

# O Fera Músicas baixa o áudio do YouTube no cliente com yt-dlp + ffmpeg. Este
# alvo só confere e imprime como instalar; nunca roda sudo. Não falha: sem as
# ferramentas o jogo abre e o cadastro funciona, só o áudio fica indisponível.
.PHONY: check-media-deps
check-media-deps: ## confere yt-dlp e ffmpeg (Fera Músicas); só avisa, não instala
	@missing=""; \
	for t in yt-dlp ffmpeg; do command -v $$t >/dev/null 2>&1 || missing="$$missing $$t"; done; \
	if [ -z "$$missing" ]; then \
		echo "==> yt-dlp e ffmpeg encontrados"; \
	else \
		echo "==> faltando:$$missing (o Fera Músicas precisa deles para baixar o áudio)"; \
		case "$$(uname -s)" in \
			Linux) \
				echo "    ffmpeg:  sudo apt-get install -y ffmpeg"; \
				echo "    yt-dlp:  pipx install yt-dlp   (o pacote do apt costuma estar velho e quebrar com o YouTube)";; \
			Darwin) echo "    brew install yt-dlp ffmpeg";; \
			*) echo "    winget install yt-dlp.yt-dlp Gyan.FFmpeg";; \
		esac; \
		echo "    se o download falhar depois, atualize: yt-dlp -U"; \
	fi

deps:
	@echo "Verificando deps de áudio"
	@$(MAKE) check-audio-deps
	@echo "Verificando deps de área de transferência"
	@$(MAKE) check-clipboard
	@echo "Verificando deps de download (Fera Músicas)"
	@$(MAKE) check-media-deps

.PHONY: run
run: ## abre o menu (go run ./cmd/fera)
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

.PHONY: musicas-probe
musicas-probe: ## baixa o trecho de um link do YouTube (URL=https://youtu.be/... PROBE_ARGS="-yt-start 1:30 -yt-play")
	@test -n "$(URL)" || { echo "uso: make musicas-probe URL=https://youtu.be/<id>"; exit 2; }
	$(GO) run ./cmd/probe -yt "$(URL)" $(PROBE_ARGS)

.PHONY: snapshot
snapshot: $(GORELEASER) ## binários para todas as plataformas em dist/ (FERA_VERSION=v0.1.0 nomeia a versão)
	FERA_VERSION=$(VERSION) $(GORELEASER) build --snapshot --clean

$(GORELEASER):
	GOTOOLCHAIN=auto $(GO) install github.com/goreleaser/goreleaser/v2@latest

.PHONY: clean
clean: ## remove binário e dist/
	rm -rf $(BIN) dist/

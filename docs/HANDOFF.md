# Fera Games — handoff para a sessão de código

Atualizado em 2026-09-16. Plano completo: https://claude.ai/code/artifact/5bd1dcf1-d389-491a-a513-994b3c7f65d8

## 1. O projeto

- **O que é:** aplicação de terminal chamada **Fera Games** que roda jogos diários (Bandle, Termo, Songless) com experiência 100% TUI.
- **Restrições fixas:**
  - Go **1.26**, binário único (Linux, macOS, Windows).
  - TUI com Bubble Tea. **Nunca** renderizar HTML nem embutir navegador ou webview.
  - **Sem servidor próprio:** o cliente fala direto com o servidor de cada jogo e guarda o estado localmente.
  - **Uso pessoal:** repo privado, sem release público e sem redistribuir áudio.
- **Prova de conceito:** Bandle (https://bandle.app/daily).
  - Critério de pronto: jogar o puzzle do dia de ponta a ponta no terminal (ouvir as camadas, pular, chutar com autocomplete, ver o resultado para compartilhar), com o mesmo puzzle do site.

## 2. Arquitetura decidida

A arquitetura é hexagonal simples. A TUI fala só com a Engine. A Engine usa um Provider (HTTP), o Audio e o Store. A Engine não conhece HTTP nem terminal.

```
TUI (Bubble Tea) → App/Router → Game Engine → Provider (HTTP) → servidor do jogo
                                           → Audio (oto + beep)
                                           → Store local (JSON)
```

```go
// internal/game
type Game interface {
    ID() string
    Name() string
    NewSession(ctx context.Context, day time.Time) (Session, error)
}
type Session interface {
    State() State
    Guess(ctx context.Context, answer string) (Result, error)
    Skip(ctx context.Context) (Result, error)
    ShareText() string
}

// internal/games/bandle
type Provider interface {
    DailyPuzzle(ctx context.Context, day time.Time) (Puzzle, error)
    SongCatalog(ctx context.Context) ([]Song, error)
    Stem(ctx context.Context, p Puzzle, layer int) (io.ReadCloser, error)
}
```

Layout de pacotes (✅ = já existe):

```
cmd/fera/               main (Fase 1)
cmd/probe/          ✅  spike: número do puzzle, inspeção de URL, tocar áudio (-tags audio)
cmd/harscan/        ✅  spike: HAR do DevTools → relatório de endpoints + testdata
internal/app/           router, config
internal/tui/           menu, input com autocomplete, grade de tentativas, player
internal/game/          interfaces comuns
internal/games/bandle/  ✅ (parcial) PuzzleNumber; engine + provider na Fase 2
internal/audio/         decodificar, mixar N faixas, play/pause/seek
internal/store/         estado do dia em os.UserConfigDir()/fera-games
internal/httpx/         timeout, retry, User-Agent, cache em disco
internal/har/       ✅  parser/classificador de HAR
testdata/               payloads reais capturados
```

## 3. Stack

| Camada | Escolha |
| --- | --- |
| TUI | Bubble Tea v2: `charm.land/bubbletea/v2`. `View()` retorna `tea.View`; teclas chegam como `tea.KeyPressMsg` com `key.Code`, `key.Text` e `key.Mod` |
| Estilo e componentes | Lip Gloss v2, Bubbles v2 (`textinput`, `list`, `progress`, `spinner`, `help`) |
| Áudio | `github.com/ebitengine/oto/v3` (sem cgo) + `github.com/gopxl/beep/v2` (mp3, vorbis, wav, mixer) |
| Fuzzy | `github.com/sahilm/fuzzy` |
| HTTP | `net/http` com cache em disco |
| Persistência | JSON em `os.UserConfigDir()` |
| Testes | `testing`, `teatest` (golden), `httptest` com os payloads de `testdata/` |
| Build | GoReleaser + GitHub Actions (build local, sem release público) |

UX do terminal:
- **Atalhos:** `espaço` tocar/pausar, `←/→` ±5s, `tab` pular, `enter` confirmar, `c` copiar resultado (OSC52), `q` sair.
- **Sem áudio:** `--mute`, ou quando não houver dispositivo, o jogo abre mesmo assim, mostra um aviso e não trava.
- **Tamanho:** mínimo 80×24; abaixo disso o layout se reorganiza.

## 4. Fases

| Fase | Objetivo | Pronto quando | Status |
| --- | --- | --- | --- |
| 0. Spike de protocolo | Mapear puzzle, catálogo e áudio do Bandle | `probe` baixa o puzzle de hoje e toca a faixa 1 | **Parcial**: ver §5 |
| 1. Esqueleto | `cmd/fera`, menu "Fera Games", CI com lint + test | Binário abre o menu e sai com `q` | A fazer |
| 2. Engine + Provider | Regras do Bandle e cliente HTTP, sem TUI | Partida simulada em teste: vitória, derrota e pulos | A fazer |
| 3. Áudio | Tocar e sincronizar as faixas por camada | Nova camada entra em sincronia, sem estalo | A fazer |
| 4. Tela de jogo | Engine + áudio + autocomplete | Bandle do dia jogado de ponta a ponta | A fazer |
| 5. Polimento | Estado persistente, erros, `--mute` | Reabrir no mesmo dia mostra a partida salva; build v0.1.0 local | A fazer |

Depois da PoC:
1. Extrair o que é comum para `internal/game` e `internal/tui`.
2. Termo (term.ooo): grade 6×5, sem áudio.
3. Songless: reaproveita o áudio e o autocomplete.
4. Hub com status do dia, sequência e estatísticas.

## 5. Estado da Fase 0

### O que já se sabe

| Item | Valor | Confiança |
| --- | --- | --- |
| Numeração | `N = dias desde 2022-08-17` (#1 = 2022-08-18; 2026-09-16 = #1491), implementado em `bandle.PuzzleNumber` | Confirmado por posts oficiais |
| Virada do dia | Meia-noite no fuso local | Provável |
| Mecânica | 6 tentativas; cada erro ou pulo libera um instrumento (bateria → baixo → harmonia → voz); a 6ª tentativa traz uma dica textual | Provável |
| Dicas | Ano, visualizações e dificuldade, visíveis desde o início | Provável |
| Áudio | Recriações feitas em Logic Pro, não stems originais | Confirmado |
| Catálogo | Mais de 1.900 músicas | Fraca |
| Backend | SPA/PWA sem login; Google Analytics e NitroPay; `robots.txt` sem restrições | Parcial |
| Termos (10/10/2025) | Sem cláusula contra scraping ou clientes de terceiros; o áudio tem direitos autorais | Confirmado |

### O que falta, e só dá para fazer na máquina local

O ambiente da nuvem bloqueou `bandle.app` e `proxy.golang.org`.

1. Abrir o Chrome em `https://bandle.app/daily` → DevTools → Network → marcar Preserve log e Disable cache → jogar uma partida inteira, com um pulo → **Save all as HAR with content** → salvar como `bandle.har` na raiz (já está no `.gitignore`).
2. Rodar `go run ./cmd/harscan -har bandle.har`, que gera `docs/bandle-har-report.md` e `testdata/bandle/*.json`.
3. Rodar `go run ./cmd/probe -url <URL de mídia>` para cada faixa listada no relatório.
4. Rodar `go get github.com/gopxl/beep/v2@latest && go run -tags audio ./cmd/probe -url <faixa 1> -play`.
   - `play_audio.go` **nunca foi compilado**. Conferir as assinaturas do beep v2 caso dê erro.
5. Na aba Sources do DevTools, buscar no `main*.js` por `fetch(`, `firebase`, `.mp3`, `answer` e `2022-08`.
6. Preencher as células marcadas "_preencher_" em `docs/bandle-protocol.md`.
   - Perguntas em aberto: endpoint do puzzle, stem por instrumento ou mix por estágio, formato do áudio e suporte a Range, entrega do catálogo, se a resposta vem no payload ou é validada no servidor, headers/CORS/rate limit, e o texto de compartilhamento.
7. Confirmar que o número do puzzle no payload bate com `go run ./cmd/probe`.

## 6. Estado do código

- `go.mod`: `module fera-games`, `go 1.26`.
  - Os testes foram rodados com Go 1.24 (trocando temporariamente a diretiva) e passaram: `internal/games/bandle`, `internal/har`, `cmd/probe`.
- Só stdlib, exceto `cmd/probe/play_audio.go` (tag `audio`, depende do beep v2).
- Git: branch `main`, 1 commit.
  - Sem remote: nenhuma conta GitHub estava conectada.
  - Para publicar: criar um repo privado vazio e rodar `git remote add origin git@github.com:<usuario>/fera-games.git && git push -u origin main`.

## 7. Riscos

| Risco | Mitigação |
| --- | --- |
| API não documentada muda | Provider isolado, testes com `testdata/`, erro claro na TUI |
| Resposta validada só no servidor, ou payload ofuscado | Engine aceita validação remota; avaliar o custo antes da Fase 2 |
| Áudio falha em WSL, SSH ou container | `--mute` e mensagem de orientação |
| Faixas dessincronizadas | Mixer do beep com posição compartilhada |
| Rate limit | Cache do puzzle e do catálogo, 1 download por dia |
| Spoiler no cache local | Guardar só o hash da resposta quando possível |

## 8. Próximo passo sugerido para a sessão de código

1. Fazer a captura (§5), preencher `docs/bandle-protocol.md` e commitar `testdata/`.
2. Com o contrato conhecido, iniciar a Fase 1 (esqueleto com `cmd/fera` e Bubble Tea v2) e a Fase 2 (engine + provider testados com `httptest`) em paralelo.

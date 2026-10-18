# Plano técnico — Songless no Fera Games

Base: mesma arquitetura do Bandle (`plan.md:16-24`):
`TUI → Engine → Provider + Audio + Store`. `internal/game/game.go:10-23` não muda.

## 1. O que o spike do bundle + API ao vivo já provou

| Item | Valor verificado |
|---|---|
| Puzzle do dia | Raspar `GET lessgames.com/songless` (HTML) → flight-data `{"categories":[{categoryId,fileUrl}],"date"}`. Ex. observado: `date=2026-09-27`, `A101.aac / B137.aac / C040.aac` |
| Categorias | `SONGLESS_CATEGORIES=[All:0, Rock:1, Hip Hop:2]` (fixo no JS) |
| Timesteps | `SONGLESS_TRACK_TIMESTEPS=[0.13, 0.53, 2, 8, 15]`s — 5 rounds (`roundIndex 0..4`) |
| Áudio | 1 arquivo `.aac` por categoria (`https://cdn.lessgames.com/songs/*.aac`, `audio/aac`, CORS `*`, ~692 KB, ADTS AAC-LC 48 kHz estéreo). Player toca do zero e **para em `steps[index]`** (timeout + rAF no site) |
| Busca | `GET api.lessgames.com/songless/search?query=` → `[{id,artist,track}]`, sem auth (testado) |
| Chute (server-side) | `POST api.lessgames.com/songless/guess {challengeDate, categoryId, roundIndex, songId\|null}` → `{state, answer?}`. `state=correct\|present\|incorrect\|skipped`; `answer={artist,track,sc}` só no acerto ou no 5º erro/skip. Skip = `songId:null`. Sem auth (testado em 27/09: All=`Ain't No Mountain…`, Rock=`Fortunate Son`, HipHop=`WHATS POPPIN`) |
| Erros | `DATE_NOT_ALLOWED` (dia futuro — UTC 28/09 ainda rejeita, server está em 27/09), `SONG_NOT_FOUND`, `VALIDATION_ERROR` (`roundIndex<=4`) |
| Stats | `GET /songless/stats?challengeDate&categoryId` → `{daily:{distribution[]}}`, público |
| Nº do challenge | `daysBetween(challengeDate, 2025-08-29)+1` (para o share) |
| Share | `SONGLESS_SHARE{guesses,timeTaken=steps[i],found,categoryName,challengeNumber}` + emoji-row + invite `https://less.gg…` (template exato pendente — capturar no browser) |

Diferença central vs Bandle: **julgamento é no servidor** (`protocol.go:59-63` do Bandle é local). Sem rede não dá para chutar nem rejulgar.

## 2. Novos arquivos (espelho do Bandle)

```
internal/games/songless/protocol.go  # consts, Puzzle/Category/Entry/Outcome, ChallengeNumber, ShareText
internal/games/songless/client.go     # Provider real: page→daily, search, guess, stats, áudio CDN + httpx.Cache
internal/games/songless/engine.go     # Game/Session (game.Session), Snapshot/Restore
internal/tui/songlessui/              # model.go, view.go, keys.go (clone de bandleui/model.go:73-98)
cmd/probe/  +flags -songless...       # spike reutilizável (cf. docs/bandle-protocol.md:108-117)
internal/games/songless/fixture/       # respostas mínimas (page, guess, search, stats, AAC curto) em Go
docs/songless-protocol.md             # contrato Fase 0
```

Provider proposto (cf. `client.go:21-25` do Bandle):

```go
type Provider interface {
    Daily(ctx context.Context, day time.Time) (Daily, error) // page scrape; Daily{Date, Categories[{ID,Name,FileURL}]}
    Search(ctx context.Context, query string) ([]Entry, error)
    Guess(ctx context.Context, g Guess) (Outcome, *Answer, error) // Guess{Date, CategoryID, RoundIndex, SongID *string}
    Audio(ctx context.Context, c Category) (io.ReadCloser, error) // GET fileUrl, cache em disco
    Stats(ctx context.Context, date string, categoryID int) (Stats, error)
}
```

`Session`: 1 categoria = 1 partida (5 tentativas). `State()` reaproveita `game.State`; `AudioStep()` = `min(step,5)`; limite de reprodução = `Timesteps[step-1]`.

## 3. Reuso por camada

- **game/store/httpx**: sem mudança. `store.Store` já é genérico por `gameID` (`store.go:38-41`); usar `songless.json`. **Atenção**: 3 partidas/dia — chave `Match.Day` precisa virar `YYYY-MM-DD#cat` ou um `Data` com as 3 sessões.
- **áudio**: `Player` (`player.go:13-33`) e `deck` reusados, mas `Decode` (`track.go:31`) só faz mp3. Fase 2 = decodificador AAC puro-Go (candidatos: `skrashevich/go-aac`, `tphakala/go-aac`, `thesyncim/goaac` — validar contra o `A101.aac` real; resample 48→44.1 kHz já existe). Limite de `steps[i]`: TUI pausa quando `Position() >= limit` no tick de 100 ms (mesmo padrão `bandleui/model.go:170-171`), em vez de `setTimeout` do site. Barra segmentada com largura `∝ s/15` (o site faz `width:100*s/15`).
- **tui**: clonar `bandleui` (player, grade, `suggest()` literal+fuzzy `model.go:427-452`, `save/restore`, `ensureAudio`, atalhos `espaço/←→/tab/enter/c/q`). Trocar: autocomplete via **API remota** (debounce + `searching` spinner, não catálogo local); sem `seek ±5s` fora do limite; adicionar tabs de categoria `All/Rock/Hip Hop`; célula `present` = artista certo (amarelo, mostra só track).
- **app**: `app.go:22-26` já lista `songless`; estender `Deps` + `openSongless()` espelhando `openBandle()` (`app.go:120-142`), mesmo `OpenPlayer`/`--mute`.

## 4. Fases

| Fase | Escopo | Pronto quando |
|---|---|---|
| 0. Spike | `probe -songless-today/-songless-search/-songless-guess/-songless-play`: baixa page, extrai `date+fileUrl`, busca, chuta skip até revelar `answer`, toca o `.aac` decodificado | Áudio do dia toca e corta em 0.13 s / 15 s |
| 1. Engine+Provider | `protocol/client/engine` + `engine_test` (vitória, artista-certo, derrota em 5, skip) e `client_test` com httptest + fixtures | `go test ./...` passa sem rede |
| 2. Áudio AAC | `audio.DecodeAAC` + teste com `A101.aac` real; `SetLoop` opcional | Troca de round recomeça do zero sem estalo, para no timestep |
| 3. Tela | `songlessui` com provider fake (padrão `bandletest`); busca remota com debounce | Puzzle do dia jogável de ponta a ponta num pty |
| 4. Polimento | Persistência por categoria, `DATE_NOT_ALLOWED` → msg amigável + `r` retry (cf. `bandleui` `friendly`), share (após capturar template), `make build` | Reabrir no dia restaura; `c` copia resultado |

## 5. Riscos

- **AAC sem decoder no `beep`**: spike da Fase 2 decide a lib; fallback = `ffmpeg` externo (quebra binário único — evitar).
- **ffmpeg (fallback) + Makefile**: o ffmpeg é dependência **condicional** — só passa a ser exigido se o spike do decoder puro-Go falhar. Nesse caso, o `Makefile` ganha (seguindo o padrão dos alvos `lint`/`snapshot` em `Makefile:46-50,72-76`, que instalam a ferramenta se faltar):
  - `make ffmpeg` (ou `make audio-deps`): verifica `command -v ffmpeg`, imprime a versão e falha com instrução de instalação por SO (`apt install ffmpeg` / `brew install ffmpeg` / `winget install ffmpeg`) se ausente;
  - os alvos de áudio do Songless (`play-songless`, `walk-songless`, contraparte de `Makefile:63-69`) passam a depender desse alvo — mas **só no modo fallback**, via variável (ex.: `SONGLESS_AUDIO=ffmpeg` opta pelo caminho externo; o padrão continua puro-Go);
  - `make check` **não** passa a exigir ffmpeg — só o caminho de reprodução Songless-ffmpeg, para não quebrar quem usa o decoder embutido nem a CI atual.
- **Snapshot não rejulgável**: `bandle/engine.go:131-154` rejulga localmente; Songless depende do servidor → `Restore` valida `len(board)<=5` e confia nos `states` salvos + `answer` (só exibir após fim; nunca usar para validar).
- **Virada do dia / fuso**: server ainda em 27/09 com UTC em 28/09 → usar data da page (`date`), não `time.Now()`, como `challengeDate`; `DATE_NOT_ALLOWED` = `ErrNoPuzzle`.
- **Page parsing frágil**: flight-data pode mudar → isolar em `Daily()` com teste contra a fixture da página; erro claro na TUI.
- **Rate limit / ToS**: 1 download de áudio/dia + cache (igual Bandle); guess é 1 req/tentativa (máx 15/dia nas 3 categorias).

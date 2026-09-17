# Fera Games — handoff para a sessão de código

Atualizado em 2026-09-17 (PoC do Bandle completa: Fases 0 a 5). Plano completo: https://claude.ai/code/artifact/5bd1dcf1-d389-491a-a513-994b3c7f65d8

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
cmd/fera/           ✅  main: flags --mute, --data-dir, --version; roda o app
cmd/probe/          ✅  spike: baixa/decifra qualquer arquivo do Bandle (-file), inspeção de URL, tocar (-play) e passear pelos estágios (-walk)
cmd/harscan/        ✅  spike: HAR do DevTools → relatório de endpoints + testdata (não foi preciso usar)
internal/app/       ✅  router (menu ↔ Bandle), Config, Deps injetáveis (jogo, abertura do áudio), lista de jogos do hub
internal/tui/       ✅  theme, menu, bandleui (tela do Bandle: player, grade, dicas, input com autocomplete)
internal/game/      ✅  interfaces Game, Session, State, Result e Info (menu)
internal/games/bandle/  ✅  protocol.go (endpoints, tipos, Decrypt, Judge, MergeAnswer), client.go (Provider real: auth anônimo, URL assinada, Referer, cache), engine.go (Game/Session)
internal/audio/     ✅  Track (mp3 → memória), Player (play/pause/seek/loop, Swap com crossfade de 20 ms), NewSilent para --mute/sem dispositivo
internal/store/     ✅  Store: <DataDir>/<jogo>.json com as partidas por dia (Match com Data = Snapshot do jogo) e Stats (jogadas, vitórias, sequências)
internal/httpx/     ✅  Client (timeout, User-Agent, StatusError) e Cache em disco (Get/Fresh/Put atômico); sem retry por enquanto
internal/har/       ✅  parser/classificador de HAR
testdata/bandle/    ✅  payloads reais de 2026-09-16: puzzle (cifrado e decifrado), catálogo, details, bonus
```

## 3. Stack

| Camada | Escolha |
| --- | --- |
| TUI | Bubble Tea v2: `charm.land/bubbletea/v2`. `View()` retorna `tea.View`; teclas chegam como `tea.KeyPressMsg` com `key.Code`, `key.Text` e `key.Mod` |
| Estilo e componentes | Lip Gloss v2, Bubbles v2 (`textinput`, `list`, `progress`, `spinner`, `help`) |
| Áudio | `github.com/ebitengine/oto/v3` + `github.com/gopxl/beep/v2` v2.1.1 (mp3). **No Linux o oto usa cgo + ALSA** (`libasound2-dev`, `pkg-config`) e isso vale para compilar o projeto inteiro desde a Fase 3; macOS e Windows são sem cgo. Não precisa de mixer: cada estágio é um mp3 já mixado; `audio.Player.Swap` troca a faixa na mesma amostra com crossfade curto |
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
| 0. Spike de protocolo | Mapear puzzle, catálogo e áudio do Bandle | `probe` baixa o puzzle de hoje e toca a faixa 1 | **Feito** (verificado tocando a faixa 1 de #1491) |
| 1. Esqueleto | `cmd/fera`, menu "Fera Games", CI com lint + test | Binário abre o menu e sai com `q` | **Feito** (teatest cobre o critério; CI em `.github/workflows/ci.yml`) |
| 2. Engine + Provider | Regras do Bandle e cliente HTTP, sem TUI | Partida simulada em teste: vitória, derrota e pulos | **Feito** (`engine_test.go`: vitória com pulo e artista certo, derrota em 6, catálogo; `client_test.go`: fluxo completo com httptest, cache, 403 → ErrNoPuzzle; `TestLive` opcional com `FERA_LIVE=1`) |
| 3. Áudio | Tocar o mp3 do estágio e trocar de estágio | Novo estágio recomeça do início e toca, sem estalo (como no site) | **Feito** (`deck` testado sem dispositivo; `probe -walk` verificado no ALSA) |
| 4. Tela de jogo | Engine + áudio + autocomplete | Bandle do dia jogado de ponta a ponta | **Feito** (`bandleui` testado com provider fake; fluxo real verificado num pty contra produção: carregar, pular, chutar, voltar) |
| 5. Polimento | Estado persistente, erros, `--mute` | Reabrir no mesmo dia mostra a partida salva; build v0.1.0 local | **Feito** (restauração verificada num pty contra produção; `make build VERSION=v0.1.0` e `make snapshot VERSION=v0.1.0`) |

Depois da PoC:
1. Extrair o que é comum para `internal/game` e `internal/tui`.
2. Termo (term.ooo): grade 6×5, sem áudio.
3. Songless: reaproveita o áudio e o autocomplete.
4. Hub com status do dia, sequência e estatísticas.

## 5. Estado da Fase 0 — concluída

Contrato completo em `docs/bandle-protocol.md`. Resumo do que a Fase 2 precisa saber:

| Item | Valor |
| --- | --- |
| Auth | Login anônimo do Firebase Auth (`accounts:signUp`, apiKey pública) → idToken (1 h) |
| URL assinada | `GET cloudfunctions.net/getSignedUrl?file=<path>` com `Authorization: Bearer` → `{url, utc}`, url expira em 60 s |
| Download | `GET url` com **`Referer: https://bandle.app/daily`** (sem ele, 403) |
| Puzzle | `/v2/planning/YYYY-MM-DD.txt` (data local) → hex XOR → `[Puzzle]`; `id` bate com `PuzzleNumber` (confirmado: #1491) |
| Resposta | Vem no payload (`song`, `sources`); validação local (`bandle.Judge`) |
| Catálogo | `/v2/guesslist/songs.txt` → 2 576 itens `{i,t,s}`; a resposta pode faltar → `bandle.MergeAnswer` |
| Áudio | `/v2/files/<path>/<1..5>.mp3`: mix acumulado por estágio, MP3 64 kbps 44,1 kHz ≈25 s, `Range` ok; estágio 6 é a dica textual |
| Compartilhar | `Bandle #N step/6` + emojis `🟩🟨🟥⬛` + `Found: x/y (pct%)` + `#Bandle` |

No Linux o áudio exige `libasound2-dev` e `pkg-config` (oto v3 usa cgo+ALSA); a CI instala os dois.

## 6. Estado do código

- `go.mod`: `module fera-games`, `go 1.26` (toolchain baixado automaticamente; máquina local tem Go 1.25.3).
  - `go test ./...` passa: `internal/games/bandle` (numeração, decifragem, regra de acerto com payload real), `internal/har`, `cmd/probe`.
- Dependências: `charm.land/bubbletea/v2 v2.0.9`, `charm.land/bubbles/v2 v2.2.1`, `charm.land/lipgloss/v2 v2.0.6`, `github.com/charmbracelet/x/exp/teatest/v2` (testes), `github.com/gopxl/beep/v2 v2.1.1` (só em `cmd/probe/play_audio.go`, tag `audio`).
- API da Fase 2 que a TUI vai usar: `bandle.NewGame(bandle.NewClient(httpx.New(30*time.Second), &httpx.Cache{Dir: cfg.DataDir+"/cache"}))`; `Game.Start(ctx, day)` devolve `*Session` com `Puzzle()`, `Catalog()` (já com a resposta), `Guess`/`Skip`, `Stem(ctx)` (mp3 do `AudioStep()`), `UnlockedInstruments()`, `Clue(lang)`, `Outcomes()`, `ShareTextWith(Stats)`. Erros: `ErrUnknownSong` (chute fora do catálogo, não consome tentativa), `ErrFinished`, `ErrNoPuzzle`.
- Cache em disco guarda os `.txt` ainda cifrados (spoiler só em memória) e os mp3; puzzle do dia nunca é rebaixado, catálogo a cada 24 h.
- API da Fase 3: `audio.Open()` (dispositivo real; erro → usar `audio.NewSilent()`), `audio.Decode(io.Reader) (*Track, error)`, `Player.Load/Swap/Play/Pause/Toggle/Seek/SeekTo/Position/Duration/Playing/SetLoop/Silent/Close`. O `speaker` do beep é inicializado uma vez por processo (44,1 kHz, buffer de 100 ms) e o `deck` fica instalado nele para sempre: pausado gera silêncio. A TUI deve fazer polling de `Position()` num tick (~100 ms).
- O player mudo avança a posição pelo relógio, então a barra de progresso funciona igual com `--mute`.
- Fase 4, `internal/tui/bandleui`: dois modos de teclado. **Player**: `espaço` toca/pausa, `←/→` ±5 s, `tab` pula, `enter` ou `/` ou qualquer letra entra em digitação, `c` copia o resultado (OSC52, só no fim), `?` ajuda, `q`/`esc` menu (pausa o áudio), `ctrl+c` sai. **Digitação**: letras vão para o `textinput`, `↑/↓` escolhem a sugestão, `tab` completa, `enter` chuta (sugestão selecionada, ou texto exato se estiver no catálogo), `esc` volta. Sugestões: substring literal primeiro (mais curta antes), fuzzy (`sahilm/fuzzy`) completa até 6. Áudio: `ensureAudio` pede o mp3 de `Session.AudioStep()` via `StemAt` num `tea.Cmd` e chama `Player.Restart` (novo estágio recomeça do zero e toca, com fade da faixa anterior — igual ao site, que pausa e recomeça; `Swap`, que mantém a posição, continua disponível no pacote); respostas atrasadas de estágios que já passaram são ignoradas. Tick de 100 ms redesenha a barra; o app não encaminha mensagens a telas inativas, então `Resume()` religa o tick ao voltar do menu.
- `internal/app`: `Deps{Bandle, OpenPlayer, Now}`; `DefaultDeps(cfg)` cria o `Client` com cache em `DataDir/cache` e escolhe `audio.Open` ou `NewSilent` (`--mute`). O dispositivo abre na primeira partida; se falhar, o jogo segue mudo com aviso na tela. Voltar ao menu mantém a partida em memória (`enter` retoma).
- Testes de TUI que executam `tea.Cmd` devem descartar cmds lentos (ticks, blink do cursor a 530 ms): ver `run`/`drain` em `bandleui/model_test.go` e `app_test.go`. Provider fake compartilhado: `internal/games/bandle/bandletest`; mp3 sintético: `internal/audio/audiotest`.
- Fase 5: `bandle.Session.Snapshot()/Restore()` — o snapshot guarda só chutes e códigos de resultado (nunca a resposta) e `Restore` **rejulga** cada chute contra o puzzle, então um arquivo editado não vira vitória. `bandleui` salva a cada jogada (`store.Match{Data: Snapshot}`), restaura em `loadedMsg` (o áudio já carrega no estágio certo) e, ao terminar, lê `Store.Stats` para as linhas "Found"/"Current streak" do compartilhamento. Sequência: dias consecutivos com vitória, válida só se o último dia jogado for hoje ou ontem. Erros na carga viram frases amigáveis (`friendly`: sem rede, timeout, `ErrNoPuzzle`) com `r` para tentar de novo.
- Arquivos do usuário: `os.UserConfigDir()/fera-games/bandle.json` (partidas) e `.../cache/` (payloads cifrados e mp3). `--data-dir` troca a raiz.
- Lint: `.golangci.yml` (formato v2; `misspell` desligado porque os comentários são em português). Localmente o `golangci-lint` v1 do PATH não roda com `go 1.26`: use `GOTOOLCHAIN=go1.26.0 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`.
- GoReleaser: `.goreleaser.yaml` com `release.disable: true`; `make snapshot VERSION=v0.1.0` gera os 5 binários em `dist/` (Linux amd64 nativo com cgo/ALSA; darwin e windows amd64/arm64 cruzados sem cgo). Sem tag git a versão vem de `FERA_VERSION`; com tag, do `git describe`. O goreleaser exige Go ≥ 1.27 e o Makefile instala com `GOTOOLCHAIN=auto`.
- Git: branch `main`.
  - Sem remote: nenhuma conta GitHub estava conectada.
  - Para publicar: criar um repo privado vazio e rodar `git remote add origin git@github.com:<usuario>/fera-games.git && git push -u origin main`.

## 7. Riscos

| Risco | Mitigação |
| --- | --- |
| API não documentada muda | Provider isolado, testes com `testdata/`, erro claro na TUI |
| Chave de decifragem ou apiKey mudam no bundle | `TestEncryptionKey` quebra; refazer a extração (§5 de `bandle-protocol.md`) |
| Áudio falha em WSL, SSH ou container | `--mute` e mensagem de orientação |
| Troca de estágio com estalo | Arquivos têm a mesma duração: trocar a fonte e `Seek` para a mesma posição |
| Rate limit | Cache do puzzle e do catálogo, 1 download por dia |
| Spoiler no cache local | O payload traz a resposta em claro; guardar o arquivo ainda cifrado e decifrar em memória |

## 8. Próximo passo sugerido para a sessão de código

A PoC está completa. Antes de seguir: commitar (o histórico está todo no working tree), criar o repo privado e a tag `v0.1.0`.

Depois da PoC (ordem do §4):
1. Extrair o comum para `internal/game`/`internal/tui`: a grade de tentativas, o input com autocomplete e o fluxo salvar/restaurar são reutilizáveis; `store` já é genérico por `gameID`.
2. Termo (term.ooo): mapear o protocolo com o mesmo método da Fase 0 (bundle da SPA + `probe`).
3. Songless: reaproveita `audio` e o autocomplete.
4. Hub: mostrar no menu o status do dia e a sequência a partir de `store.Stats`.

Pendências pequenas do Bandle: loop do áudio (`Player.SetLoop`), idioma da dica (`Deps.Lang`), rodadas bônus, telemetria opcional (nenhuma é necessária para jogar).

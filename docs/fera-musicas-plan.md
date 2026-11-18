# Plano da POC: "Fera Músicas", um jogo próprio no estilo Songless com catálogo cadastrável

> Nome no menu: **Fera Músicas**. Id e pacote Go: `feramusicas` (pacotes Go não aceitam hífen nem acento).

## Status: implementado (fases 1 a 4)

Tudo abaixo foi feito. A Fase 0 (download ao vivo de um link real) **ainda precisa ser rodada
numa máquina com `yt-dlp` e `ffmpeg`**: `make musicas-probe URL=https://youtu.be/<id>`
(`PROBE_ARGS=-yt-play` toca os 5 rounds). O fluxo completo foi validado no binário real com
`yt-dlp`/`ffmpeg` falsos no `PATH`.

Diferenças em relação ao plano original:

- **Digitar o chute** começa com `enter` ou `/`, e não ao teclar qualquer letra como no Songless:
  `c` (copiar) e os dígitos de categoria são teclas de comando.
- **Sem tecla `r` no jogo** (a seção 4 previa uma): sair da categoria e voltar (outra aba, ou `q` e
  `enter` no menu) já baixa o áudio de novo. Sem `yt-dlp`/`ffmpeg`, a tela pede para instalar e reabrir o
  programa, porque as ferramentas são procuradas na abertura. O `r` só existe na tela de erro ao
  abrir o catálogo.
- **Abas de categoria** ficam numa linha própria e o rótulo encolhe conforme a quantidade
  (até 9, teclas `1`-`9`).
- **Autocomplete** usa o catálogo inteiro, não só a categoria aberta (senão uma categoria pequena
  entregaria a resposta). `suggest` foi extraído para `internal/tui/suggest` e o Bandle passou a usá-lo.
- **Acerto por nome**: duas músicas cadastradas com o mesmo título valem a mesma resposta.
- **Cadastro em sequência**: ao salvar, o formulário limpa nome e link e mantém a categoria.
- `game.State.Number` fica em 0 (não há "puzzle #N"); a tela mostra a data.
- Se o `yt-dlp`/`ffmpeg` faltam, o jogo abre mesmo assim (`UnavailableSource`): o cadastro funciona e
  o player explica o que instalar.
- O cancelamento usa `WaitDelay` no `exec`: o `yt-dlp` pode deixar um processo filho segurando o
  pipe, e sem isso o timeout não destravava.
- **Corte do round exato**, em vez do "pausa no tick de 100 ms" da seção 4: o player recebe a faixa
  já cortada no limite (`audio.Track.Head`) e para sozinho no último sample. Pelo tick, somando o
  buffer de 100 ms do dispositivo, o round 1 (0,13 s) chegava a tocar ~0,3 s.
- **Só a resposta fica congelada**: a cada mudança no catálogo, as sessões abertas recebem a lista nova
  (`Session.UseSongs`), então músicas cadastradas no meio da partida já valem como chute. A resposta
  continua chutável mesmo se for removida.
- **Estado de áudio por sessão** na TUI (download em andamento, erro e faixa): trocar de aba no meio
  do download não baixa de novo, e o erro de uma categoria não aparece na outra.
- "Vídeo indisponível" só é reconhecido por frases inteiras do `yt-dlp`; `Requested format is not
  available` (falha passageira, resolvida com `yt-dlp -U`) não manda mais remover a música.

### Admin (tecla `a` no menu inicial)

O cadastro saiu do jogo e foi para um **menu administrativo**
([internal/tui/feramusicasadmin](../internal/tui/feramusicasadmin)), aberto pela tecla `a` no menu
inicial. Dentro do jogo não há mais lista nem cadastro (a tecla `m` saiu), porque a lista entrega as
respostas. Com o catálogo vazio, o jogo manda cadastrar no Admin.

- **Lista:** todas as músicas, agrupadas por categoria, com o ID do vídeo e o início do trecho.
  (Uma versão anterior mostrava a última e a próxima data de cada música no sorteio; saiu a pedido.
  `feramusicas.Calendar` continua, usado pelos testes que garantem que o sorteio dá chance igual a
  todas.)
- **Teclas da lista:** `a` cadastra, `e`/`enter` edita a música do cursor, `d` remove (com
  confirmação), `q`/`esc` volta ao menu.
- **Formulário:** `enter` valida os campos e **pergunta antes de gravar** ("Cadastrar “X” em Rock,
  a partir de 1:30? (y/n)"; na edição, "Salvar as alterações: …"). `y`/`enter` grava, e `n`/`esc` volta
  aos campos sem perder nada. `ctrl+l` limpa os quatro campos (e para a prévia). Erros de validação
  aparecem sem perguntar.
- **Edição:** o formulário abre preenchido. Salvar volta à lista com o cursor na música editada.
  `esc` descarta a edição, e um cadastro novo depois dela começa vazio (o rascunho de um cadastro
  novo, ao contrário, fica ao sair e voltar). O catálogo ganhou `Update(id, Song)`: na memória
  mantém o ID e a posição; no Firestore, trocar o link troca o ID do documento (que é o VideoID)
  numa transação que cria o novo com o `createdAt` do antigo e apaga o antigo. Um link de outra
  música dá `ErrDuplicate`, e uma música removida no meio tempo, `ErrNotFound`.
- **Prévia do trecho (`ctrl+p` no formulário):** baixa e toca o trecho do link a partir do início
  digitado, do jeito que o jogo vai tocar (`Game.Preview`, o mesmo `Fetch`), sem salvar a música.
  A linha da prévia mostra o progresso, de onde o trecho começa no vídeo e em que round aquele
  pedaço aparece ("revelado no round N"). Se o link ou o início mudarem depois, ela avisa para ouvir
  de novo. `ctrl+p` também para. Sair do formulário ou salvar para o som, e uma prévia cancelada no
  meio do download não toca depois. O Admin tem um player próprio (o speaker mistura os dois),
  então a prévia não troca o trecho do jogo. Como a fonte é a mesma, o jogo reaproveita o download.
- **Atualização:** ao voltar para o jogo ou para o Admin, o catálogo é recarregado. Partidas em
  andamento continuam; músicas novas passam a valer como chute.

### Sorteio corrigido

O Admin mostrou um bug do sorteio: o FNV-1a sem mistura deixava os dois sorteios (categorias do dia
e música da categoria) correlacionados. Com 4 categorias, **1 em cada 4 músicas nunca era
sorteada**, e com 5 a frequência variava quase 2× entre elas. Agora:

- O hash passa pelo finalizador do SplitMix64 (`score`).
- O `Pick` escolhe por nota: cada música recebe `score("data#song#categoria#VideoID")` e ganha a
  maior, no lugar de `hash % quantidade`. A distribuição fica uniforme, e mexer no catálogo muda
  pouco: cadastrar uma música só troca a do dia se ela vencer; remover uma que não foi sorteada não
  muda nada.
- Os testes cobrem 10 anos com 1 a 7 categorias: toda música é sorteada, e a diferença entre a mais
  e a menos sorteada é de no máximo 1,6×.
- Isso mudou quais músicas caem em cada dia (continua igual em todas as máquinas).

### Início do trecho

O cadastro ganhou um quarto campo opcional, **Início**: de onde o trecho começa no vídeo, para pular
introduções longas ou silêncio (era a evolução prevista no risco "intro longa"). Ele aceita `45`,
`45,5`, `1:30` ou `1:02:03`; vazio começa do 0. O máximo é 12 h, só para barrar erro de digitação.

- Vira `-ss` no `ffmpeg`, antes do `-i`, junto do `-t`: o download pula direto para o ponto e só
  baixa os ~15,5 s do trecho.
- Fica em `Song.Start`, e no Firestore no campo `startSeconds`. Um valor impossível editado à mão
  no console vale como 0.
- O cache de áudio é por vídeo **e** início. Para mudar o início de uma música, remova e cadastre
  de novo (não há edição).
- Início depois do fim do vídeo gera `ErrStartPastEnd`, e a tela pede para corrigir o início.
- A lista do catálogo mostra o início ao lado do ID do vídeo. `make musicas-probe` aceita
  `PROBE_ARGS="-yt-start 1:30"`.

### 3 músicas por dia

O dia tem **3 músicas, uma de cada uma das 3 categorias sorteadas** (no estilo Songless), em vez
de uma por categoria do catálogo. As abas mostram só essas categorias (teclas `1`-`3`), e o
cabeçalho mostra quantas já foram concluídas.

- **Sorteio das categorias** (`feramusicas.DailyCategories`): cada categoria recebe uma nota pelo
  hash de `YYYY-MM-DD#cat#categoria` e ficam as 3 maiores, mostradas em ordem alfabética. O
  resultado é o mesmo em qualquer máquina e não depende da ordem nem da caixa do catálogo. Com
  menos de 3 categorias, entram todas.
- **Catálogo mudando no meio do dia:** remover uma categoria que não foi sorteada não muda nada, e
  cadastrar uma nova troca no máximo uma das 3 (só se ela tirar nota maior). Uma categoria que sai
  do dia perde a partida em andamento.
- **Compartilhar:** com mais de uma música do dia concluída, o `c` copia o resumo
  (`feramusicas.ShareDaily`): `Fera Músicas <data> <acertos>/<total>` e uma linha por categoria.
- O cadastro continua sugerindo todas as categorias, não só as do dia.

### Catálogo no Firestore

O catálogo agora vive no Cloud Firestore
([internal/games/feramusicas/firestorecatalog](../internal/games/feramusicas/firestorecatalog)), com o
cliente oficial `cloud.google.com/go/firestore`. As credenciais vêm de um `.env` (modelo em
[.env.example](../.env.example)), lido de `./.env` e de `<data-dir>/.env`
([internal/app/env.go](../internal/app/env.go)).

- **Credenciais:** conta de serviço por `FIREBASE_CLIENT_EMAIL` + `FIREBASE_PRIVATE_KEY`, ou
  `GOOGLE_APPLICATION_CREDENTIALS` apontando para o JSON. `FIREBASE_PROJECT_ID` é obrigatório.
  `FIRESTORE_DATABASE` e `FIRESTORE_COLLECTION` (padrão `feramusicas_songs`) são opcionais.
- **Documento por música, com o VideoID como ID:** duplicados são barrados pelo próprio Firestore
  (`Create` → `AlreadyExists`), inclusive com dois cadastros ao mesmo tempo. Campos: `name`,
  `category`, `categoryKey`, `url`, `videoId` e `createdAt` (hora do servidor).
- **Mesma música do dia para todos:** com o catálogo compartilhado, `Pick` sorteia igual em todas
  as máquinas.
- **Sem `FIREBASE_PROJECT_ID`**, o app usa o catálogo em memória e a tela avisa. Com uma
  configuração inválida, ele não cai para a memória em silêncio: a tela do catálogo mostra o erro.
- **Leituras:** `ByCategory` e `Categories` filtram o `List` em memória. Abrir uma categoria lê a
  coleção inteira duas vezes, o que é aceitável para um catálogo pequeno.
- **Segurança:** a conta de serviço ignora as regras de segurança do Firestore e dá acesso total ao
  projeto. Quem tem o `.env` pode cadastrar e apagar músicas. Para distribuir o jogo a outras pessoas,
  o caminho seria um login de cliente (Firebase Auth) com regras restringindo escrita.
- **Teste contra o emulador:** `TestEmulador` cobre o CRUD, a ordem, os duplicados (inclusive
  concorrentes) e o jogo sobre o Firestore; roda com `FIRESTORE_EMULATOR_HOST` definido.

## Contexto

O Songless atual ([internal/games/songless](internal/games/songless)) depende do servidor da lessgames: o puzzle, a busca, o julgamento e o `.aac` vêm todos de lá. O objetivo agora é um jogo **próprio** com a mesma mecânica: ouvir um trecho que cresce a cada tentativa (0.13 s → 15 s, em 5 rounds) e acertar a música. A diferença é que **você cadastra as músicas**: nome, categoria e link do YouTube.

Restrições combinadas:
- **Não há blob storage.** O áudio é baixado **no cliente de quem joga**, na hora da partida, a partir do link do YouTube. O download usa **yt-dlp + ffmpeg**.
- **O cadastro fica só em memória** e é feito **só pela tela da TUI**: o catálogo começa vazio e some ao fechar. A interface do catálogo já nasce pronta para trocar a implementação por Firebase depois.
- **Modo: música do dia por categoria.** O sorteio é determinístico pela data, como os outros jogos do hub.

A arquitetura segue a do hub: `TUI → Engine → Catalog + AudioSource + Player` ([internal/game/game.go](internal/game/game.go) não muda).

## 1. Modelo e catálogo (o ponto de troca para o Firebase)

`internal/games/feramusicas/catalog.go`

```go
type Song struct {
    ID       string // gerado no Add (no Firebase vira o doc ID)
    Name     string // o que o jogador precisa acertar, ex.: "Queen - Bohemian Rhapsody"
    Category string // texto livre, normalizado (trim + comparação case-insensitive)
    URL      string // link original colado
    VideoID  string // 11 chars extraídos do link; chave de unicidade
}

type Catalog interface {
    List(ctx context.Context) ([]Song, error)
    ByCategory(ctx context.Context, category string) ([]Song, error)
    Categories(ctx context.Context) ([]string, error) // derivadas das músicas, ordenadas
    Add(ctx context.Context, s Song) (Song, error)    // valida e rejeita VideoID duplicado
    Remove(ctx context.Context, id string) error
}

var ErrDuplicate, ErrInvalid, ErrNotFound = ...
```

- `MemoryCatalog`: `map[string]Song` + `sync.RWMutex`, IDs via contador ou `crypto/rand`. Fica em `memory.go`.
- O `context.Context` e os erros existem desde já para o Firebase entrar só com um `FirestoreCatalog` novo, sem mexer em engine nem TUI.
- `youtube.go`: `ParseVideoID(link string) (string, error)` aceita `youtube.com/watch?v=`, `youtu.be/`, `youtube.com/shorts/`, `music.youtube.com/watch?v=` e `m.youtube.com`. Valida `^[A-Za-z0-9_-]{11}$`.
- O `Add` valida: nome e categoria não vazios, link parseável e VideoID ainda não cadastrado.

## 2. Download do áudio no cliente (yt-dlp + ffmpeg)

`internal/games/feramusicas/source.go`

```go
type AudioSource interface {
    Fetch(ctx context.Context, s Song, max time.Duration) (io.ReadCloser, error) // PCM s16le 44.1 kHz estéreo
}
type YTDLP struct{ YTDLPPath, FFmpegPath string } // LookPath no construtor
```

O fluxo é pensado para baixar **só os ~16 s necessários**, e não o vídeo inteiro:
1. `yt-dlp -f bestaudio --no-playlist -g <URL>` devolve a URL direta do stream (a googlevideo, que é temporária).
2. `ffmpeg -nostdin -loglevel error -t 15.5 -i <streamURL> -vn -f s16le -ar 44100 -ac 2 pipe:1` gera o PCM já na taxa do dispositivo.

Detalhes:
- Os dois processos rodam com `exec.CommandContext`, para que o timeout e o cancelamento ao sair da tela matem os processos.
- O stderr é capturado e vira um erro amigável. Os casos tratados são: binário ausente, vídeo indisponível ou com restrição de idade, "confirm you're not a bot" e timeout.
- Há um cache **em memória** por `VideoID` durante a execução, para que trocar de categoria e voltar não baixe de novo. Não há cache em disco: a ideia é não armazenar a música.
- `NewYTDLP()` devolve `ErrMissingTools` quando `exec.LookPath` falha. Nesse caso a TUI mostra como instalar, e o cadastro continua funcionando.

**Áudio:** um arquivo novo, `internal/audio/pcm.go`, com `DecodePCM(r io.Reader) (*Track, error)`. Ele lê s16le estéreo em 44.1 kHz e monta o `beep.Buffer`, reaproveitando a conversão de amostras e o `sliceStreamer` de [aac.go](internal/audio/aac.go) (vale extrair um helper comum). `Player`, `deck`, `Restart` e `Silent` ([player.go](internal/audio/player.go)) são reusados sem mudança.

## 3. Engine

`internal/games/feramusicas/engine.go` e `protocol.go`

- `Timesteps = []float64{0.13, 0.53, 2, 8, 15}`, `MaxGuesses = 5` e `LimitFor(step)` são copiados de [songless/protocol.go](internal/games/songless/protocol.go). São copiados em vez de importados para os dois jogos não ficarem acoplados.
- `Pick(day time.Time, category string, songs []Song) (Song, bool)`: ~~ordena por `VideoID` e escolhe pelo índice `fnv64(...) % len`~~ (trocado; veja "Sorteio corrigido" acima). O resultado é determinístico pela data.
  - Uma limitação conhecida da POC: cadastrar ou remover músicas muda a música do dia **das sessões ainda não abertas**. Uma sessão já aberta **congela** a música escolhida.
- `Game{catalog Catalog, source AudioSource}` implementa `game.Game`: `ID()="feramusicas"`, `Name()="Fera Músicas"`, e `NewSession` abre a primeira categoria. `Start(ctx, day, category)` devolve `ErrEmptyCategory` se a categoria não tiver músicas.
- `Session` implementa `game.Session`, e o julgamento é **local**:
  - `GuessSong(id)` compara com `answer.ID`, e os resultados possíveis são `correct`, `incorrect` e `skipped`. Não existe `present`, porque não há campo de artista separado.
  - `Guess(text)` resolve pelo nome exato (case-insensitive) dentro do catálogo. `Skip()`, `State()`, `AudioStep()` e `AudioLimit()` seguem a mesma semântica de [songless/engine.go](internal/games/songless/engine.go).
  - `Answer()` só é exposto depois do fim da partida.
  - O `ShareText()` é simples: `Fera Músicas <data> <categoria> 🟥⬜🟩…`.
- `Suggest(query)` faz o autocomplete sobre os `Name` do catálogo. Ele reusa a lógica de `suggest()` (literal + `sahilm/fuzzy`) de [bandleui/model.go:428](internal/tui/bandleui/model.go#L428). Vale mover essa função para um pacote comum (ex.: `internal/tui/suggest`) e usá-la nos dois jogos.
- **Sem persistência na POC.** Como o catálogo some ao fechar, um snapshot salvo apontaria para um ID que não existe mais. O `Store` só entra junto com o Firebase.

## 4. TUI

`internal/tui/feramusicasui/` (model.go, view.go, keys.go)

Há duas telas internas:

**Catálogo (cadastro)**: ~~tela do jogo, aberta pela tecla `m`~~ (foi para o Admin; veja acima).
- Uma lista agrupada por categoria (nome, categoria, videoID). As teclas são `a` (adicionar), `d` (remover com confirmação) e `↑/↓`.
- Um formulário com 3 `textinput` (Nome, Categoria e Link do YouTube). `tab` troca de campo e `enter` salva.
  - O campo Categoria sugere as categorias já existentes.
  - Os erros de validação (`ErrInvalid`, `ErrDuplicate`, link inválido) aparecem embaixo do campo.

**Jogo**: um clone enxuto de [songlessui](internal/tui/songlessui).
- As tabs de categoria são **dinâmicas** (vêm de `Categories()`) e usam as teclas `1..9`, no lugar das tabs fixas All/Rock/Hip Hop.
- O carregamento abre um spinner com "baixando o trecho do YouTube…" (`fetchAudio` chama `source.Fetch` e depois `audio.DecodePCM`) e já pré-baixa a categoria atual ao abrir.
- O player para no `AudioLimit()` no tick de 100 ms, que é o mesmo mecanismo do songlessui.
- O autocomplete é **local** (`Game.Suggest`), sem debounce nem rede.
- Os atalhos são os mesmos: `espaço`, `tab` (pular), `enter` (chutar), `c` (copiar) e `q` (menu).
- Os erros de download mostram a mensagem amigável e a tecla `r` (retry). Quando faltam ferramentas, aparece a instrução de instalação.

**App** ([internal/app/app.go](internal/app/app.go)):
- Uma nova entrada em `Games`, `{ID:"feramusicas", Name:"Fera Músicas", Description:"adivinhe a música pelo trecho — catálogo próprio", Available:true}`.
- Os campos `FeraMusicas *feramusicas.Game` em `Deps` e `screenFeraMusicas`.
- `openFeraMusicas()` espelha `openSongless()` e usa o mesmo `ensurePlayer()`/`--mute`.
- `DefaultDeps` cria um `MemoryCatalog` e um `YTDLP`. Se as ferramentas faltarem, o jogo abre com o source em erro e o cadastro continua usável.

## 5. Dependências e Makefile

- Um alvo novo, `check-media-deps`, que roda `command -v yt-dlp` e `command -v ffmpeg` e entra no `deps`.
  - Ele **só imprime** os comandos de instalação, sem rodar sudo: `sudo apt-get install -y ffmpeg`, e para o yt-dlp `pipx install yt-dlp` ou o binário oficial. O yt-dlp do apt costuma estar velho e quebrar com o YouTube.
  - Ele também sugere `yt-dlp -U` quando o download falhar.
- Um alvo `make musicas-probe URL=...` que chama `go run ./cmd/probe -yt <url>` (spike da Fase 0).
- `make check` e a CI **não** passam a exigir yt-dlp e ffmpeg, porque os testes usam um source fake.

## 6. Fases

| Fase | Escopo | Pronto quando |
|---|---|---|
| 0. Spike | Flag `-yt <url> [-play]` em [cmd/probe](cmd/probe): `ParseVideoID`, depois yt-dlp `-g`, ffmpeg, `DecodePCM` e reprodução, com corte em 0.13 s / 15 s | Um link real toca o trecho e o tempo de download fica medido (meta < 5 s) |
| 1. Catálogo | `Song`, `Catalog`, `MemoryCatalog`, `ParseVideoID` + testes de tabela | `go test ./internal/games/feramusicas/...` cobre links válidos e inválidos, duplicado, remover e categorias |
| 2. Engine | `Pick`, `Game`, `Session`, `Suggest` + `feramusicastest` (catálogo semeado e `AudioSource` fake com PCM curto, padrão [songlesstest](internal/games/songless/songlesstest/songlesstest.go)) | Os testes passam para vitória, derrota em 5, skip, `Pick` estável por data e categoria vazia |
| 3. TUI | Tela de catálogo + tela de jogo + wiring no app/menu | O fluxo cadastrar 3 músicas → jogar → acertar/errar funciona num pty |
| 4. Polimento | Erros amigáveis do yt-dlp, retry, pré-download, `check-media-deps`, share/copiar | `make check` verde, e sem yt-dlp aparece a instrução em vez de um crash |

## 7. Riscos

- **O YouTube muda ou bloqueia** (bot check, throttling, restrição de idade/região). A mitigação é usar o yt-dlp atualizado (`-U`), dar mensagem clara e permitir retry. Isso é aceitável numa POC; num produto vale reavaliar, inclusive pelos termos de uso do YouTube.
- **Latência:** `-g` leva cerca de 1 a 3 s e depois vem o download dos 16 s. A mitigação é pré-baixar ao abrir a tela e manter o cache em memória.
- **Intro longa ou silêncio no começo do vídeo:** o trecho de 0.13 s pode ficar mudo. Fica anotado como evolução um campo opcional `Start` (offset) no cadastro, que vira `-ss` no ffmpeg. Ele fica fora da POC para respeitar o cadastro de 3 campos.
- **Spoiler:** o nome da música não aparece em nenhum lugar da tela de jogo. Na tela de catálogo ele aparece, o que é inevitável com cadastro local.

## Arquivos

Novos:
- `internal/games/feramusicas/{catalog,memory,youtube,source,protocol,engine}.go` + `_test.go`
- `internal/games/feramusicas/feramusicastest/`
- `internal/audio/pcm.go`
- `internal/tui/feramusicasui/{model,view,keys}.go`
- `docs/fera-musicas-plan.md` (cópia deste plano no repo)

Alterados:
- [internal/app/app.go](internal/app/app.go)
- [Makefile](Makefile)
- [cmd/probe/main.go](cmd/probe/main.go)
- opcionalmente, [internal/audio/aac.go](internal/audio/aac.go) (extrair o helper de PCM) e [bandleui/model.go](internal/tui/bandleui/model.go) (extrair `suggest`)

## Verificação

1. `make check`: fmt, vet, test e lint passam sem yt-dlp e ffmpeg (os testes usam um `AudioSource` fake).
2. `TestLive` em `feramusicas`, só quando `FERA_LIVE=1`, como em [songless/live_test.go](internal/games/songless/live_test.go): baixa um link real e confere que o `Track` tem cerca de 15 s.
3. `go run ./cmd/probe -yt "https://youtu.be/<id>" -play` toca o trecho.
4. `make run` → Fera Músicas. A tela de catálogo abre vazia; cadastre 3 músicas em 2 categorias e confira:
   - a tecla `1/2` troca de categoria;
   - o trecho para em cada limite;
   - o autocomplete acha os nomes;
   - acertar e errar 5 vezes revela a resposta;
   - reabrir o app volta ao catálogo vazio, porque o cadastro é só em memória.
5. `make run ARGS=--mute` com o yt-dlp fora do PATH: aparece a instrução de instalação e o cadastro continua funcionando.

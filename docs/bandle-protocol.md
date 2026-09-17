# Protocolo do Bandle (Fase 0)

Status: **completo** em 2026-09-16. Mapeado lendo o bundle da SPA (`/_expo/static/js/web/index-*.js`) e reproduzindo o fluxo com `curl`/`go run ./cmd/probe` contra o puzzle #1491. Faixa 1 e a troca de estágios verificadas com o dispositivo real.

Código que já implementa isto: `internal/games/bandle/protocol.go` (constantes, tipos, `Decrypt`, `EncryptionKey`, `Judge`, `MergeAnswer`) e `cmd/probe` (fluxo HTTP completo). Payloads reais em `testdata/bandle/`.

## 0. Fluxo completo (o que a Fase 2 precisa implementar)

```
1. POST https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=<FirebaseAPIKey>
   {"returnSecureToken":true}                     → {idToken, expiresIn:"3600", localId}
2. GET  https://us-central1-bandle-358421.cloudfunctions.net/getSignedUrl?file=<path>
   Authorization: Bearer <idToken>                → {url, utc}   (url válida por 60 s)
3. GET  <url>   (https://songs.bandle.app<path>?u=…&Expires=…&Key-Pair-Id=…&Signature=…)
   Referer: https://bandle.app/daily              → bytes        (sem Referer: 403)
4. .txt → hex → XOR com a dobra da chave → JSON.   .mp3 → tocar direto.
```

Ordem no jogo: `planning/<dia>.txt` (puzzle) → `guesslist/songs.txt` (catálogo) → `files/<path>/1.mp3` … `5.mp3` conforme avança → opcionalmente `bonus/<path>.txt`.

## 1. Identificação do puzzle diário

| Item | Valor | Confiança |
| --- | --- | --- |
| Numeração | `N = dias desde 2022-08-17` (#1 = 2022-08-18). 2026-09-16 → payload `id: 1491` = `bandle.PuzzleNumber` | **Confirmado** |
| Virada do dia | Meia-noite local: a SPA monta o nome do arquivo com `formatDate(new Date())` (data civil local). Se `serverUtc - now > 10 min` e a data do servidor for outra, ela mostra erro | Confirmado |
| Endpoint do puzzle | `/v2/planning/YYYY-MM-DD.txt` (via URL assinada) | Confirmado |
| Payload | Array JSON com **1** objeto (ver `testdata/bandle/planning-2026-09-16.json`). Campos: `id`, `day`, `folder`, `path`, `song` ("Artista - Título"), `sources` (artistas), `instruments` (6), `clue{de,en,es,fr}`, `par`, `year`, `view`, `stream`, `bpm`, `genre[]`, `frontperson`, `youtube`, `youtubeStart`, `spotifyId`, `appleMusicId`, `wiki_*`, `minVersion` | Confirmado |
| Outros arquivos de planning | `/v2/planning/weekly.txt` (lista da semana), `/v2/planning/onboarding.txt` (tutorial) | Confirmado no bundle, não baixados |
| Cache | `Cache-Control` ausente no planning; ETag presente. A SPA cacheia o dia em storage local | Confirmado |

## 2. Áudio

| Item | Valor | Confiança |
| --- | --- | --- |
| Origem | Recriações feitas à mão (Logic Pro), não stems originais | Confirmado |
| Estágios | `instruments` tem 6 itens; os 5 primeiros têm áudio, o 6º é `"clue"` (dica textual). #1491: `drum`, `[bass] + [electric]`, `strings`, `piano`, `voice`, `clue`. Colchetes marcam nomes de instrumento a traduzir | Confirmado |
| **Stem por instrumento ou mix por estágio** | **Mix por estágio.** `/v2/files/<path>/<step>.mp3`, step 1..5; cada arquivo já contém todos os instrumentos liberados até ali (`Play(step)` toca um só arquivo). `6.mp3` não existe (403). No estágio da dica a SPA reutiliza `5.mp3` | Confirmado |
| Formato | MP3 MPEG-1 Layer III, 64 kbps CBR, 44,1 kHz, estéreo, tag ID3v2.4 (299 bytes), ≈25 s. Os 5 arquivos do dia têm o mesmo tamanho (199 873 bytes) e a mesma duração → trocar de estágio mantendo a posição é trivial | Confirmado |
| Host | `songs.bandle.app` (CloudFront → S3), `Content-Type: application/octet-stream` | Confirmado |
| Suporta `Range` | Sim: `Accept-Ranges: bytes`, `206 Partial Content` com `Content-Range` | Confirmado |
| Loop | A SPA toca em loop opcional (`loopAudio` nas configurações) | Confirmado |

Consequência para a Fase 3: não precisa de mixer; basta um player com "trocar a fonte preservando `position`". O plano original (mixer do beep) fica como fallback desnecessário.

## 3. Catálogo do autocomplete

| Item | Valor | Confiança |
| --- | --- | --- |
| Endpoint | `/v2/guesslist/songs.txt` (skins alternativas: `game`, `anime`, `musicals`, `series`) | Confirmado |
| Entrega | JSON único, cifrado: 359 866 bytes hex → 179 933 bytes JSON, **2 576** itens `{i: id, t: "Artista - Título", s: [artistas]}` | Confirmado |
| Cache | `Cache-Control: max-age=0`, ETag `"3f78…"`. A SPA guarda o catálogo por ETag e revalida com `If-None-Match` (304) no máximo a cada 5 min | Confirmado |
| Resposta no catálogo? | **Nem sempre.** Em 2026-09-16 a resposta não estava na lista; a SPA insere `{t: song, s: sources}` na posição 1000. Reproduzido em `bandle.MergeAnswer` | Confirmado |
| Normalização | A SPA gera `value = lower(t).replace(/[^a-z0-9]/g,"")` para o filtro do autocomplete | Confirmado |

## 4. Validação da resposta

- **A resposta vem no payload** (`song`, `sources`). Toda a validação é local; não há endpoint de chute.
- Regra (`bandle.Judge`):
  1. `lower(trim(chute.t)) == lower(trim(song))` → acerto (`g`, 🟩).
  2. Senão, se algum `sources` do chute bate (case-insensitive) com algum da resposta → artista certo (`y`, 🟨); o jogo continua.
  3. Senão → erro (`r`, 🟥). Pular → `b` (⬛).
  4. Erro ou pulo avança `step`; se `step == len(instruments)` e errou, derrota.
- Chamadas opcionais no chute: callback de estatísticas por música (contagem de chutes por `folder/step/id`, não obrigatório) e `POST https://api.nicelight.games/plays/done` só quando existe `nlgid` em `sessionStorage` (integração externa). Nenhuma das duas afeta o jogo.
- Arquivo auxiliar: `/v2/details/<path>.txt` traz `instruments`, `youtube`, `bpm`, `clue` — só é buscado quando o item da playlist não tem `instruments`. O puzzle diário já vem completo.
- Bônus (pós-jogo): `/v3/bonus/<path>.txt` → array de `{type, data:{question?, answers:[{id, label, value, count, correct?}]}}`, tipos `photo, frontperson, nationality, trivia1, year1, year2, album, tempo, instruments` (ver `testdata/bandle/bonus-*.json`). Fora do escopo da PoC.

## 5. Backend, auth e limites

| Item | Valor | Confiança |
| --- | --- | --- |
| Conta/login | Não há conta, mas há **login anônimo do Firebase Auth** (projeto `bandle-358421`, apiKey pública no bundle). idToken dura 1 h; renovar via `securetoken.googleapis.com` ou refazer o signUp | Confirmado |
| Tipo de backend | SPA Expo/React Native Web em S3+CloudFront (`bandle.app`); Cloud Functions (`getSignedUrl`, vouchers, `getCustomOnboardingSong`); arquivos em `songs.bandle.app` (CloudFront assinado, origem S3). Firebase Remote Config/Installations e Bugsnag também aparecem no bundle mas não são necessários | Confirmado |
| Headers obrigatórios | `Authorization: Bearer <idToken>` no `getSignedUrl`; **`Referer: https://bandle.app/…`** no download (CloudFront Function devolve 403 sem ele; `Origin` sozinho não basta). `User-Agent` livre | Confirmado |
| CORS | `access-control-allow-origin: *` nos arquivos | Confirmado |
| Rate limit | Nenhum observado em ~30 requisições; URL assinada expira em 60 s, então pedir uma nova por download | Provável |
| Terceiros | Google Analytics, Meta Pixel, NitroPay, Bugsnag — todos dispensáveis | Confirmado |
| robots.txt | Sem restrições | Confirmado |

Armadilha de decifragem: a "criptografia" é XOR de cada byte com `fold(xor, bytes(chave))`. A chave (`bandle.EncryptionKey()`, um UUID) é reconstruída a partir de três blocos ofuscados em base 36 no bundle; se o site trocar os blocos, o teste `TestEncryptionKey` quebra e aponta o lugar.

## 6. Regras e compartilhamento

- 6 tentativas (`len(instruments)`); pular gasta uma tentativa e libera o próximo instrumento; a 6ª mostra `clue[lang]`.
- Dicas visíveis desde o início: `year`, `view`/`stream` (popularidade), `par` (meta de tentativas, estilo golfe), `genre`.
- `par` também define o "par −N" do resultado (`par - step`).
- Texto de compartilhamento (reproduzido do bundle):

```
Bandle #<id> <step ou "x" se perdeu>/<len(instruments)>
<um emoji por tentativa: 🟩 g, 🟨 y, 🟥 r, ⬛ b, ⬜ w>
Found: <ganhas>/<jogadas> (<pct>%)
Current streak: <n> (max <m>)        ← só se maxStreak > 0
Bonus rounds: <acertos>/<total> 🖼️ 🧑 🌍 …   ← só se jogou bônus
#Bandle <hashtag do puzzle, se houver>
```

Exemplo montado pela regra acima (jogo ganho na 3ª com um pulo, primeira partida): `Bandle #1491 3/6\n⬛🟨🟩\nFound: 1/1 (100%)\n#Bandle `. Os rótulos ("Found", "Current streak", "Bonus rounds") vêm do i18n da SPA; o cliente pode usar o inglês.

## 7. Termos de uso

- Termos de 10/10/2025 (SAS BANDLE, Paris) não têm cláusula sobre scraping, acesso automatizado ou clientes de terceiros.
- O áudio segue protegido por direito autoral. Decisão do projeto: uso pessoal, repo privado, sem redistribuir áudio, `User-Agent` identificável, 1 download por dia com cache.
- A apiKey do Firebase e a chave de decifragem são públicas (estão no bundle servido a qualquer visitante); mesmo assim o repo continua privado.

## 8. Como reproduzir / o que falta

```
go test ./...                                        # inclui decifragem e regra de acerto com payload real
go run ./cmd/probe -file today                       # puzzle de hoje sem spoiler; confere id == PuzzleNumber
go run ./cmd/probe -file today -spoiler              # payload inteiro
go run ./cmd/probe -file stem:1                      # headers + formato da faixa 1
go run ./cmd/probe -file /v2/guesslist/songs.txt     # catálogo
sudo apt install libasound2-dev pkg-config           # Linux: oto v3 usa cgo+ALSA (macOS/Windows são sem cgo)
go run ./cmd/probe -file stem:1 -play                # tocar a faixa 1
go run ./cmd/probe -walk 4s                          # trocar de estágio mantendo a posição
```

`cmd/harscan` continua útil se um dia for preciso capturar um HAR (ex.: mudança de protocolo), mas não foi necessário: o bundle não é ofuscado além dos nomes.

## Fontes

- Bundle: `https://bandle.app/_expo/static/js/web/index-1bb4ac4c050ee0c8721b8c45381b5cb4.js` (2026-09-16)
- https://bandle.app/terms.html
- https://bandle.app/privacy.html
- https://online.berklee.edu/takenote/bandle-creator-johann-levy-on-the-music-quiz-100k-daily-players-love/
- https://www.nbcnews.com/tech/tech-news/bandle-song-guessing-game-builds-buzz-online-rcna149012
- https://www.ggrecon.com/word-games/bandle-answer-today-hints/

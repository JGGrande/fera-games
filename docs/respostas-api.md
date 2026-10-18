# Respostas das APIs (Bandle e Songless)

Referência das respostas reais observadas, no lugar do antigo `testdata/`. Os
testes usam versões mínimas no mesmo formato, em código:
`internal/games/bandle/fixture` e `internal/games/songless/fixture`.
Valores abaixo são de 2026-09-16 (Bandle) e 2026-09-27 (Songless).

## Bandle

Arquivos do CDN vêm **cifrados** (hex de XOR com a máscara da chave; ver
`bandle.Decrypt`). Abaixo, o conteúdo já decifrado. Fluxo completo e
endpoints em [bandle-protocol.md](bandle-protocol.md).

### `/v2/planning/2026-09-16.txt` (puzzle do dia)

Array com **um** objeto.

```json
[{
  "id": 1491, "day": "2026-09-16", "folder": "202608/Only",
  "song": "Sam Smith - I'm Not the Only One",
  "path": "253ab864e93431dfa181",
  "instruments": ["drum", "[bass] + [electric]", "strings", "piano", "voice", "clue"],
  "youtube": "nCkpzqqog4k", "youtubeStart": 228,
  "view": 1800, "year": 2014, "par": 2, "bpm": 82, "genre": ["Soul"],
  "clue": {"de": "...", "en": "I Am Not the Sole Individual", "es": "...", "fr": "..."},
  "wiki_en": "I%27m_Not_the_Only_One", "wiki_fr": "...", "wiki_es": "...",
  "spotifyId": "7795WJLVKJoAyVoOtCWqXN", "stream": 2600, "appleMusicId": "1440814433",
  "frontperson": "Sam Smith", "sources": ["Sam Smith"], "minVersion": "8.6.2"
}]
```

- `id` = número do puzzle (`PuzzleNumber(day)`); `path` monta as faixas:
  `/v2/files/<path>/<n>.mp3`, uma por estágio (6 estágios aqui).
- `song` é "Artista - Título"; `sources` são os artistas.

### `/v2/guesslist/songs.txt` (catálogo do autocomplete)

Array de ~2,5 mil itens, ordenado por título. `i` = índice, `t` = título,
`s` = artistas.

```json
[{"i": 1, "t": "(G)I-DLE - Nxde", "s": ["(G)I-DLE"]},
 {"i": 4, "t": "*NSYNC - Bye Bye Bye", "s": ["*NSYNC"]}]
```

A resposta do dia **não vem** no catálogo; `MergeAnswer` a insere (posição
1000, ou no fim se o catálogo for menor).

### `/v2/details/<path>.txt`

Subconjunto do puzzle, sem `id/day/song/path`: `instruments`, `youtube`,
`youtubeStart`, `bpm`, `clue`, `wiki_*`, `spotifyId`, `stream`,
`appleMusicId`, `frontperson`, `sources`.

### `/v3/bonus/<path>.txt` (pós-jogo, fora do escopo)

Array de `{type, data:{answers:[...]}}`. Tipos vistos: `photo`,
`nationality`, `trivia1`, `year1`, `year2`, `album`, `tempo`, `instruments`,
`frontperson`. Cada resposta: `id`, `count` (votos), `label` (texto, URL de
imagem ou objeto `{de,en,es,fr,pt}`), `value` (opcional) e `correct: true`
na certa.

```json
{"type": "nationality", "data": {"answers": [
  {"id": 1786012206, "count": 3265, "label": "England", "correct": true},
  {"id": 1786012207, "count": 1383, "label": "USA"}]}}
```

## Songless

### `GET https://lessgames.com/songless` (página do dia)

Não há API de "daily": o HTML traz o flight-data do Next.js com as
categorias e a data (`ParseDailyPage`):

```
{\"categories\":[{\"categoryId\":0,\"fileUrl\":\"https://cdn.lessgames.com/songs/A101.aac\"},
                 {\"categoryId\":1,\"fileUrl\":\".../B137.aac\"},
                 {\"categoryId\":2,\"fileUrl\":\".../C040.aac\"}],\"date\":\"2026-09-27\"}
```

Categorias: 0 = All, 1 = Rock, 2 = Hip Hop. O `fileUrl` muda todo dia.

### Áudio (`fileUrl`)

AAC-LC em ADTS, 48 kHz, ~29,5 s (708 KB no dia observado). Os limites de
cada rodada são tempos dentro do mesmo arquivo.

### `GET /songless/search?query=marvin`

```json
[{"id": "SEMMQgZhm--14_IC7XGtZ", "artist": "Marvin Gaye", "track": "Ain't No Mountain High Enough"},
 {"id": "Z-tYFxL9NVdKHg2yye5DB", "artist": "Marvin Gaye", "track": "Sexual Healing"}]
```

### `POST /songless/guess`

Corpo: `challengeDate`, `categoryId`, `roundIndex`, `songId` (`null` = skip).
O servidor julga; a resposta só traz `answer` quando o jogo termina.

| Situação | Resposta |
| --- | --- |
| acerto | `{"state":"correct","answer":{"artist":"Marvin Gaye","track":"Ain't No Mountain High Enough","sc":"2087362767"}}` |
| artista certo | `{"state":"present"}` |
| errado | `{"state":"incorrect"}` |
| skip | `{"state":"skipped"}` |
| skip na última rodada | `{"state":"skipped","answer":{"artist":"Creedence Clearwater Revival","track":"Fortunate Son","sc":"253572245"}}` |
| data não permitida (HTTP 400) | `{"code":"DATE_NOT_ALLOWED"}` |

Outros códigos de erro: `SONG_NOT_FOUND`.

### `GET /songless/stats?challengeDate=2026-09-27&categoryId=0`

```json
{"daily": {"distribution": [{"point": 1, "weight": 0.1692}, {"point": 2, "weight": 0.3746},
                            {"point": 3, "weight": 0.7408}, {"point": 4, "weight": 0.2954},
                            {"point": 5, "weight": 0.2862}, {"point": 6, "weight": 1}]},
 "allTime": {"distribution": [], "winStreak": null}}
```

`weight` é relativo (o maior vale 1), não uma porcentagem.

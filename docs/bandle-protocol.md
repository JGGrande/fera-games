# Protocolo do Bandle (Fase 0)

Status: **parcial** — regras, numeração e termos mapeados por fontes públicas; endpoints e áudio aguardam a captura do HAR (ver "Como completar").

## 1. Identificação do puzzle diário

| Item | Valor | Confiança |
| --- | --- | --- |
| Numeração | `N = dias desde 2022-08-17` (#1 = 2022-08-18) | Confirmado por posts oficiais `#Bandle #N` |
| Virada do dia | Meia-noite no horário local do jogador | Provável |
| Implementação | `internal/games/bandle.PuzzleNumber` (testada) | — |
| Endpoint do puzzle | _preencher com o HAR_ | Desconhecido |
| Campos do payload | _preencher com a estrutura gerada pelo harscan_ | Desconhecido |

## 2. Áudio

| Item | Valor | Confiança |
| --- | --- | --- |
| Origem | Recriações feitas à mão (Logic Pro), não stems originais | Confirmado |
| Estágios | 6 tentativas; 1–5 somam instrumentos (bateria → baixo → harmonia → voz, ordem varia); 6ª traz dica textual | Provável |
| Stem por instrumento ou mix por estágio | _preencher_ | Desconhecido |
| Formato / host / padrão de URL | _preencher com `probe -url`_ | Desconhecido |
| Suporta `Range` (seek) | _preencher_ | Desconhecido |

## 3. Catálogo do autocomplete

| Item | Valor | Confiança |
| --- | --- | --- |
| Tamanho | ~1.900+ músicas (fonte de terceiros) | Fraca |
| Entrega (JSON único vs busca) | _preencher_ | Desconhecido |

## 4. Validação da resposta

- Resposta no payload do puzzle? _preencher_
- Endpoint de validação/estatística ao chutar? _preencher_

## 5. Backend, auth e limites

| Item | Valor | Confiança |
| --- | --- | --- |
| Conta/login | Não há | Confirmado (política de privacidade) |
| Terceiros | Google Analytics, anúncios NitroPay | Confirmado |
| robots.txt | Sem restrições | Confirmado |
| Tipo de backend (API própria, Firebase, CDN) | _preencher_ | Desconhecido |
| Headers obrigatórios, CORS, rate limit | _preencher_ | Desconhecido |

## 6. Regras e compartilhamento

- 6 tentativas; pular gasta uma tentativa e libera o próximo instrumento.
- Dicas visíveis desde o início: ano, visualizações, dificuldade.
- Texto de compartilhamento: _copiar um exemplo real ao terminar a partida_.

## 7. Termos de uso

- Termos de 10/10/2025 (SAS BANDLE, Paris) não têm cláusula sobre scraping, acesso automatizado ou clientes de terceiros.
- O áudio segue protegido por direito autoral. Decisão do projeto: uso pessoal, repo privado, sem redistribuir áudio, `User-Agent` identificável, 1 download por dia com cache.

## Como completar (≈15 min, na sua máquina)

1. Chrome → abrir `https://bandle.app/daily` → DevTools (F12) → **Network** → marcar **Preserve log** e **Disable cache**.
2. Recarregar, jogar a partida inteira (incluindo 1 pulo) e abrir o compartilhamento.
3. Botão direito na lista → **Save all as HAR with content** → salvar como `bandle.har` na raiz do repo (não commitar; já está no `.gitignore`).
4. `go run ./cmd/harscan -har bandle.har` → gera `docs/bandle-har-report.md` e `testdata/bandle/*.json`.
5. Para cada URL de mídia do relatório: `go run ./cmd/probe -url <URL>`.
6. Tocar a primeira faixa: `go get github.com/gopxl/beep/v2@latest && go run -tags audio ./cmd/probe -url <URL da faixa 1> -play`.
7. Na aba **Sources**, buscar no `main*.js`: `fetch(`, `firebase`, `.mp3`, `answer`, `2022-08`.
8. Preencher as células _preencher_ acima e confirmar se o número no payload bate com `go run ./cmd/probe`.

## Fontes

- https://bandle.app/terms.html
- https://bandle.app/privacy.html
- https://online.berklee.edu/takenote/bandle-creator-johann-levy-on-the-music-quiz-100k-daily-players-love/
- https://www.nbcnews.com/tech/tech-news/bandle-song-guessing-game-builds-buzz-online-rcna149012
- https://www.ggrecon.com/word-games/bandle-answer-today-hints/

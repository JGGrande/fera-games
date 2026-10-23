# Protocolo do Conexo (Fase 0)

Status: **completo** em 2026-10-22. Mapeado lendo o bundle da SPA (`/assets/index-wuQPZXQF.js`, 1 186 714 bytes, Vite + React), reproduzindo as chamadas com Node/Go contra a produção e jogando o puzzle **#1117 (pt, 2026-10-22)** de ponta a ponta num Chrome headless (o Chrome do sistema dirigido por Playwright), comparando estado, `localStorage` e texto de compartilhamento com o que o código prevê.

Mesmo método do [bandle-protocol.md](bandle-protocol.md): primeiro o bundle, depois a reprodução fora do navegador. Diferença central para os outros jogos: **o backend é o Firestore e exige um token do Firebase App Check (reCAPTCHA Enterprise)**. Fora isso, o Conexo é o jogo mais simples dos três: um único documento JSON por dia, nenhuma mídia e validação 100% local.

## 0. Fluxo completo (o que o port precisa implementar)

```
1. [navegador] reCAPTCHA Enterprise (site key 6LfUk0IrAAAAAN1bu0aEGQXoZ5pu-rlct_4WT1WL)
   → grecaptcha.enterprise.execute()                                → token reCAPTCHA
2. [navegador] POST https://content-firebaseappcheck.googleapis.com/v1/projects/daydashgames/
        apps/1:689008611670:web:45c4d4dbce26e56deb46f7:exchangeRecaptchaEnterpriseToken?key=<apiKey>
   {"recaptcha_enterprise_token":"<token reCAPTCHA>"}             → {"ttl":"604800s","token":"<JWT App Check>"}
3. GET https://firestore.googleapis.com/v1/projects/daydashgames/databases/(default)/documents/conexo-daily-<lang>/<id>
   X-Firebase-AppCheck: <JWT App Check>                              → documento tipado do Firestore (~5 KB)
4. Converter o JSON tipado → {id, groups[4]{number, theme, words[4]}, startingBoard[16], credits, corrections?, version?, images?}
5. Jogar localmente: seleção de 4 → julgamento local → dicas → vitória → texto de compartilhamento.
```

Os passos 1 e 2 **só funcionam num navegador real** (é para isso que o App Check existe). O passo 3 funciona de qualquer cliente HTTP (testado com Node `fetch` e Go `net/http`) enquanto o JWT for válido, e ele **vale 7 dias**. Ver §2 para as opções.

O site **não manda nada ao servidor durante a partida** (usuário deslogado): o único tráfego do Firebase numa partida inteira foi o exchange do App Check e a leitura do documento (verificado na partida simulada, §4.6).

## 1. Identificação do puzzle diário

| Item | Valor | Confiança |
| --- | --- | --- |
| Numeração | `id = dias civis entre a data local de hoje e startDates[lang]`. `startDates = {pt: 2023-10-01, en: 2024-02-01, es: 2024-06-01}` (config `He` no bundle). `id 0` = data de início | **Confirmado** |
| Hoje | 2026-10-22 → pt **1117**, en **994**, es **873**. O navegador pediu `conexo-daily-pt/1117` (`addTarget` do Listen) e o documento tem `id: 1117` | **Confirmado** |
| Virada do dia | Meia-noite **local** (`dayjs()` sem fuso fixo; `startOf('day')` e `diff(…,'day')`). Em Go: diferença entre `time.Date(y,m,d,0,0,0,0,time.UTC)` das duas datas civis, para não sofrer com horário de verão | Confirmado no código |
| Data exibida | `title = YYYY-MM-DD` do dia; na tela `DD/MM/YYYY` (pt/es) ou `MM/DD/YYYY` (en) | Confirmado |
| Publicação | Os puzzles são criados em lote com semanas de antecedência (`createTime` de #1117 = 2026-09-01). Em 2026-10-22 existiam documentos até **2026-11-30** nos três idiomas (pt 0–1156, en 0–1033, es 0–912), sem buracos | **Confirmado** (listagem completa) |
| Dias futuros | Bloqueio **só no cliente**: `MZ` lança `"<data> is still to come"` se a data > hoje (exceto rota de prévia). As regras do Firestore **deixam ler** documentos futuros (testado só o status: 1118 e 1120 → 200; 1200 → 404). O port não deve mostrar dias futuros | Confirmado |
| Arquivo | `/pt/previous` (calendário desde a data de início) e `/pt/previous/YYYY-MM-DD` → `id = B_(data)`. Sem paywall: o "Conexo Pass" (`/pass`, Paddle) só remove anúncios | Confirmado no código |
| Idiomas | Coleções separadas: `conexo-daily-pt`, `conexo-daily-en`, `conexo-daily-es`; mesma estrutura | Confirmado |

Rotas da SPA: `/`, `/daily`, `/previous`, `/previous/:date`, `/pass` e as mesmas com prefixo `/:language`, mais `/:language/preview/:date` (prévia de editores). `/pt/daily` aberta direto, sem `location.state.fromApp`, redireciona para `/pt/` (é preciso clicar em "Jogar"), o que não afeta o port.

## 2. App Check: o bloqueio principal

### 2.1 O que foi verificado

| Item | Valor | Confiança |
| --- | --- | --- |
| Provider web | `ReCaptchaEnterpriseProvider` (site key `6LfUk0IrAAAAAN1bu0aEGQXoZ5pu-rlct_4WT1WL`), `isTokenAutoRefreshEnabled: true`. Apps nativos (Capacitor) usam o plugin `FirebaseAppCheck` (provavelmente Play Integrity/App Attest) | Confirmado (web) |
| Exchange | `POST …:exchangeRecaptchaEnterpriseToken?key=<apiKey>` com `{"recaptcha_enterprise_token": …}` e `Referer: https://conexo.ws/` → `{"ttl":"604800s","token":"<JWT>"}` | **Confirmado** (capturado no navegador) |
| JWT | RS256, claims `sub = 1:689008611670:web:45c4d4dbce26e56deb46f7`, `aud = [projects/689008611670, projects/daydashgames]`, `provider = recaptcha_enterprise`, `iss = https://firebaseappcheck.googleapis.com/689008611670`, `exp - iat = 168 h`, `jti`. Não é *limited-use*: o mesmo token serviu para dezenas de requisições | **Confirmado** |
| Vínculo | Funcionou fora do Chrome, com outro `User-Agent` (Node e Go), sem `Referer`, sem `Origin` e sem `?key=`. Outro IP: não testado | Confirmado / IP não testado |
| Firestore sem token, com token lixo ou adulterado | `403 {"status":"PERMISSION_DENIED","message":"Missing or insufficient permissions."}`: as três situações são indistinguíveis | **Confirmado** |
| Exchange com token reCAPTCHA inválido, ou `exchangeDebugToken` com UUID qualquer | `403 {"status":"PERMISSION_DENIED","message":"App attestation failed."}` | Confirmado |
| Firebase Auth | Também protegido: `accounts:signUp` sem App Check → `401 "Firebase App Check token is invalid."`. O jogo **não precisa** de Auth (o login é opcional, só para salvar na nuvem) | Confirmado |
| Onde o navegador guarda | O SDK grava em IndexedDB `firebase-app-check-database` / store `firebase-app-check-store` (keyPath `compositeKey`) o registro `{compositeKey: "1:689008611670:web:45c4d4dbce26e56deb46f7-[DEFAULT]", value: {token, expireTimeMillis, issuedAtTimeMillis}}`. No Chrome headless do estudo o store ficou vazio (token só em memória) | Código confirmado (2026-10-23) |
| Reaproveitamento | Ao ativar o App Check, o SDK lê o token do IndexedDB (`cachedTokenPromise`) e, se ainda vale, **não chama o exchange**. A renovação automática fica para `issuedAt + 50% da validade + 5 min` (≈3,5 dias). Por isso, num navegador que já abriu o site, a request `exchangeRecaptchaEnterpriseToken` não aparece na aba Network | **Confirmado** no código (2026-10-23) e relatado pelo usuário |

### 2.2 Como o terminal pode obter o token

Não existe caminho sem navegador: o token reCAPTCHA só sai do JavaScript do Google rodando numa página de `conexo.ws`. Gerar esse token fora do navegador é exatamente o que o App Check foi feito para impedir e está fora de escopo. O debug token só pode ser criado pelo dono do projeto, e a atestação nativa (Play Integrity/App Attest) não é viável.

| Opção | Como | Prós | Contras |
| --- | --- | --- | --- |
| **A. Colar o token** (**adotada**) | O usuário abre `conexo.ws` no próprio navegador, DevTools → Application → IndexedDB → `firebase-app-check-database` → `firebase-app-check-store` → registro `…-[DEFAULT]` → `value.token` (§8). O fera recebe pela tela do Conexo ou pela env `FERA_CONEXO_APPCHECK` e grava em `<data-dir>/conexo-appcheck.jwt` | Binário único, sem navegador embutido, sessão do próprio usuário | Repetir quando vencer (até 7 dias, menos se o token guardado já estiver na metade); atrito na primeira vez |
| B. Helper com o Chrome do sistema | O fera abre o Chrome do sistema via CDP num perfil vazio, carrega `conexo.ws/pt/` e captura a resposta do exchange | Automático | **Descartada.** Testada em 2026-10-23 com `chromedp` v0.21 e Chrome 155 headless, sem camuflagem: o exchange acontece em ~2 s, mas responde `403 App attestation failed.` nas 3 tentativas (o reCAPTCHA deu nota de robô). Fazer passar exigiria esconder a automação, o que está fora de escopo. Também feria a restrição "nunca embutir navegador" |
| C. Cache adiantado (complementa A ou B) | Com um token válido, baixar hoje e os próximos dias já publicados (até ~5 semanas) e guardar em cache; só **exibir** cada puzzle no dia dele | Um token por mês em vez de por semana | Lê o que o site esconde até o dia (o bloqueio do site é só no cliente). Decisão de projeto/termos (§7) |

Comportamento esperado do port: decodificar o `exp` do JWT (base64url do payload; não precisa validar a assinatura) para avisar quando faltar menos de 1 dia; tratar `403 PERMISSION_DENIED` do Firestore como "token expirado ou inválido, cole um novo", porque o servidor não diferencia token ruim de falta de permissão.

## 3. Payload do puzzle

### 3.1 Requisição

```
GET https://firestore.googleapis.com/v1/projects/daydashgames/databases/(default)/documents/conexo-daily-pt/1117
X-Firebase-AppCheck: eyJraWQiOiJBNzFLSHciLCJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.…
```

- `?key=AIzaSyBRNP7U8vZEuCTumdim_ZLencN0u_I93no` é opcional (funciona sem).
- Resposta `200`, `Content-Type: application/json; charset=UTF-8`, **sem `Cache-Control` e sem ETag**. O SDK web não cacheia entre visitas: relê a cada carga.
- `404 {"status":"NOT_FOUND","message":"Document \"…/conexo-daily-pt/9999\" not found."}` quando não existe.
- O SDK web não usa REST: usa o WebChannel `POST/GET https://firestore.googleapis.com/google.firestore.v1.Firestore/Listen/channel?VER=8&database=projects%2Fdaydashgames%2Fdatabases%2F(default)&…` com corpo `req0___data__={"database":"projects/daydashgames/databases/(default)","addTarget":{"documents":{"documents":["projects/daydashgames/databases/(default)/documents/conexo-daily-pt/1117"]},"targetId":2}}` e o token no pseudo-header `x-firebase-appcheck`. O REST `GET` é equivalente e muito mais simples, então o port usa REST.
- Também liberados pelas regras (com App Check): listar a coleção (`GET …/conexo-daily-pt?pageSize=300&pageToken=…`, opcionalmente com `mask.fieldPaths=id`) e `documents:runQuery`. `user-data/…` e `subscriptions/…` → 403 sem Auth.

### 3.2 Resposta (formato tipado do Firestore)

```json
{
  "name": "projects/daydashgames/databases/(default)/documents/conexo-daily-pt/1117",
  "fields": {
    "id": {"integerValue": "1117"},
    "groups": {"arrayValue": {"values": [
      {"mapValue": {"fields": {
        "number": {"integerValue": "1"},
        "theme": {"stringValue": "Equipamentos de pesca"},
        "words": {"arrayValue": {"values": [{"stringValue": "molinete"}, {"stringValue": "vara"}, {"stringValue": "linha"}, {"stringValue": "anzol"}]}}
      }}}
    ]}},
    "startingBoard": {"arrayValue": {"values": [{"stringValue": "manilha"}, {"stringValue": "espaldar"}]}},
    "credits": {"mapValue": {"fields": {"author": {"stringValue": "Anna Duzzi"}, "reviewer": {"stringValue": "Elysa Marques"}}}}
  },
  "createTime": "2026-09-01T17:42:42.500266Z",
  "updateTime": "2026-09-01T17:42:42.500266Z"
}
```

Atenção: `integerValue` vem como **string**. Mapas usam `mapValue.fields`, listas `arrayValue.values` (ausente quando vazia), nulos `nullValue`.

### 3.3 Documento já convertido (#1117, pt, 2026-10-22)

```json
{
  "id": 1117,
  "groups": [
    {"number": 1, "theme": "Equipamentos de pesca", "words": ["molinete", "vara", "linha", "anzol"]},
    {"number": 2, "theme": "Moldável", "words": ["maleável", "elástico", "mole", "flexível"]},
    {"number": 3, "theme": "Componentes de uma armadura medieval", "words": ["manopla", "espaldar", "braçal", "greva"]},
    {"number": 4, "theme": "Truco", "words": ["manilha", "zap", "vira", "vaza"]}
  ],
  "startingBoard": ["manilha", "espaldar", "vara", "braçal", "flexível", "linha", "maleável", "vira",
                    "greva", "elástico", "manopla", "mole", "molinete", "vaza", "anzol", "zap"],
  "credits": {"author": "Anna Duzzi", "reviewer": "Elysa Marques"}
}
```

### 3.4 Campos (varredura dos 1 157 documentos pt; en e es conferidos por amostra e por máscara)

| Campo | Tipo | Presença | Uso |
| --- | --- | --- | --- |
| `id` | int | sempre; igual ao ID do documento | número do puzzle |
| `groups` | 4 × `{number, theme, words[4]}` | sempre; `number` 1..4 **na ordem** | resposta. `number` define a cor (1 laranja, 2 verde, 3 azul, 4 roxo) e a ordem das dicas |
| `startingBoard` | 16 strings | sempre; permutação exata da união dos grupos, sem repetição, **nunca** na ordem dos grupos | ordem inicial do tabuleiro (tela, linha a linha, 4 × 4) |
| `credits` | `{author, reviewer}` | sempre | rodapé "Redação: …" / "Revisão: …" |
| `corrections` | mapa `original → exibido` | 37 docs pt (36 não vazios) | só **exibição** (`X_`): `corrections[w] \|\| w`, aplicado a palavras (26 casos) e a temas (9). Ex.: `{"sacodindo":"sacudindo"}`, `{"carrapato":"coró"}`. 3 chaves órfãs (correções antigas). O ID interno continua sendo o original |
| `version` | número (visto `2`) | 4 docs pt, 3 en | se mudar, o progresso salvo é descartado (§5). Ausente → tratado como `null` |
| `images` | mapa `palavra → caminho relativo` | 1 doc pt (#913), 1 doc en | tabuleiro de imagens (`/images/games/pt/913/uai.png` etc., em `conexo.ws`); `alt` = palavra corrigida. No terminal: mostrar o texto |

Formato das palavras: sempre minúsculas no dado (a tela põe em maiúsculas por CSS e diminui a fonte até caber); 456 palavras com espaço (`"chave de roda"`); a maior tem 25 caracteres (`"encaminhe para 10 pessoas"`), o maior tema 62. Há dígitos, `' + - . = ² ¾ ↑` e acentos variados (`ñ ö è`). Temas podem ter lacunas `___` (`"Parada ___"`). 890 docs têm `updateTime` depois do `createTime` (852 num lote de 2026-01-14, o resto espalhado até 2026-07), sem mudar a estrutura: puzzles antigos podem ser corrigidos depois de publicados.

## 4. Regras e validação (100% local)

A resposta vem no payload, como no Bandle. **Não há endpoint de chute.** As funções abaixo foram lidas no bundle e conferidas na partida simulada (§4.6).

### 4.1 Estado da partida

```
GameData = {status: "selecting"|"won", attempts: Attempt[], groups: Group[] (achados, na ordem em que foram achados),
            items: string[] (palavras ainda soltas, na ordem do tabuleiro), selected: string[], hints: string[], version}
Attempt  = {selected: [4 palavras na ordem do clique], group: number|null}   // chute
         | {hints: string[]}                                                  // uso de dica
inicial  = {status: "selecting", groups: [], items: spec.startingBoard, selected: [], attempts: [], hints: [], version: spec.version}
```

### 4.2 Seleção e chute (`ov`, `sv`, `lv`)

- Clicar numa palavra solta a seleciona; clicar de novo desseleciona. Máximo de 4.
- **A 4ª seleção já envia o chute** (não há botão "enviar").
- Julgamento: `grupo = spec.groups.find(g => selected.every(w => g.words.includes(w)))`.
  - Sem grupo → `attempts += {selected, group: null}`, `selected = []`, tabuleiro igual. **Não existe aviso de "faltou uma"**.
  - Com grupo → `attempts += {selected, group: n}`, `items -= selected`, `groups += grupo`, `selected = []`; se `hints[0]` pertence ao grupo, `hints = []`; `status = "won"` quando `groups.length == 4`.
- Tentativas ilimitadas, sem derrota. Chute repetido conta de novo (verificado).
- O 4º grupo também precisa ser selecionado e conta como tentativa (verificado: o último acerto entrou na contagem).
- Reordenar (arrastar e soltar) **troca** duas posições de `items` (`cv`); é salvo, mas não é tentativa. Opcional no terminal.

### 4.3 Dicas (`ev`, `uv`)

```
dica(spec, achados, hints):
  se len(achados) >= 3 ou len(hints) == 3: nenhuma (botão desabilitado)
  se len(hints) == 2: as 3 primeiras palavras (na ordem do spec) do grupo de hints[0]
  senão:              as 2 primeiras palavras do grupo não achado de menor number
```

- Cada uso grava `attempts += {hints: novas}` e `hints = novas` (as palavras ganham um canto com a cor do grupo: classe `hint hint<n>`).
- Sequência: 1ª dica → 2 palavras do grupo; 2ª → 3 palavras do mesmo grupo; 3ª → bloqueada até achar esse grupo. Achar o grupo da dica zera `hints`, e a próxima dica vai para o próximo grupo de menor `number`. Achar **outro** grupo mantém a dica (verificado).
- Dicas não contam em "Tentativas"; contam em "Dicas" e aparecem como 💡 no resultado.

### 4.4 Animações (só para referência do ritmo da TUI)

Erro: 500 ms "checando" → 1 000 ms com borda vermelha e tremida → volta. Acerto: 500 ms → 1 000 ms com as 4 na cor do grupo → 1 000 ms com elas puxadas para o início → a faixa do grupo aparece com `tema` e `palavras` (`", "`). Vitória: mais 2 000 ms de "celebração". Durante a animação os cliques e a dica ficam bloqueados.

### 4.5 Fim de jogo e resultado

- Barra de status: data, `Tentativas: <chutes sem dica>` e `Dicas: <n>` (esta só quando > 0).
- Mensagem final: `Parabéns!` / `Você acertou em <N> tentativas[ e <H> dica|dicas].` + grade de emojis + botão "Compartilhe".
- Emojis por tentativa, na ordem: grupo 1 🟧, 2 🟩, 3 🟦, 4 🟪, dica 💡, erro ❌; quebra de linha a cada **10**.
- Texto de compartilhamento (`ZZ`), com i18next (`hints_one` no singular):

| lang | Modelo |
| --- | --- |
| pt | `Joguei conexo.ws <DD/MM/YYYY> e consegui em <N> tentativas[ e <H> dica\|dicas].\n\n<emojis>` |
| en | `I played conexo.ws <MM/DD/YYYY> and got it in <N> guesses[ and <H> hint\|hints].\n\n<emojis>` |
| es | `Jugué conexo.ws <DD/MM/YYYY> y lo conseguí en <N> intentos[ y <H> pista\|pistas].\n\n<emojis>` |

No mobile usa `navigator.share`; no desktop copia para a área de transferência.

### 4.6 Partida simulada (verificação de ponta a ponta)

Sequência jogada no Chrome headless em #1117 (pt): erro, o mesmo erro de novo, dica, grupo 1, dica, dica, grupo 3, **recarregar a página**, grupo 2, grupo 4.

| Passo | Resultado observado |
| --- | --- |
| `molinete vara linha maleável` (×2) | ❌ duas vezes, `Tentativas: 2`, `attempts[i].group = null` |
| dica | `molinete`, `vara` marcadas `hint1`; `Dicas: 1` |
| grupo 1 | 🟧, `hints` zerado, `items` com 12 palavras na ordem original |
| dica, dica | `maleável elástico` → `maleável elástico mole` (`hint2`) |
| grupo 3 | 🟦, dica do grupo 2 mantida |
| recarregar | grupos, itens e dicas restaurados do `localStorage` |
| grupo 2, grupo 4 | 🟩 🟪, `status: "won"` |
| Fim | `Parabéns!\n\nVocê acertou em 6 tentativas e 3 dicas.\n\n❌❌💡🟧💡💡🟦🟩🟪` |
| Compartilhar | `Joguei conexo.ws 22/10/2026 e consegui em 6 tentativas e 3 dicas.\n\n❌❌💡🟧💡💡🟦🟩🟪` |
| Rede (Firebase) | exchange do App Check + 2 leituras `Listen/channel` (a 2ª é o reload). Nenhuma escrita |

## 5. Persistência no site (modelo para o `Snapshot` do port)

- `localStorage["conexo-games-<lang>"]` = `{"<id>": {status, attempts, groups, items, hints, version}}` (sem `selected`, então a seleção se perde ao recarregar). Gravado a cada chute, dica e reordenação.
- Exemplo real (fim da partida acima, abreviado):
  `{"1117":{"status":"won","attempts":[{"selected":["molinete","vara","linha","maleável"],"group":null},…,{"hints":["molinete","vara"]},{"selected":["molinete","vara","linha","anzol"],"group":1},…],"groups":[…],"items":[],"hints":[],"version":null}}`
- Outras chaves: `conexo-language`, `conexo-tutorial-done-in` (o tutorial abre sozinho só em pt), `daydash-login-state-cache`, `daydash-subscription-cache`, `conexo-top-banner-dismissed`.
- **Restauração validada** (`$_`): o salvo é descartado (partida recomeça) se
  1. `version` difere da do spec;
  2. algum chute não tem 4 palavras do puzzle, ou o `group` gravado difere do que o julgamento dá agora (dicas não são revalidadas);
  3. os grupos recalculados a partir dos chutes diferem de `groups`;
  4. alguma palavra do puzzle não está em `groups ∪ items`.
  Depois `status = groups.length == 4 ? "won" : "selecting"`. É o mesmo princípio do `bandle.Session.Restore` (rejulgar, não confiar no arquivo).
- Calendário: `won` ou `selecting` (iniciado, com pelo menos um chute ou dica) por `id`.
- Logado (fora do escopo): `user-data/<uid>/conexo-daily-<lang>/game-data/saved/<id>` (partida) e `user-data/<uid>/conexo-daily-<lang>/overview` (`{<id>: status}`), `subscriptions/<uid>`. Exige Firebase Auth (Google, Apple ou e-mail) **e** App Check.

## 6. Backend, hospedagem e terceiros

| Item | Valor | Confiança |
| --- | --- | --- |
| Hospedagem | Firebase Hosting (CDN Fastly, `x-served-by: cache-gru-…`, `vary: x-fh-requested-host`). HTML `no-cache`; `/assets/*` `max-age=31536000, immutable` | Confirmado |
| Firebase | `apiKey AIzaSyBRNP7U8vZEuCTumdim_ZLencN0u_I93no`, `authDomain conexo.ws`, `projectId daydashgames`, `storageBucket daydashgames.firebasestorage.app`, `messagingSenderId 689008611670`, `appId 1:689008611670:web:45c4d4dbce26e56deb46f7` (ios `…:ios:6c3ce8b14b6a9b2eeb46f7`, android `…:android:65425cb38e939e5deb46f7`). SDK Firestore 4.7.3 (`fire/10.14.1`) | Confirmado |
| Cloud Functions | Uma callable: `us-central1` / `deleteAccount` (exclusão de conta). Nada no jogo | Confirmado |
| Projeto | `daydashgames`, da Daydash (Contexto, Letroso, Expresso, Ordeno). O código traz config por jogo (`gameName: conexo`, `startDates`) | Confirmado |
| Terceiros | Google Analytics (`G-EX17W5NMNE`), anúncios Playwire/Ramp, Paddle (assinatura), Confiant. Todos dispensáveis (bloqueados na partida simulada sem efeito no jogo) | Confirmado |
| Rate limit | Nenhum observado (~25 leituras avulsas + 13 páginas de listagem). Cota do Firestore é do dono do projeto: o port faz **1 leitura por dia** e cacheia | Provável |
| robots.txt | `Disallow:` vazio | Confirmado |

## 7. Termos de uso

- Termos de 26/07/2026 (DAYDASH GAMES LTDA, CNPJ 47.824.782/0001-47): sem cláusula específica sobre scraping ou clientes de terceiros, mas "usar os jogos conforme sua finalidade e não interferir, interromper **ou tentar acessar sem autorização** os serviços". O conteúdo (puzzles) é propriedade da Daydash.
- O App Check é um controle de acesso. Reaproveitar o token da sessão do próprio usuário (opção A) é o caminho menos agressivo; gerar tokens fora do navegador ou contornar o reCAPTCHA não é aceitável. Ler puzzles futuros (opção C) contraria a intenção do site, mesmo que as regras deixem.
- Decisão do projeto (mesma do Bandle e do Songless): uso pessoal, repo privado, sem redistribuir conteúdo, `User-Agent` identificável, 1 leitura por dia com cache.

## 8. Como reproduzir

O spike usou scripts descartáveis (fora do repo): Playwright (`playwright-core`) dirigindo `/usr/bin/google-chrome` em modo headless para capturar o exchange e jogar a partida, e chamadas REST em Node e Go. O núcleo do port, já testado em Go puro:

```go
start := time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC) // startDates["pt"]
now := time.Now()                                       // fuso local
today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
id := int(today.Sub(start).Hours() / 24)                // 2026-10-22 → 1117

url := fmt.Sprintf("https://firestore.googleapis.com/v1/projects/daydashgames/databases/(default)/documents/conexo-daily-pt/%d", id)
req, _ := http.NewRequest("GET", url, nil)
req.Header.Set("User-Agent", "fera-games/dev (+terminal)")
req.Header.Set("X-Firebase-AppCheck", appCheckJWT) // §2.2
// 200 → JSON tipado (§3.2); 403 → token inválido/expirado; 404 → sem puzzle
```

Capturar um token manualmente: abrir `https://conexo.ws/pt/` e esperar carregar → DevTools → Application → IndexedDB → `firebase-app-check-database` → `firebase-app-check-store` → registro `1:689008611670:web:45c4d4dbce26e56deb46f7-[DEFAULT]` → `value` → `token` (começa com `eyJ`). O fera aceita o registro inteiro colado, porque extrai o JWT do texto. Validade em `exp` (segundos Unix) no payload do JWT, ou em `value.expireTimeMillis`. A request `exchangeRecaptcha` na aba Network só aparece quando o navegador não tem token guardado, como numa janela anônima com o DevTools aberto antes de carregar a página (§2.1).

## 9. Mapa para o port (Fera Games)

Mesma arquitetura (`TUI → Engine → Provider + Store`), sem áudio e sem catálogo.

```go
// internal/games/conexo
type Provider interface {
    Puzzle(ctx context.Context, lang string, id int) (Puzzle, error) // GET Firestore REST + App Check; cache em disco
}
type Puzzle struct {
    ID            int
    Groups        [4]Group          // Number 1..4, Theme, Words[4]
    StartingBoard [16]string
    Credits       struct{ Author, Reviewer string }
    Corrections   map[string]string // só exibição
    Version       *int
}
```

- **Engine**: `Session` com `Select(word)`, `Guess([4]string)`, `Hint()`, `Swap(i, j)`, `ShareText()`, `Snapshot()`/`Restore()` (validação do §5). Funções puras como `bandle.Judge`: `judge`, `nextHint`, `emojiGrid`, `shareText`.
- **`game.Session` não encaixa direto**: `Guess(ctx, answer string)` e `Skip` pensam em um chute textual por rodada, e `State.Steps` supõe limite. Ou se estende a interface, ou se faz um adaptador (`Guess` recebendo as 4 palavras separadas por `|`, `Skip` = dica, `Steps = 0` = ilimitado), ou a TUI fala com a sessão concreta, como `songlessui` já faz.
- **Erros**: `ErrNoPuzzle` (404 ou data futura), `ErrAppCheck` (403: pedir token novo), rede/timeout (o site desiste em 15 s). Usar o `friendly` do `bandleui` com `r` para tentar de novo.
- **Cache**: o documento do dia muda raramente (`version`), então basta cachear por `(lang, id)` e reler só se o cache for de outro dia. O payload traz a resposta, então valem os mesmos cuidados do Bandle com spoiler no cache.
- **Store**: `store.Match{Day, Number: id, Status: playing|won, Tries: chutes sem dica, Data: Snapshot}`; não existe `lost`. Para a sequência, uma vitória por dia (o site não tem sequência própria; o port pode reaproveitar `store.Stats`).
- **TUI** (sugestão): grade 4 × 4 de palavras em maiúsculas (colunas de ~25 caracteres, quebrando em duas linhas se precisar); setas movem o cursor, `espaço`/`enter` alterna a seleção, a 4ª envia; `d` dica; `c` copia o resultado (OSC52); grupos achados viram faixas no topo com `TEMA` e `palavras`. Cores do tema escuro do site (padrão): grupo 1 `#c2410c`, 2 `#047857`, 3 `#0e7490`, 4 `#6d28d9`, borda de erro `#b91c1c`; tema claro: `#f6c9ab`, `#b8e5c4`, `#b4dcec`, `#d6c9f2`. A palavra com dica ganha um canto na cor do grupo dela.
- **Casos especiais do site** (podem ser ignorados): pt #913 e en #790 com tabuleiro de imagens e ícone "clippy" na dica ("Redação e Arte"); 2025-04-01 (pt #548) mostra todos os grupos em roxo até vencer.

## Fontes

- Bundle: `https://conexo.ws/assets/index-wuQPZXQF.js` e `https://conexo.ws/assets/index-BZCr7nM6.css` (servidos em 2026-10-22)
- Tráfego capturado no Chrome headless (exchange do App Check, `Listen/channel`), respostas REST do Firestore e listagem completa de `conexo-daily-pt` (2026-10-22)
- https://conexo.ws/terms.html (vigência 26/07/2026)
- https://conexo.ws/robots.txt

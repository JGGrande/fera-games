# Voce é o Fera Games?

Cliente de terminal para descobrir o real ser humano games.

## Rodar

```
make run # abre o menu;
make deps # instala as dependências necessárias;
```

No menu, a tecla `a` abre o Admin do Fera Músicas: cadastro, edição e remoção de músicas, com
prévia do trecho.

## Dependências

Instale como preferir (gerenciador de pacotes da sua distro, Homebrew, winget, pip, binário oficial…).

Para compilar e rodar:

- **Go 1.26** ou mais novo.
- **make**, se quiser usar os atalhos acima. Sem ele, `go run ./cmd/fera` faz o mesmo que `make run`.
- **Linux, para o áudio:** um compilador C (gcc ou clang) e os arquivos de desenvolvimento do ALSA
  (o pacote que fornece `alsa` para o `pkg-config`). O áudio no Linux é compilado com cgo.
  macOS e Windows não precisam de nada extra.

Para usar:

- **Terminal de pelo menos 80x24.**
- **Linux, para copiar o resultado (`c`):** `xclip`, `xsel` ou `wl-clipboard` (Wayland). Sem eles,
  a cópia só funciona em terminais com suporte a OSC 52. macOS e Windows já têm o necessário.
- **Fera Músicas:** `yt-dlp` e `ffmpeg` no `PATH`. O áudio é baixado do YouTube na hora da
  partida. Sem os dois, o jogo abre e o cadastro funciona, mas não toca nada. Instalou com o
  programa aberto? Feche e abra de novo. Mantenha o `yt-dlp` atualizado: versões antigas param
  de funcionar quando o YouTube muda.
- **Fera Músicas, catálogo compartilhado (opcional):** um projeto no Firebase com o Cloud
  Firestore criado e uma conta de serviço com acesso a ele. Sem isso, o catálogo fica só em
  memória e some ao fechar. Veja a configuração abaixo.

## Configuração (.env)

```
cp .env.example .env
```

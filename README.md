# Voce é o Fera Games?

Cliente de terminal para descobrir o real ser humano games.

## Rodar

```
make run                     # abre o menu; enter no Bandle joga o puzzle de hoje; q volta/sai
make run ARGS=--mute         # sem áudio (a barra de progresso anda mesmo assim)
make build                   # compila ./fera
make check                   # fmt + vet + test + lint (instala o golangci-lint v2 se faltar)
make snapshot VERSION=v0.1.0 # binários linux/darwin/windows em dist/ (instala o goreleaser se faltar)
make                         # lista todos os alvos
```
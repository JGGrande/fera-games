# Fera Games

Cliente de terminal (Go 1.26 + Bubble Tea) para jogos diários. Uso pessoal.

## Fase 0 — spike do protocolo do Bandle

```
go test ./...                         # numeração do puzzle, parser de HAR, detecção de formato
go run ./cmd/probe                    # número do Bandle de hoje
go run ./cmd/harscan -har bandle.har  # relatório de endpoints + payloads em testdata/
go run ./cmd/probe -url <URL>         # status, headers e formato de uma URL
go run -tags audio ./cmd/probe -url <faixa> -play   # tocar (requer beep v2)
```

Passo a passo da captura e o que falta descobrir: [docs/bandle-protocol.md](docs/bandle-protocol.md).

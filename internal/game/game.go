// Package game define o contrato comum entre a TUI e as engines dos jogos.
// A TUI só conhece estas interfaces; HTTP, áudio e persistência ficam atrás delas.
package game

import (
	"context"
	"time"
)

// Game é um jogo diário registrado no hub.
type Game interface {
	ID() string   // identificador estável, usado em arquivos e flags ("bandle")
	Name() string // nome mostrado no menu ("Bandle")
	NewSession(ctx context.Context, day time.Time) (Session, error)
}

// Session é uma partida do dia em andamento (ou já terminada).
type Session interface {
	State() State
	Guess(ctx context.Context, answer string) (Result, error)
	Skip(ctx context.Context) (Result, error)
	ShareText() string
}

// Status é a situação da partida.
type Status int

const (
	Playing Status = iota
	Won
	Lost
)

// State é a foto da partida que a TUI renderiza.
type State struct {
	Day     time.Time
	Number  int      // número do puzzle (#N)
	Step    int      // tentativa atual, 1-based
	Steps   int      // total de tentativas
	Guesses []string // texto de cada tentativa já feita ("" em pulo)
	Status  Status
}

// Result é o retorno de um chute ou pulo.
type Result struct {
	Correct bool
	Hint    string // observação para mostrar ao jogador (ex.: "artista certo")
	State   State
}

// Info descreve um jogo para o menu, mesmo que a engine ainda não exista.
type Info struct {
	ID          string
	Name        string
	Description string
	Available   bool // false = "em breve"
}

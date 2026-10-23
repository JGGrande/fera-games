// Package conexotest oferece um Provider fake sobre a fixture de
// conexo/fixture, para testar a TUI sem rede.
package conexotest

import (
	"context"
	"testing"

	"fera-games/internal/games/conexo"
	"fera-games/internal/games/conexo/fixture"
)

// Provider serve o #1117 (2026-10-22). Com Tokens definido, imita o Client:
// devolve ErrNoToken enquanto não houver token válido.
type Provider struct {
	Data   conexo.Puzzle
	Tokens *conexo.TokenStore
	Calls  int
}

// New carrega conexo/fixture.
func New(t testing.TB) *Provider {
	t.Helper()
	p, err := conexo.ParseDocument([]byte(fixture.Doc1117JSON))
	if err != nil {
		t.Fatal(err)
	}
	return &Provider{Data: p}
}

func (p *Provider) Puzzle(_ context.Context, _ string, id int) (conexo.Puzzle, error) {
	p.Calls++
	if p.Tokens != nil && p.Tokens.Get() == "" {
		return conexo.Puzzle{}, conexo.ErrNoToken
	}
	if id != p.Data.ID {
		return conexo.Puzzle{}, conexo.ErrNoPuzzle
	}
	return p.Data, nil
}

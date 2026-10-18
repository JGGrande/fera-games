// Package bandletest oferece um Provider fake sobre as fixtures de
// bandle/fixture, para testar engine e TUI sem rede nem dispositivo.
package bandletest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"fera-games/internal/audio/audiotest"
	"fera-games/internal/games/bandle"
	"fera-games/internal/games/bandle/fixture"
)

// Answer é a resposta do puzzle da fixture (2026-09-16, #1491).
const Answer = "Sam Smith - I'm Not the Only One"

// Provider serve o puzzle e o catálogo da fixture e mp3 sintéticos.
type Provider struct {
	Puzzle  bandle.Puzzle
	Catalog bandle.Catalog
	Stems   []int // estágios pedidos em Stem, na ordem
}

// New carrega as fixtures de bandle/fixture.
func New(t testing.TB) *Provider {
	t.Helper()
	var ps []bandle.Puzzle
	if err := json.Unmarshal([]byte(fixture.PlanningJSON), &ps); err != nil {
		t.Fatal(err)
	}
	var cat bandle.Catalog
	if err := json.Unmarshal([]byte(fixture.CatalogJSON), &cat); err != nil {
		t.Fatal(err)
	}
	return &Provider{Puzzle: ps[0], Catalog: cat}
}

func (p *Provider) DailyPuzzle(context.Context, time.Time) (bandle.Puzzle, error) {
	return p.Puzzle, nil
}

func (p *Provider) SongCatalog(context.Context) (bandle.Catalog, error) { return p.Catalog, nil }

func (p *Provider) Stem(_ context.Context, _ bandle.Puzzle, step int) (io.ReadCloser, error) {
	p.Stems = append(p.Stems, step)
	return io.NopCloser(bytes.NewReader(audiotest.SilentMP3(10))), nil
}

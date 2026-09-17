// Package bandletest oferece um Provider fake sobre os payloads reais de
// testdata/bandle, para testar engine e TUI sem rede nem dispositivo.
package bandletest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"fera-games/internal/audio/audiotest"
	"fera-games/internal/games/bandle"
)

// Answer é a resposta do puzzle de testdata (2026-09-16, #1491).
const Answer = "Sam Smith - I'm Not the Only One"

// Provider serve o puzzle e o catálogo de testdata e mp3 sintéticos.
type Provider struct {
	Puzzle  bandle.Puzzle
	Catalog bandle.Catalog
	Stems   []int // estágios pedidos em Stem, na ordem
}

// New carrega testdata/bandle; falha o teste se os arquivos não existirem.
func New(t testing.TB) *Provider {
	t.Helper()
	var ps []bandle.Puzzle
	if err := json.Unmarshal(read(t, "planning-2026-09-16.json"), &ps); err != nil {
		t.Fatal(err)
	}
	var cat bandle.Catalog
	if err := json.Unmarshal(read(t, "guesslist-songs.json"), &cat); err != nil {
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

// read localiza testdata/ a partir deste arquivo, independente do pacote que chama.
func read(t testing.TB, name string) []byte {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, "testdata", "bandle", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

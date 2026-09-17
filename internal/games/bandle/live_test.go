package bandle

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"fera-games/internal/httpx"
)

// TestLive fala com o servidor real. Fora da CI; rode com FERA_LIVE=1.
func TestLive(t *testing.T) {
	if os.Getenv("FERA_LIVE") == "" {
		t.Skip("defina FERA_LIVE=1 para falar com o servidor real")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := NewClient(httpx.New(30*time.Second), &httpx.Cache{Dir: t.TempDir()})
	now := time.Now()
	s, err := NewGame(c).Start(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Puzzle()
	if p.ID != PuzzleNumber(now) {
		t.Errorf("id do payload %d != PuzzleNumber %d", p.ID, PuzzleNumber(now))
	}
	if len(s.Catalog()) < 2000 {
		t.Errorf("catálogo com %d itens", len(s.Catalog()))
	}
	if _, ok := s.Catalog().Find(p.Song); !ok {
		t.Error("resposta não está no catálogo da sessão")
	}
	rc, err := s.Stem(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 3)
	_, _ = io.ReadFull(rc, head)
	rc.Close()
	if string(head) != "ID3" {
		t.Errorf("faixa 1 não parece mp3: %q", head)
	}
	t.Logf("puzzle #%d, %d instrumentos, %d músicas no catálogo", p.ID, p.Stages(), len(s.Catalog()))
}

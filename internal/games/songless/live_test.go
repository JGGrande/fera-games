package songless

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"fera-games/internal/audio"
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
	d, err := c.Daily(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("data do servidor: %s, %d categorias", d.Date, len(d.Categories))
	if len(d.Categories) != 3 {
		t.Fatalf("categorias = %+v", d.Categories)
	}
	entries, err := c.Search(ctx, "mountain")
	if err != nil || len(entries) == 0 {
		t.Fatalf("busca: %v %+v", err, entries)
	}
	s, err := NewGame(c).Start(ctx, time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := s.Audio(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 2)
	_, _ = io.ReadFull(rc, head)
	rc.Close()
	if len(head) != 2 || head[0] != 0xFF || head[1]&0xF0 != 0xF0 {
		t.Errorf("áudio não parece ADTS: %x", head)
	}
	// Decodifica de verdade (puro-Go, sem dispositivo).
	rc, _ = s.Audio(ctx)
	tr, err := audio.DecodeAAC(rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("challenge #%d, áudio %s, limite round 1 = %s", s.State().Number, tr.Duration().Round(time.Second), s.AudioLimit())
}

package conexo

import (
	"context"
	"os"
	"testing"
	"time"

	"fera-games/internal/httpx"
)

// TestLive fala com o Firestore real. Fora da CI; rode com FERA_LIVE=1 e um
// token do App Check em FERA_CONEXO_APPCHECK.
func TestLive(t *testing.T) {
	if os.Getenv("FERA_LIVE") == "" || os.Getenv(TokenEnv) == "" {
		t.Skip("defina FERA_LIVE=1 e " + TokenEnv + " para falar com o servidor real")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c := NewClient(httpx.New(15*time.Second), &httpx.Cache{Dir: t.TempDir()}, &TokenStore{})
	s, err := NewGame(c, "pt").Start(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p := s.Puzzle()
	// Sem spoiler: só o número e a forma.
	t.Logf("Conexo #%d: %d palavras soltas, créditos %q", p.ID, len(s.Items()), p.Credits.Author)
	if len(s.Items()) != Groups*GroupSize {
		t.Fatalf("tabuleiro com %d palavras", len(s.Items()))
	}
}

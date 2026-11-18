// Package feramusicastest oferece um catálogo semeado e uma AudioSource fake,
// para testar engine e TUI sem rede, yt-dlp, ffmpeg nem dispositivo de áudio.
package feramusicastest

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"fera-games/internal/games/feramusicas"
)

// Links das músicas semeadas.
const (
	BohemianURL = "https://www.youtube.com/watch?v=fJ9rUzIMcZQ"
	TeenURL     = "https://youtu.be/hTWKbfoikeg"
	BillieURL   = "https://music.youtube.com/watch?v=Zi_XLOBDo_Y"
)

// NewCatalog devolve um MemoryCatalog com 2 músicas em Rock e 1 em Pop.
func NewCatalog(t testing.TB) *feramusicas.MemoryCatalog {
	t.Helper()
	c := feramusicas.NewMemoryCatalog()
	for _, s := range []feramusicas.Song{
		{Name: "Queen - Bohemian Rhapsody", Category: "Rock", URL: BohemianURL},
		{Name: "Nirvana - Smells Like Teen Spirit", Category: "Rock", URL: TeenURL},
		{Name: "Michael Jackson - Billie Jean", Category: "Pop", URL: BillieURL},
	} {
		if _, err := c.Add(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

// Source é uma AudioSource que entrega silêncio PCM sem tocar na rede.
type Source struct {
	Seconds float64 // duração do silêncio (padrão 3 s)
	Err     error   // se não nulo, Fetch falha com ele
	// Gate, se não nulo, segura cada Fetch até ser fechado (ou o contexto
	// acabar): simula um download demorado.
	Gate chan struct{}

	mu      sync.Mutex
	fetched []feramusicas.Song
}

// Fetch implementa feramusicas.AudioSource.
func (s *Source) Fetch(ctx context.Context, song feramusicas.Song, _ time.Duration) (io.ReadCloser, error) {
	s.mu.Lock()
	s.fetched = append(s.fetched, song)
	gate := s.Gate
	s.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.Err != nil {
		return nil, s.Err
	}
	secs := s.Seconds
	if secs == 0 {
		secs = 3
	}
	return io.NopCloser(bytes.NewReader(make([]byte, int(secs*44100)*4))), nil
}

// Fetched são os VideoIDs pedidos até agora, em ordem.
func (s *Source) Fetched() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, len(s.fetched))
	for i, song := range s.fetched {
		ids[i] = song.VideoID
	}
	return ids
}

// FetchedSongs são as músicas pedidas até agora (com o início), em ordem.
func (s *Source) FetchedSongs() []feramusicas.Song {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]feramusicas.Song(nil), s.fetched...)
}

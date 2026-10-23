package conexo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fera-games/internal/games/conexo/fixture"
	"fera-games/internal/httpx"
)

// fakeFirestore serve o #1117 só com o token esperado (403 sem ele) e 404
// para os outros ids.
type fakeFirestore struct {
	*httptest.Server
	token string
	hits  atomic.Int32
}

func newFakeFirestore(t *testing.T, token string) *fakeFirestore {
	t.Helper()
	f := &fakeFirestore{token: token}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if r.Header.Get("X-Firebase-AppCheck") != f.token {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(fixture.ForbiddenJSON))
			return
		}
		if r.URL.Path != "/documents/conexo-daily-pt/1117" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(fixture.NotFoundJSON))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		_, _ = w.Write([]byte(fixture.Doc1117JSON))
	}))
	t.Cleanup(f.Close)
	return f
}

func newClient(f *fakeFirestore, cacheDir string, tokens *TokenStore) *Client {
	c := NewClient(httpx.New(5*time.Second), &httpx.Cache{Dir: cacheDir}, tokens)
	c.Endpoints = Endpoints{Documents: f.URL + "/documents"}
	return c
}

func tokensWith(t *testing.T, tok string) *TokenStore {
	t.Helper()
	t.Setenv(TokenEnv, "")
	s := &TokenStore{}
	if tok != "" {
		if _, err := s.Set(tok); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestClientFetchesWithTokenAndCaches(t *testing.T) {
	tok := fixture.Token(time.Now().Add(7 * 24 * time.Hour))
	f := newFakeFirestore(t, tok)
	dir := t.TempDir()
	c := newClient(f, dir, tokensWith(t, tok))

	p, err := c.Puzzle(context.Background(), "pt", 1117)
	if err != nil || p.ID != 1117 {
		t.Fatalf("Puzzle = %+v, %v", p.ID, err)
	}
	// Segunda leitura vem do cache, mesmo sem token.
	c2 := newClient(f, dir, tokensWith(t, ""))
	if _, err := c2.Puzzle(context.Background(), "pt", 1117); err != nil {
		t.Fatal(err)
	}
	if n := f.hits.Load(); n != 1 {
		t.Fatalf("servidor chamado %d vezes", n)
	}
}

func TestClientErrors(t *testing.T) {
	tok := fixture.Token(time.Now().Add(time.Hour))
	f := newFakeFirestore(t, tok)

	_, err := newClient(f, t.TempDir(), tokensWith(t, "")).Puzzle(context.Background(), "pt", 1117)
	if !errors.Is(err, ErrNoToken) || f.hits.Load() != 0 {
		t.Fatalf("sem token: %v (hits %d)", err, f.hits.Load())
	}
	other := fixture.Token(time.Now().Add(2 * time.Hour))
	_, err = newClient(f, t.TempDir(), tokensWith(t, other)).Puzzle(context.Background(), "pt", 1117)
	if !errors.Is(err, ErrAppCheck) {
		t.Fatalf("token recusado: %v", err)
	}
	_, err = newClient(f, t.TempDir(), tokensWith(t, tok)).Puzzle(context.Background(), "pt", 9999)
	if !errors.Is(err, ErrNoPuzzle) {
		t.Fatalf("404: %v", err)
	}
}

func TestTokenStore(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	t.Setenv(TokenEnv, "")
	path := filepath.Join(t.TempDir(), "sub", "conexo-appcheck.jwt")
	s := &TokenStore{Path: path, Now: func() time.Time { return now }}
	if s.Get() != "" {
		t.Fatal("store vazio deveria não ter token")
	}
	if _, err := s.Set(fixture.Token(now.Add(-time.Minute))); !errors.Is(err, ErrBadToken) {
		t.Fatalf("token expirado: %v", err)
	}
	if _, err := s.Set("lixo"); !errors.Is(err, ErrBadToken) {
		t.Fatalf("lixo: %v", err)
	}
	tok := fixture.Token(now.Add(48 * time.Hour))
	if _, err := s.Set(`{"ttl":"604800s","token":"` + tok + `"}`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("arquivo do token: %v %v", info, err)
	}

	// Outro processo lê do arquivo; o token expira com o relógio.
	s2 := &TokenStore{Path: path, Now: func() time.Time { return now }}
	if s2.Get() != tok {
		t.Fatal("token não foi lido do arquivo")
	}
	if exp, ok := s2.Expiry(); !ok || !exp.Equal(now.Add(48*time.Hour)) {
		t.Fatalf("Expiry = %v %v", exp, ok)
	}
	now = now.Add(49 * time.Hour)
	if s2.Get() != "" {
		t.Fatal("token expirado não deveria ser usado")
	}

	// A env vale quando não há token colado nesta execução.
	envTok := fixture.Token(now.Add(time.Hour))
	t.Setenv(TokenEnv, envTok)
	if s2.Get() != envTok {
		t.Fatal("env ignorada")
	}
	var nilStore *TokenStore
	if nilStore.Get() != "" {
		t.Fatal("store nil")
	}
}

package bandle

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fera-games/internal/httpx"
)

// encrypt é o inverso de Decrypt (a cifra é simétrica), para servir testdata cifrado.
func encrypt(key string, plain []byte) string {
	var mask byte
	for i := 0; i < len(key); i++ {
		mask ^= key[i]
	}
	out := make([]byte, len(plain))
	for i, b := range plain {
		out[i] = b ^ mask
	}
	return hex.EncodeToString(out)
}

// fakeServer imita o Firebase Auth, a Cloud Function getSignedUrl e o CDN.
type fakeServer struct {
	*httptest.Server
	key    string
	files  map[string][]byte // path → corpo já no formato servido
	hits   map[string]*atomic.Int32
	logins atomic.Int32
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{key: "chave-de-teste", files: map[string][]byte{}, hits: map[string]*atomic.Int32{}}
	fs.files[PlanningFile(day())] = []byte(encrypt(fs.key, testdata(t, "planning-2026-09-16.json")))
	fs.files[GuessListFile("songs")] = []byte(encrypt(fs.key, testdata(t, "guesslist-songs.json")))
	for i := 1; i <= 5; i++ {
		fs.files[fmt.Sprintf("/v2/files/253ab864e93431dfa181/%d.mp3", i)] = []byte("ID3stem" + fmt.Sprint(i))
	}
	for p := range fs.files {
		fs.hits[p] = &atomic.Int32{}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth", func(w http.ResponseWriter, r *http.Request) {
		fs.logins.Add(1)
		_, _ = fmt.Fprint(w, `{"idToken":"tok-123","expiresIn":"3600","localId":"anon"}`)
	})
	mux.HandleFunc("GET /getSignedUrl", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" {
			http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		// Produção assina qualquer caminho; objeto inexistente dá 403 no CDN.
		file := r.URL.Query().Get("file")
		_, _ = fmt.Fprintf(w, `{"url":%q,"utc":%d}`, fs.URL+"/cdn"+file+"?Signature=abc", time.Now().UnixMilli())
	})
	mux.HandleFunc("GET /cdn/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Referer"), "https://bandle.app/") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/cdn")
		body, ok := fs.files[path]
		if !ok {
			w.WriteHeader(http.StatusForbidden) // CloudFront responde 403 para objeto inexistente
			return
		}
		fs.hits[path].Add(1)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	})
	fs.Server = httptest.NewServer(mux)
	t.Cleanup(fs.Close)
	return fs
}

func day() time.Time {
	d, _ := time.Parse("2006-01-02", "2026-09-16")
	return d
}

func newTestClient(t *testing.T, fs *fakeServer, cacheDir string) *Client {
	t.Helper()
	c := NewClient(httpx.New(5*time.Second), &httpx.Cache{Dir: cacheDir})
	c.Endpoints = Endpoints{AuthSignUp: fs.URL + "/auth", SignedURL: fs.URL + "/getSignedUrl", Referer: Referer}
	c.Key = fs.key
	return c
}

func TestClientFullFlow(t *testing.T) {
	fs := newFakeServer(t)
	c := newTestClient(t, fs, t.TempDir())
	ctx := context.Background()

	p, err := c.DailyPuzzle(ctx, day())
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 1491 || p.Stages() != 6 {
		t.Fatalf("puzzle: %+v", p)
	}
	cat, err := c.SongCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) != 2576 {
		t.Fatalf("catálogo com %d itens", len(cat))
	}
	rc, err := c.Stem(ctx, p, 1)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if string(body) != "ID3stem1" {
		t.Fatalf("stem 1 = %q", body)
	}
	if n := fs.logins.Load(); n != 1 {
		t.Fatalf("login anônimo feito %d vezes, o token deveria ser reaproveitado", n)
	}
	if _, err := c.Stem(ctx, p, 6); err == nil {
		t.Fatal("estágio 6 não tem áudio")
	}
}

func TestClientUsesDiskCache(t *testing.T) {
	fs := newFakeServer(t)
	dir := t.TempDir()
	ctx := context.Background()

	c := newTestClient(t, fs, dir)
	if _, err := c.DailyPuzzle(ctx, day()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SongCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Stem(ctx, Puzzle{Path: "253ab864e93431dfa181", Instruments: make([]string, 6)}, 2); err != nil {
		t.Fatal(err)
	}

	// Um cliente novo (outro processo) com o mesmo diretório não volta à rede.
	c2 := newTestClient(t, fs, dir)
	if _, err := c2.DailyPuzzle(ctx, day()); err != nil {
		t.Fatal(err)
	}
	if _, err := c2.SongCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c2.Stem(ctx, Puzzle{Path: "253ab864e93431dfa181", Instruments: make([]string, 6)}, 2); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{PlanningFile(day()), GuessListFile("songs"), "/v2/files/253ab864e93431dfa181/2.mp3"} {
		if n := fs.hits[p].Load(); n != 1 {
			t.Errorf("%s baixado %d vezes, quero 1", p, n)
		}
	}

	// Catálogo com mais de 24 h é baixado de novo; o puzzle do dia nunca.
	c2.Now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	if _, err := c2.SongCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c2.DailyPuzzle(ctx, day()); err != nil {
		t.Fatal(err)
	}
	if n := fs.hits[GuessListFile("songs")].Load(); n != 2 {
		t.Errorf("catálogo velho não foi renovado (%d downloads)", n)
	}
	if n := fs.hits[PlanningFile(day())].Load(); n != 1 {
		t.Errorf("puzzle rebaixado (%d downloads)", n)
	}
}

func TestClientNoPuzzleYet(t *testing.T) {
	fs := newFakeServer(t)
	c := newTestClient(t, fs, "")
	_, err := c.DailyPuzzle(context.Background(), day().AddDate(0, 0, 1))
	if !errors.Is(err, ErrNoPuzzle) {
		t.Fatalf("erro = %v", err)
	}
}

func TestClientRefererIsSent(t *testing.T) {
	fs := newFakeServer(t)
	c := newTestClient(t, fs, "")
	c.Endpoints.Referer = "https://example.com/"
	_, err := c.Fetch(context.Background(), PlanningFile(day()))
	var se *httpx.StatusError
	if !errors.As(err, &se) || se.Status != http.StatusForbidden {
		t.Fatalf("sem Referer certo o CDN deveria dar 403, veio %v", err)
	}
}

func TestClientWithoutCache(t *testing.T) {
	fs := newFakeServer(t)
	c := NewClient(httpx.New(5*time.Second), nil)
	c.Endpoints = Endpoints{AuthSignUp: fs.URL + "/auth", SignedURL: fs.URL + "/getSignedUrl", Referer: Referer}
	c.Key = fs.key
	if _, err := c.DailyPuzzle(context.Background(), day()); err != nil {
		t.Fatal(err)
	}
}

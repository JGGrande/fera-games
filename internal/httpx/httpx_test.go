package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheRoundTripAndKeys(t *testing.T) {
	c := &Cache{Dir: t.TempDir()}
	if err := c.Put("/v2/planning/2026-09-16.txt", []byte("abc")); err != nil {
		t.Fatal(err)
	}
	data, at, ok := c.Get("/v2/planning/2026-09-16.txt")
	if !ok || string(data) != "abc" || time.Since(at) > time.Minute {
		t.Fatalf("Get = %q %v %v", data, at, ok)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "v2", "planning", "2026-09-16.txt")); err != nil {
		t.Fatalf("chave virou caminho inesperado: %v", err)
	}
	if err := c.Put("../../etc/passwd?x=1", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.Get("../../etc/passwd?x=1"); !ok {
		t.Fatal("chave com caracteres estranhos não foi gravada dentro de Dir")
	}
	if _, ok := c.Fresh("/v2/planning/2026-09-16.txt", time.Hour); !ok {
		t.Fatal("Fresh recém-gravado deveria valer")
	}
	if _, ok := c.Fresh("/v2/planning/2026-09-16.txt", -time.Second); ok {
		t.Fatal("Fresh com maxAge negativo não deveria valer")
	}
}

func TestNilCacheIsNoop(t *testing.T) {
	var c *Cache
	if err := c.Put("k", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.Get("k"); ok {
		t.Fatal("cache nil devolveu dado")
	}
}

func TestGetSetsUserAgentAndMapsStatus(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		if r.URL.Path == "/nope" {
			http.Error(w, "sumiu", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(time.Second)
	resp, err := c.Get(context.Background(), srv.URL+"/ok", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ua != UserAgent {
		t.Fatalf("User-Agent = %q", ua)
	}
	_, err = c.Get(context.Background(), srv.URL+"/nope", nil)
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 404 || se.Body != "sumiu" {
		t.Fatalf("erro = %v", err)
	}
}

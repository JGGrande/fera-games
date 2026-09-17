// Package httpx é o cliente HTTP comum: timeout, User-Agent identificável e
// cache simples em disco para respeitar a regra de "1 download por dia".
package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UserAgent identifica o cliente nos servidores dos jogos.
const UserAgent = "FeraGames/0.1 (uso pessoal; cliente de terminal)"

// Client envolve http.Client acrescentando o User-Agent em toda requisição.
type Client struct {
	HTTP      *http.Client
	UserAgent string
}

// New cria um cliente com o timeout dado (0 = sem timeout).
func New(timeout time.Duration) *Client {
	return &Client{HTTP: &http.Client{Timeout: timeout}, UserAgent: UserAgent}
}

// Do executa a requisição com o User-Agent do cliente.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	return c.HTTP.Do(req)
}

// Get faz um GET com headers extras e devolve a resposta; status >= 300 vira erro
// (exceto 304), com o começo do corpo na mensagem.
func (c *Client) Get(ctx context.Context, url string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header[k] = v
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotModified {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, &StatusError{URL: url, Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	return resp, nil
}

// StatusError é um HTTP status fora de 2xx.
type StatusError struct {
	URL    string
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.Status)
	}
	return fmt.Sprintf("GET %s: HTTP %d: %s", e.URL, e.Status, e.Body)
}

// Cache guarda corpos de resposta em arquivos dentro de Dir. Um Cache nil ou
// com Dir vazio não guarda nada, o que simplifica os chamadores.
type Cache struct {
	Dir string
}

// Get devolve o conteúdo da chave e a hora em que foi gravado.
func (c *Cache) Get(key string) ([]byte, time.Time, bool) {
	if c == nil || c.Dir == "" {
		return nil, time.Time{}, false
	}
	path := c.path(key)
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false
	}
	return data, info.ModTime(), true
}

// Fresh é Get restrito a entradas com menos de maxAge.
func (c *Cache) Fresh(key string, maxAge time.Duration) ([]byte, bool) {
	data, at, ok := c.Get(key)
	if !ok || time.Since(at) > maxAge {
		return nil, false
	}
	return data, true
}

// Put grava a chave; escreve num temporário e renomeia para não deixar
// arquivo pela metade.
func (c *Cache) Put(key string, data []byte) error {
	if c == nil || c.Dir == "" {
		return nil
	}
	path := c.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name()) // melhor esforço
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name()) // melhor esforço
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// path converte a chave num caminho seguro dentro de Dir ("/" vira subpasta).
func (c *Cache) path(key string) string {
	parts := strings.Split(strings.Trim(key, "/"), "/")
	for i, p := range parts {
		parts[i] = sanitize(p)
	}
	return filepath.Join(append([]string{c.Dir}, parts...)...)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 || strings.Trim(b.String(), ".") == "" {
		return "_"
	}
	return b.String()
}

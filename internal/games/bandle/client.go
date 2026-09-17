package bandle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"fera-games/internal/httpx"
)

// Provider é o que a engine precisa do servidor. Client é a implementação
// real; os testes usam httptest ou um fake.
type Provider interface {
	DailyPuzzle(ctx context.Context, day time.Time) (Puzzle, error)
	SongCatalog(ctx context.Context) (Catalog, error)
	Stem(ctx context.Context, p Puzzle, step int) (io.ReadCloser, error)
}

// Endpoints são as URLs do fluxo; os testes apontam para um httptest.Server.
type Endpoints struct {
	AuthSignUp string // POST → idToken
	SignedURL  string // GET ?file= → {url, utc}
	Referer    string // exigido pelo CloudFront
}

// DefaultEndpoints são os de produção, mapeados na Fase 0.
var DefaultEndpoints = Endpoints{
	AuthSignUp: AuthSignUpURL,
	SignedURL:  SignedURLEndpoint,
	Referer:    Referer,
}

// Validade do cache. O puzzle é imutável por dia; o catálogo muda raramente
// (regra do projeto: no máximo 1 download por dia); os mp3 nunca mudam.
const (
	catalogMaxAge = 24 * time.Hour
	tokenSlack    = 5 * time.Minute
)

// Client fala com o Bandle: login anônimo, URL assinada, download com Referer,
// decifragem e cache em disco.
type Client struct {
	HTTP      *httpx.Client
	Cache     *httpx.Cache
	Endpoints Endpoints
	Key       string // chave de decifragem; vazio = EncryptionKey()
	Now       func() time.Time

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewClient cria o cliente de produção. cache pode ser nil.
func NewClient(h *httpx.Client, cache *httpx.Cache) *Client {
	return &Client{HTTP: h, Cache: cache, Endpoints: DefaultEndpoints, Key: EncryptionKey(), Now: time.Now}
}

var _ Provider = (*Client)(nil)

// Token devolve um idToken anônimo válido, reaproveitando o anterior enquanto durar.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoints.AuthSignUp,
		strings.NewReader(`{"returnSecureToken":true}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("login anônimo: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		IDToken   string `json:"idToken"`
		ExpiresIn string `json:"expiresIn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.IDToken == "" {
		return "", fmt.Errorf("login anônimo: HTTP %d (%v)", resp.StatusCode, err)
	}
	ttl := time.Hour
	if d, err := time.ParseDuration(out.ExpiresIn + "s"); err == nil {
		ttl = d
	}
	c.token, c.tokenExp = out.IDToken, c.Now().Add(ttl-tokenSlack)
	return c.token, nil
}

// SignedURL troca um caminho do CDN por uma URL assinada (válida por ~60 s).
func (c *Client) SignedURL(ctx context.Context, path string) (string, error) {
	token, err := c.Token(ctx)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Get(ctx, c.Endpoints.SignedURL+"?file="+url.QueryEscape(path),
		http.Header{"Authorization": {"Bearer " + token}})
	if err != nil {
		return "", fmt.Errorf("getSignedUrl %s: %w", path, err)
	}
	defer resp.Body.Close()
	var out struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.URL == "" {
		return "", fmt.Errorf("getSignedUrl %s: resposta inesperada (%v)", path, err)
	}
	return out.URL, nil
}

// Open abre um arquivo do CDN para leitura em stream (áudio).
func (c *Client) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	signed, err := c.SignedURL(ctx, path)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Get(ctx, signed, http.Header{"Referer": {c.Endpoints.Referer}})
	if err != nil {
		return nil, fmt.Errorf("baixar %s: %w", path, err)
	}
	return resp.Body, nil
}

// Fetch baixa um arquivo inteiro.
func (c *Client) Fetch(ctx context.Context, path string) ([]byte, error) {
	rc, err := c.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// fetchText baixa um .txt (cifrado), usando o cache quando fresh permitir,
// e devolve o JSON decifrado. O cache guarda o arquivo ainda cifrado para
// não deixar a resposta em claro no disco.
func (c *Client) fetchText(ctx context.Context, path string, fresh func([]byte, time.Time) bool) ([]byte, error) {
	var raw []byte
	if data, at, ok := c.Cache.Get(path); ok && fresh(data, at) {
		raw = data
	} else {
		var err error
		if raw, err = c.Fetch(ctx, path); err != nil {
			return nil, err
		}
		if err := c.Cache.Put(path, raw); err != nil {
			return nil, fmt.Errorf("gravar cache de %s: %w", path, err)
		}
	}
	plain, err := Decrypt(c.key(), string(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return plain, nil
}

func (c *Client) key() string {
	if c.Key == "" {
		return EncryptionKey()
	}
	return c.Key
}

// ErrNoPuzzle indica que o servidor ainda não publicou o puzzle do dia.
var ErrNoPuzzle = errors.New("o puzzle de hoje ainda não está disponível")

// DailyPuzzle baixa o puzzle do dia civil de day (no fuso de day).
func (c *Client) DailyPuzzle(ctx context.Context, day time.Time) (Puzzle, error) {
	path := PlanningFile(day)
	plain, err := c.fetchText(ctx, path, func([]byte, time.Time) bool { return true })
	if err != nil {
		var se *httpx.StatusError
		if errors.As(err, &se) && (se.Status == http.StatusForbidden || se.Status == http.StatusNotFound) {
			return Puzzle{}, fmt.Errorf("%w (%s)", ErrNoPuzzle, FormatDate(day))
		}
		return Puzzle{}, err
	}
	var ps []Puzzle
	if err := json.Unmarshal(plain, &ps); err != nil || len(ps) == 0 {
		return Puzzle{}, fmt.Errorf("%s: payload inesperado (%v): %.120s", path, err, plain)
	}
	p := ps[0]
	if len(p.Instruments) == 0 || p.Song == "" || p.Path == "" {
		return Puzzle{}, fmt.Errorf("%s: puzzle incompleto: %+v", path, p)
	}
	return p, nil
}

// SongCatalog baixa o guesslist padrão, no máximo uma vez por dia.
func (c *Client) SongCatalog(ctx context.Context) (Catalog, error) {
	path := GuessListFile("songs")
	plain, err := c.fetchText(ctx, path, func(_ []byte, at time.Time) bool {
		return c.Now().Sub(at) < catalogMaxAge
	})
	if err != nil {
		return nil, err
	}
	var cat Catalog
	if err := json.Unmarshal(plain, &cat); err != nil || len(cat) == 0 {
		return nil, fmt.Errorf("%s: catálogo inesperado (%v)", path, err)
	}
	return cat, nil
}

// Stem devolve o mp3 do estágio step (1..AudioStages), do cache se já baixado.
func (c *Client) Stem(ctx context.Context, p Puzzle, step int) (io.ReadCloser, error) {
	if step < 1 || step > p.AudioStages() {
		return nil, fmt.Errorf("estágio %d fora de 1..%d", step, p.AudioStages())
	}
	path := StemFile(p, step)
	if data, _, ok := c.Cache.Get(path); ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	data, err := c.Fetch(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := c.Cache.Put(path, data); err != nil {
		return nil, fmt.Errorf("gravar cache de %s: %w", path, err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

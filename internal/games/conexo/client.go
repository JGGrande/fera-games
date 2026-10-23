package conexo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"fera-games/internal/httpx"
)

// Provider é o que a engine precisa do servidor. Client é a implementação
// real; os testes usam httptest ou um fake.
type Provider interface {
	Puzzle(ctx context.Context, lang string, id int) (Puzzle, error)
}

// Endpoints são as URLs do fluxo; os testes apontam para um httptest.Server.
type Endpoints struct {
	Documents string // prefixo …/documents do Firestore REST
}

// DefaultEndpoints são os de produção.
var DefaultEndpoints = Endpoints{Documents: DocumentsURL}

// Client lê o documento do dia no Firestore com o token do App Check e o
// guarda em cache: o documento é imutável por dia (salvo correções raras),
// então o token só é exigido na primeira abertura de cada dia.
type Client struct {
	HTTP      *httpx.Client
	Cache     *httpx.Cache
	Endpoints Endpoints
	Tokens    *TokenStore
}

// NewClient cria o cliente de produção. cache e tokens podem ser nil.
func NewClient(h *httpx.Client, cache *httpx.Cache, tokens *TokenStore) *Client {
	return &Client{HTTP: h, Cache: cache, Endpoints: DefaultEndpoints, Tokens: tokens}
}

var _ Provider = (*Client)(nil)

var (
	// ErrNoPuzzle indica que o documento do dia não existe (404).
	ErrNoPuzzle = errors.New("o Conexo de hoje ainda não está disponível")
	// ErrAppCheck é o 403 do Firestore: token expirado, adulterado ou ausente
	// (o servidor não diferencia).
	ErrAppCheck = errors.New("o token do App Check expirou ou é inválido")
)

// Puzzle devolve o puzzle id do idioma, do cache se já baixado.
func (c *Client) Puzzle(ctx context.Context, lang string, id int) (Puzzle, error) {
	key := fmt.Sprintf("conexo-%s-%d.json", lang, id)
	if data, _, ok := c.Cache.Get(key); ok {
		if p, err := ParseDocument(data); err == nil && p.ID == id {
			return p, nil
		}
	}
	tok := c.Tokens.Get()
	if tok == "" {
		return Puzzle{}, ErrNoToken
	}
	h := http.Header{}
	h.Set("X-Firebase-AppCheck", tok)
	resp, err := c.HTTP.Get(ctx, fmt.Sprintf("%s/%s/%d", c.Endpoints.Documents, Collection(lang), id), h)
	if err != nil {
		var se *httpx.StatusError
		if errors.As(err, &se) {
			switch se.Status {
			case http.StatusForbidden:
				return Puzzle{}, fmt.Errorf("%w (HTTP 403)", ErrAppCheck)
			case http.StatusNotFound:
				return Puzzle{}, fmt.Errorf("%w (#%d)", ErrNoPuzzle, id)
			}
		}
		return Puzzle{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Puzzle{}, err
	}
	p, err := ParseDocument(body)
	if err != nil {
		return Puzzle{}, err
	}
	if p.ID != id {
		return Puzzle{}, fmt.Errorf("conexo: pedi o #%d e veio o #%d", id, p.ID)
	}
	if err := c.Cache.Put(key, body); err != nil {
		return Puzzle{}, fmt.Errorf("gravar cache de %s: %w", key, err)
	}
	return p, nil
}

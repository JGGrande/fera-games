package songless

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"fera-games/internal/httpx"
)

// Provider é o que a engine precisa do servidor. Client é a implementação
// real; os testes usam httptest ou um fake.
type Provider interface {
	Daily(ctx context.Context, day time.Time) (Daily, error)
	Search(ctx context.Context, query string) ([]Entry, error)
	Guess(ctx context.Context, g Guess) (Outcome, *Answer, error)
	Audio(ctx context.Context, c Category) (io.ReadCloser, error)
	Stats(ctx context.Context, date string, categoryID int) (Stats, error)
}

// Endpoints são as URLs do fluxo; os testes apontam para um httptest.Server.
type Endpoints struct {
	Page string // GET → HTML com flight-data
	API  string // prefixo de /songless/...
}

// DefaultEndpoints são os de produção.
var DefaultEndpoints = Endpoints{Page: PageURL, API: APIBase}

// Client fala com o Songless: scrape da página, busca, chute, stats e áudio
// do CDN, com cache em disco (1 download de áudio por dia).
type Client struct {
	HTTP      *httpx.Client
	Cache     *httpx.Cache
	Endpoints Endpoints
	Now       func() time.Time
}

// NewClient cria o cliente de produção. cache pode ser nil.
func NewClient(h *httpx.Client, cache *httpx.Cache) *Client {
	return &Client{HTTP: h, Cache: cache, Endpoints: DefaultEndpoints, Now: time.Now}
}

var _ Provider = (*Client)(nil)

// ErrNoPuzzle indica que o servidor ainda não publicou o puzzle do dia
// (DATE_NOT_ALLOWED: dia futuro ou virada de fuso).
var ErrNoPuzzle = errors.New("a música de hoje ainda não está disponível")

// ErrSongNotFound é um chute com songId desconhecido.
var ErrSongNotFound = errors.New("música não encontrada")

// Daily raspa a página e devolve a data do servidor + os áudios. O day
// passado é só uma dica (a virada segue o servidor): a challengeDate usada
// no jogo é sempre Daily.Date.
func (c *Client) Daily(ctx context.Context, _ time.Time) (Daily, error) {
	const key = "songless-page.html"
	if data, ok := c.Cache.Fresh(key, time.Hour); ok {
		if d, err := ParseDailyPage(string(data)); err == nil {
			return d, nil
		}
	}
	resp, err := c.HTTP.Get(ctx, c.Endpoints.Page, nil)
	if err != nil {
		return Daily{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return Daily{}, err
	}
	d, err := ParseDailyPage(string(body))
	if err != nil {
		return Daily{}, err
	}
	_ = c.Cache.Put(key, body)
	return d, nil
}

// Search devolve o autocomplete remoto para o texto digitado.
func (c *Client) Search(ctx context.Context, query string) ([]Entry, error) {
	u := c.Endpoints.API + "/songless/search?query=" + url.QueryEscape(query)
	resp, err := c.HTTP.Get(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []Entry
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("busca do Songless: %w", err)
	}
	return out, nil
}

// guessResp é o corpo do /songless/guess (ou um {code} de erro).
type guessResp struct {
	State  Outcome `json:"state"`
	Answer *Answer `json:"answer"`
	Code   string  `json:"code"`
}

// Guess chuta (ou pula, com SongID nil) no round atual.
func (c *Client) Guess(ctx context.Context, g Guess) (Outcome, *Answer, error) {
	body, _ := json.Marshal(map[string]any{
		"challengeDate": g.Date,
		"categoryId":    g.CategoryID,
		"roundIndex":    g.RoundIndex,
		"songId":        g.SongID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.Endpoints.API+"/songless/guess", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// O httpx.Client transforma status >=300 em *StatusError; mas um
		// {code} pode vir com 200 ou 4xx — tratamos o corpo abaixo.
		var se *httpx.StatusError
		if errors.As(err, &se) {
			if code := errorCode(se.Body); code != "" {
				return "", nil, codeError(code)
			}
			return "", nil, err
		}
		return "", nil, err
	}
	defer resp.Body.Close()
	var out guessResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil, fmt.Errorf("chute do Songless: %w", err)
	}
	if out.Code != "" {
		return "", nil, codeError(out.Code)
	}
	switch out.State {
	case OutcomeCorrect, OutcomePresent, OutcomeIncorrect, OutcomeSkipped:
		return out.State, out.Answer, nil
	default:
		return "", nil, fmt.Errorf("chute do Songless: state inesperado %q", out.State)
	}
}

func errorCode(body string) string {
	var v struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return ""
	}
	return v.Code
}

func codeError(code string) error {
	switch code {
	case "DATE_NOT_ALLOWED":
		return fmt.Errorf("%w (%s)", ErrNoPuzzle, code)
	case "SONG_NOT_FOUND":
		return fmt.Errorf("%w (%s)", ErrSongNotFound, code)
	default:
		return fmt.Errorf("songless: %s", code)
	}
}

// Audio devolve o .aac da categoria, do cache se já baixado.
func (c *Client) Audio(ctx context.Context, cat Category) (io.ReadCloser, error) {
	// A chave carrega o hash da URL: cada dia tem um arquivo novo e a chave só
	// com a categoria serviria o áudio de um dia anterior.
	sum := sha256.Sum256([]byte(cat.FileURL))
	key := fmt.Sprintf("songless-audio-%d-%x.aac", cat.ID, sum[:8])
	if data, _, ok := c.Cache.Get(key); ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	resp, err := c.HTTP.Get(ctx, cat.FileURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("áudio vazio para %s", cat.Name)
	}
	if err := c.Cache.Put(key, data); err != nil {
		return nil, fmt.Errorf("gravar cache do áudio: %w", err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Stats devolve a distribuição pública do dia/categoria.
func (c *Client) Stats(ctx context.Context, date string, categoryID int) (Stats, error) {
	u := fmt.Sprintf("%s/songless/stats?challengeDate=%s&categoryId=%d",
		c.Endpoints.API, url.QueryEscape(date), categoryID)
	resp, err := c.HTTP.Get(ctx, u, nil)
	if err != nil {
		return Stats{}, err
	}
	defer resp.Body.Close()
	var out Stats
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Stats{}, fmt.Errorf("stats do Songless: %w", err)
	}
	return out, nil
}

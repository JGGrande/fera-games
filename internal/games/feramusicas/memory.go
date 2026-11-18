package feramusicas

import (
	"context"
	"sort"
	"strconv"
	"sync"
)

// MemoryCatalog guarda as músicas só em memória: some quando o programa
// fecha. É seguro para uso concorrente.
type MemoryCatalog struct {
	mu    sync.RWMutex
	songs []Song // ordem de cadastro
	next  int
}

// NewMemoryCatalog cria um catálogo vazio.
func NewMemoryCatalog() *MemoryCatalog { return &MemoryCatalog{} }

var _ Catalog = (*MemoryCatalog)(nil)

func (c *MemoryCatalog) List(ctx context.Context) ([]Song, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Song(nil), c.songs...), nil
}

func (c *MemoryCatalog) ByCategory(ctx context.Context, category string) ([]Song, error) {
	all, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	key := CategoryKey(category)
	var out []Song
	for _, s := range all {
		if CategoryKey(s.Category) == key {
			out = append(out, s)
		}
	}
	return out, nil
}

func (c *MemoryCatalog) Categories(ctx context.Context) ([]string, error) {
	all, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return DistinctCategories(all), nil
}

// DistinctCategories devolve as categorias sem repetir (ignorando caixa),
// ordenadas; vale a grafia da primeira música da lista (a mais antiga, se a
// lista vier em ordem de cadastro). Toda implementação de Catalog usa isto.
func DistinctCategories(songs []Song) []string {
	seen := map[string]string{}
	for _, s := range songs {
		k := CategoryKey(s.Category)
		if _, ok := seen[k]; !ok {
			seen[k] = CleanText(s.Category)
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = seen[k]
	}
	return out
}

func (c *MemoryCatalog) Add(ctx context.Context, s Song) (Song, error) {
	if err := ctx.Err(); err != nil {
		return Song{}, err
	}
	s, err := PrepareSong(s)
	if err != nil {
		return Song{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, other := range c.songs {
		if other.VideoID == s.VideoID {
			return Song{}, ErrDuplicate
		}
	}
	c.next++
	s.ID = "s" + strconv.Itoa(c.next)
	c.songs = append(c.songs, s)
	return s, nil
}

// Update troca os dados da música id, mantendo o ID e a posição no cadastro.
func (c *MemoryCatalog) Update(ctx context.Context, id string, s Song) (Song, error) {
	if err := ctx.Err(); err != nil {
		return Song{}, err
	}
	s, err := PrepareSong(s)
	if err != nil {
		return Song{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	at := -1
	for i, other := range c.songs {
		switch {
		case other.ID == id:
			at = i
		case other.VideoID == s.VideoID:
			return Song{}, ErrDuplicate // o link novo é de outra música
		}
	}
	if at < 0 {
		return Song{}, ErrNotFound
	}
	s.ID = id
	c.songs[at] = s
	return s, nil
}

func (c *MemoryCatalog) Remove(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, s := range c.songs {
		if s.ID == id {
			c.songs = append(c.songs[:i:i], c.songs[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

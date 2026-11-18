package feramusicas

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

func add(t *testing.T, c Catalog, name, category, url string) Song {
	t.Helper()
	s, err := c.Add(ctx, Song{Name: name, Category: category, URL: url})
	if err != nil {
		t.Fatalf("Add(%q): %v", name, err)
	}
	return s
}

func TestPrepareSong(t *testing.T) {
	good := Song{Name: "  Queen   -  Bohemian Rhapsody ", Category: " Rock ", URL: " youtu.be/fJ9rUzIMcZQ "}
	got, err := PrepareSong(good)
	if err != nil {
		t.Fatal(err)
	}
	want := Song{Name: "Queen - Bohemian Rhapsody", Category: "Rock", URL: "youtu.be/fJ9rUzIMcZQ", VideoID: "fJ9rUzIMcZQ"}
	if got != want {
		t.Fatalf("PrepareSong = %+v, quero %+v", got, want)
	}

	cases := []struct {
		name  string
		song  Song
		field string
	}{
		{"nome vazio", Song{Name: " ", Category: "Rock", URL: good.URL}, FieldName},
		{"nome longo", Song{Name: strings.Repeat("a", MaxNameLen+1), Category: "Rock", URL: good.URL}, FieldName},
		{"categoria vazia", Song{Name: "x", Category: "", URL: good.URL}, FieldCategory},
		{"categoria longa", Song{Name: "x", Category: strings.Repeat("a", MaxCategoryLen+1), URL: good.URL}, FieldCategory},
		{"link vazio", Song{Name: "x", Category: "Rock"}, FieldURL},
		{"link ruim", Song{Name: "x", Category: "Rock", URL: "https://example.com"}, FieldURL},
	}
	for _, tc := range cases {
		_, err := PrepareSong(tc.song)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != tc.field || !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, quero ValidationError(%s)", tc.name, err, tc.field)
		}
	}
}

func TestMemoryCatalog(t *testing.T) {
	c := NewMemoryCatalog()
	if cats, _ := c.Categories(ctx); len(cats) != 0 {
		t.Fatalf("catálogo novo deveria estar vazio: %v", cats)
	}

	a := add(t, c, "Queen - Bohemian Rhapsody", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	b := add(t, c, "Nirvana - Teen Spirit", "rock", "https://youtu.be/hTWKbfoikeg")
	p := add(t, c, "Michael Jackson - Billie Jean", "Pop", "https://youtu.be/Zi_XLOBDo_Y")
	if a.ID == "" || a.ID == b.ID || b.ID == p.ID {
		t.Fatalf("IDs deveriam ser únicos e preenchidos: %q %q %q", a.ID, b.ID, p.ID)
	}

	cats, _ := c.Categories(ctx)
	if want := []string{"Pop", "Rock"}; !reflect.DeepEqual(cats, want) {
		t.Fatalf("Categories = %v, quero %v (Rock/rock é a mesma, vale a 1ª grafia)", cats, want)
	}
	rock, _ := c.ByCategory(ctx, "  ROCK ")
	if len(rock) != 2 || rock[0].ID != a.ID || rock[1].ID != b.ID {
		t.Fatalf("ByCategory(ROCK) = %+v", rock)
	}
	if none, _ := c.ByCategory(ctx, "Funk"); len(none) != 0 {
		t.Fatalf("ByCategory(Funk) = %+v", none)
	}

	// Mesmo vídeo por outro link: duplicado.
	_, err := c.Add(ctx, Song{Name: "Outra", Category: "Pop", URL: "https://www.youtube.com/watch?v=fJ9rUzIMcZQ&t=3"})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Add duplicado = %v, quero ErrDuplicate", err)
	}
	if _, err := c.Add(ctx, Song{Name: "", Category: "Pop", URL: "https://youtu.be/fJ9rUzIMcZQ"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Add inválido = %v, quero ErrInvalid", err)
	}

	if err := c.Remove(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remove repetido = %v, quero ErrNotFound", err)
	}
	all, _ := c.List(ctx)
	if len(all) != 2 || all[0].ID != b.ID {
		t.Fatalf("List após Remove = %+v", all)
	}
	// Depois de remover, o mesmo vídeo pode voltar.
	add(t, c, "Queen - Bohemian Rhapsody", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
}

func TestMemoryCatalogRespeitaContexto(t *testing.T) {
	c := NewMemoryCatalog()
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.List(cctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List = %v", err)
	}
	if _, err := c.Add(cctx, Song{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Add = %v", err)
	}
}

func TestMemoryCatalogListEhCopia(t *testing.T) {
	c := NewMemoryCatalog()
	add(t, c, "A", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	l, _ := c.List(ctx)
	l[0].Name = "mexi"
	if l2, _ := c.List(ctx); l2[0].Name != "A" {
		t.Fatal("alterar o resultado de List não pode mudar o catálogo")
	}
}

func TestMemoryCatalogConcorrente(t *testing.T) {
	c := NewMemoryCatalog()
	var wg sync.WaitGroup
	ids := []string{"fJ9rUzIMcZQ", "hTWKbfoikeg", "Zi_XLOBDo_Y", "aaaaaaaaaaa", "bbbbbbbbbbb"}
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Add(ctx, Song{Name: id, Category: "X", URL: "https://youtu.be/" + id})
			_, _ = c.List(ctx)
			_, _ = c.Categories(ctx)
		}()
	}
	wg.Wait()
	if l, _ := c.List(ctx); len(l) != len(ids) {
		t.Fatalf("len = %d, quero %d", len(l), len(ids))
	}
}

func TestMemoryCatalogUpdate(t *testing.T) {
	c := NewMemoryCatalog()
	a := add(t, c, "Queen - Bohemian Rhapsody", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	b := add(t, c, "Nirvana - Teen Spirit", "Rock", "https://youtu.be/hTWKbfoikeg")

	// Troca nome, categoria, link e início: mantém o ID e a posição.
	got, err := c.Update(ctx, a.ID, Song{Name: "  Queen -  Radio Ga Ga ", Category: "Pop", URL: "https://youtu.be/azdwsXLmrHE", Start: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != a.ID || got.Name != "Queen - Radio Ga Ga" || got.VideoID != "azdwsXLmrHE" || got.Start != 30*time.Second {
		t.Fatalf("Update = %+v", got)
	}
	all, _ := c.List(ctx)
	if len(all) != 2 || all[0] != got || all[1] != b {
		t.Fatalf("List depois do Update = %+v", all)
	}
	if cats, _ := c.Categories(ctx); !reflect.DeepEqual(cats, []string{"Pop", "Rock"}) {
		t.Fatalf("Categories = %v", cats)
	}

	// Mesmo link da própria música: ok (só muda o resto).
	if _, err := c.Update(ctx, b.ID, Song{Name: "Nirvana", Category: "Rock", URL: "https://www.youtube.com/watch?v=hTWKbfoikeg"}); err != nil {
		t.Fatalf("manter o próprio link: %v", err)
	}
	// Link de outra música: duplicado.
	if _, err := c.Update(ctx, b.ID, Song{Name: "x", Category: "Rock", URL: "https://youtu.be/azdwsXLmrHE"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("link de outra música = %v, quero ErrDuplicate", err)
	}
	if _, err := c.Update(ctx, "nao-existe", Song{Name: "x", Category: "Rock", URL: "https://youtu.be/ccccccccccc"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("id inexistente = %v, quero ErrNotFound", err)
	}
	var ve *ValidationError
	if _, err := c.Update(ctx, b.ID, Song{Name: "", Category: "Rock", URL: "https://youtu.be/hTWKbfoikeg"}); !errors.As(err, &ve) || ve.Field != FieldName {
		t.Fatalf("nome vazio = %v, quero ValidationError(name)", err)
	}
	// O link antigo da música editada fica livre para outra.
	add(t, c, "Queen - Bohemian Rhapsody", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
}

func TestFailingCatalogUpdate(t *testing.T) {
	boom := errors.New("fora do ar")
	if _, err := FailingCatalog(boom).Update(ctx, "x", Song{}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

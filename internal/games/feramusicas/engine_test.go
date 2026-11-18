package feramusicas_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"fera-games/internal/game"
	"fera-games/internal/games/feramusicas"
	"fera-games/internal/games/feramusicas/feramusicastest"
)

var (
	ctx = context.Background()
	day = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
)

func newGame(t *testing.T) (*feramusicas.Game, *feramusicastest.Source) {
	t.Helper()
	src := &feramusicastest.Source{}
	return feramusicas.NewGame(feramusicastest.NewCatalog(t), src), src
}

func start(t *testing.T, g *feramusicas.Game, category string) *feramusicas.Session {
	t.Helper()
	s, err := g.Start(ctx, day, category)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// answerOf descobre a música do dia pelo mesmo caminho da engine.
func answerOf(t *testing.T, g *feramusicas.Game, category string) feramusicas.Song {
	t.Helper()
	songs, _ := g.Catalog().ByCategory(ctx, category)
	a, ok := feramusicas.Pick(day, category, songs)
	if !ok {
		t.Fatal("categoria vazia")
	}
	return a
}

func wrongSong(t *testing.T, g *feramusicas.Game, answer feramusicas.Song) feramusicas.Song {
	t.Helper()
	all, _ := g.Catalog().List(ctx)
	for _, s := range all {
		if s.Name != answer.Name {
			return s
		}
	}
	t.Fatal("sem música errada")
	return feramusicas.Song{}
}

func TestPickEhDeterministicoEIndependenteDaOrdem(t *testing.T) {
	songs := []feramusicas.Song{{VideoID: "ccccccccccc"}, {VideoID: "aaaaaaaaaaa"}, {VideoID: "bbbbbbbbbbb"}}
	reversed := []feramusicas.Song{songs[2], songs[1], songs[0]}
	a, ok := feramusicas.Pick(day, "Rock", songs)
	if !ok {
		t.Fatal("Pick falhou")
	}
	if b, _ := feramusicas.Pick(day, "  rock ", reversed); a.VideoID != b.VideoID {
		t.Fatalf("a ordem do catálogo/caixa da categoria mudou a escolha: %s vs %s", a.VideoID, b.VideoID)
	}
	if _, ok := feramusicas.Pick(day, "Rock", nil); ok {
		t.Fatal("categoria vazia não pode escolher nada")
	}
	// Dias diferentes devem, em algum momento, escolher músicas diferentes.
	seen := map[string]bool{}
	for d := 0; d < 30; d++ {
		s, _ := feramusicas.Pick(day.AddDate(0, 0, d), "Rock", songs)
		seen[s.VideoID] = true
	}
	if len(seen) < 2 {
		t.Fatalf("30 dias escolheram sempre a mesma música: %v", seen)
	}
}

func TestLimitFor(t *testing.T) {
	want := []time.Duration{130 * time.Millisecond, 530 * time.Millisecond, 2 * time.Second, 8 * time.Second, 15 * time.Second}
	for i, w := range want {
		if got := feramusicas.LimitFor(i + 1); got != w {
			t.Errorf("LimitFor(%d) = %v, quero %v", i+1, got, w)
		}
	}
	if feramusicas.LimitFor(0) != want[0] || feramusicas.LimitFor(99) != want[4] {
		t.Error("LimitFor deveria fazer clamp em 1..5")
	}
}

func TestStartCategoriaVazia(t *testing.T) {
	g, _ := newGame(t)
	if _, err := g.Start(ctx, day, "Funk"); !errors.Is(err, feramusicas.ErrEmptyCategory) {
		t.Fatalf("Start(Funk) = %v, quero ErrEmptyCategory", err)
	}
	empty := feramusicas.NewGame(feramusicas.NewMemoryCatalog(), &feramusicastest.Source{})
	if _, err := empty.NewSession(ctx, day); !errors.Is(err, feramusicas.ErrEmptyCatalog) {
		t.Fatalf("NewSession vazio = %v, quero ErrEmptyCatalog", err)
	}
}

func TestNewSessionAbreAPrimeiraCategoria(t *testing.T) {
	g, _ := newGame(t)
	s, err := g.NewSession(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.(*feramusicas.Session).Category(); got != "Pop" {
		t.Fatalf("categoria = %q, quero Pop (1ª em ordem alfabética)", got)
	}
	if g.ID() != "feramusicas" || g.Name() != "Fera Músicas" {
		t.Fatalf("ID/Name = %q/%q", g.ID(), g.Name())
	}
}

func TestVitoriaNaPrimeira(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Rock")
	if s.Answer() != nil {
		t.Fatal("a resposta não pode aparecer durante a partida")
	}
	ans := answerOf(t, g, "Rock")
	r, err := s.Guess(ctx, "  "+ans.Name+" ") // caixa e espaços não importam
	if err != nil || !r.Correct || r.State.Status != game.Won {
		t.Fatalf("Guess = %+v, %v", r, err)
	}
	if a := s.Answer(); a == nil || a.Name != ans.Name {
		t.Fatalf("Answer = %+v", a)
	}
	if got := s.Outcomes(); len(got) != 1 || got[0] != feramusicas.OutcomeCorrect {
		t.Fatalf("Outcomes = %v", got)
	}
	if _, err := s.Guess(ctx, ans.Name); !errors.Is(err, feramusicas.ErrFinished) {
		t.Fatalf("Guess após o fim = %v", err)
	}
	if _, err := s.Skip(ctx); !errors.Is(err, feramusicas.ErrFinished) {
		t.Fatalf("Skip após o fim = %v", err)
	}
}

func TestDerrotaEmCincoMisturandoErroEPulo(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Rock")
	wrong := wrongSong(t, g, answerOf(t, g, "Rock"))

	steps := []func(){
		func() { r, _ := s.GuessSong(wrong); mustPlaying(t, r) },
		func() { r, err := s.Skip(ctx); mustPlaying(t, r); noErr(t, err) },
		func() { r, _ := s.GuessSong(wrong); mustPlaying(t, r) },
		func() { r, _ := s.Skip(ctx); mustPlaying(t, r) },
	}
	for i, f := range steps {
		if got := s.State().Step; got != i+1 {
			t.Fatalf("Step = %d, quero %d", got, i+1)
		}
		if got := s.AudioStep(); got != i+1 {
			t.Fatalf("AudioStep = %d, quero %d", got, i+1)
		}
		f()
	}
	r, _ := s.Skip(ctx)
	if r.State.Status != game.Lost || r.Correct {
		t.Fatalf("5º erro deveria perder: %+v", r)
	}
	if s.Answer() == nil {
		t.Fatal("derrota revela a resposta")
	}
	want := []feramusicas.Outcome{"incorrect", "skipped", "incorrect", "skipped", "skipped"}
	for i, o := range s.Outcomes() {
		if o != want[i] {
			t.Fatalf("Outcomes = %v, quero %v", s.Outcomes(), want)
		}
	}
	st := s.State()
	if st.Step != 5 || st.Steps != 5 || st.Guesses[1] != "" || st.Guesses[0] != wrong.Name {
		t.Fatalf("State = %+v", st)
	}
	if s.AudioLimit() != 15*time.Second {
		t.Fatalf("depois do fim o trecho todo é liberado, limite = %v", s.AudioLimit())
	}
}

func mustPlaying(t *testing.T, r game.Result) {
	t.Helper()
	if r.State.Status != game.Playing || r.Correct {
		t.Fatalf("deveria seguir jogando: %+v", r)
	}
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAudioLimitCresceACadaRound(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Rock")
	want := []time.Duration{130 * time.Millisecond, 530 * time.Millisecond, 2 * time.Second, 8 * time.Second, 15 * time.Second}
	for i, w := range want {
		if s.AudioLimit() != w {
			t.Fatalf("round %d: limite = %v, quero %v", i+1, s.AudioLimit(), w)
		}
		if i < 4 {
			_, _ = s.Skip(ctx)
		}
	}
}

func TestChuteForaDoCatalogo(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Rock")
	if _, err := s.Guess(ctx, "Banda Inexistente - Nada"); !errors.Is(err, feramusicas.ErrUnknownSong) {
		t.Fatalf("Guess desconhecido = %v", err)
	}
	if len(s.Board()) != 0 {
		t.Fatal("chute inválido não conta como tentativa")
	}
}

func TestChuteDeOutraCategoriaConta(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Rock")
	// Billie Jean é Pop, mas está no catálogo: é um chute válido (e errado).
	r, err := s.Guess(ctx, "michael jackson - billie jean")
	if err != nil || r.Correct || len(s.Board()) != 1 {
		t.Fatalf("Guess = %+v, %v", r, err)
	}
}

func TestMesmoNomeEmOutroVideoAcerta(t *testing.T) {
	c := feramusicas.NewMemoryCatalog()
	a, _ := c.Add(ctx, feramusicas.Song{Name: "Titulo", Category: "X", URL: "https://youtu.be/aaaaaaaaaaa"})
	b, _ := c.Add(ctx, feramusicas.Song{Name: "titulo", Category: "Y", URL: "https://youtu.be/bbbbbbbbbbb"})
	g := feramusicas.NewGame(c, &feramusicastest.Source{})
	s := start(t, g, "X")
	r, _ := s.GuessSong(b)
	if !r.Correct {
		t.Fatalf("mesmo título (%q vs %q) deveria acertar", a.Name, b.Name)
	}
}

func TestNamesSaoDoCatalogoInteiro(t *testing.T) {
	g, _ := newGame(t)
	names := start(t, g, "Pop").Names()
	if len(names) != 3 {
		t.Fatalf("Names = %v, quero as 3 músicas do catálogo", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("Names fora de ordem: %v", names)
		}
	}
}

func TestSessaoCongelaAMusicaDoDia(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Pop")
	before := answerOf(t, g, "Pop")
	for _, u := range []string{"https://youtu.be/aaaaaaaaaaa", "https://youtu.be/bbbbbbbbbbb", "https://youtu.be/ccccccccccc"} {
		if _, err := g.Catalog().Add(ctx, feramusicas.Song{Name: u, Category: "Pop", URL: u}); err != nil {
			t.Fatal(err)
		}
	}
	r, _ := s.Guess(ctx, before.Name)
	if !r.Correct {
		t.Fatal("a sessão aberta deveria manter a resposta original")
	}
}

func TestAudioUsaAFonteComAMusicaDoDia(t *testing.T) {
	g, src := newGame(t)
	s := start(t, g, "Rock")
	rc, err := s.Audio(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	if len(data) != 3*44100*4 {
		t.Fatalf("bytes = %d", len(data))
	}
	if got := src.Fetched(); len(got) != 1 || got[0] != answerOf(t, g, "Rock").VideoID {
		t.Fatalf("Fetched = %v", got)
	}

	boom := errors.New("sem rede")
	src.Err = boom
	if _, err := s.Audio(ctx); !errors.Is(err, boom) {
		t.Fatalf("Audio = %v", err)
	}
}

func TestShareText(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Rock")
	ans := answerOf(t, g, "Rock")
	_, _ = s.GuessSong(wrongSong(t, g, ans))
	_, _ = s.Skip(ctx)
	_, _ = s.Guess(ctx, ans.Name)
	want := "Fera Músicas 2026-11-15 [Rock] 3/5\n🟥⬜🟩⬛⬛"
	if got := s.ShareText(); got != want {
		t.Fatalf("ShareText = %q, quero %q", got, want)
	}

	l := start(t, g, "Rock")
	for i := 0; i < 5; i++ {
		_, _ = l.Skip(ctx)
	}
	if want := "Fera Músicas 2026-11-15 [Rock] X/5\n⬜⬜⬜⬜⬜"; l.ShareText() != want {
		t.Fatalf("ShareText derrota = %q, quero %q", l.ShareText(), want)
	}
}

func TestCategoriaUsaAGrafiaDasAbas(t *testing.T) {
	c := feramusicas.NewMemoryCatalog()
	_, _ = c.Add(ctx, feramusicas.Song{Name: "A", Category: "Rock", URL: "https://youtu.be/aaaaaaaaaaa"})
	_, _ = c.Add(ctx, feramusicas.Song{Name: "B", Category: "rock", URL: "https://youtu.be/bbbbbbbbbbb"})
	g := feramusicas.NewGame(c, &feramusicastest.Source{})
	// Em vários dias o sorteio cai na música "rock"; a sessão tem de seguir a aba ("Rock").
	for d := 0; d < 10; d++ {
		s, err := g.Start(ctx, day.AddDate(0, 0, d), "  ROCK ")
		if err != nil {
			t.Fatal(err)
		}
		if s.Category() != "Rock" {
			t.Fatalf("dia +%d: categoria = %q, quero %q (grafia de Categories)", d, s.Category(), "Rock")
		}
		if want := "[Rock]"; !strings.Contains(s.ShareText(), want) {
			t.Fatalf("ShareText = %q, quero conter %q", s.ShareText(), want)
		}
	}
}

func TestMusicaCadastradaDepoisValeComoChute(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Pop")
	nova, err := g.Catalog().Add(ctx, feramusicas.Song{Name: "Daft Punk - One More Time", Category: "Eletrônica", URL: "https://youtu.be/FGBhQbmPwH8"})
	if err != nil {
		t.Fatal(err)
	}
	all, _ := g.Catalog().List(ctx)
	s.UseSongs(all)
	if !slices.Contains(s.Names(), nova.Name) {
		t.Fatalf("Names = %v, quero a música nova", s.Names())
	}
	if r, err := s.Guess(ctx, nova.Name); err != nil || r.Correct {
		t.Fatalf("chutar a música nova = %+v, %v; quero um chute válido (e errado)", r, err)
	}
}

func TestRespostaRemovidaContinuaChutavel(t *testing.T) {
	g, _ := newGame(t)
	s := start(t, g, "Pop")
	ans := answerOf(t, g, "Pop")
	if err := g.Catalog().Remove(ctx, ans.ID); err != nil {
		t.Fatal(err)
	}
	all, _ := g.Catalog().List(ctx)
	s.UseSongs(all)
	if !slices.Contains(s.Names(), ans.Name) {
		t.Fatalf("Names = %v: a resposta sumiu do autocomplete", s.Names())
	}
	if r, err := s.Guess(ctx, ans.Name); err != nil || !r.Correct {
		t.Fatalf("Guess(resposta removida) = %+v, %v; a partida não pode ficar impossível", r, err)
	}
	// E não duplica quando a resposta ainda está no catálogo.
	g2, _ := newGame(t)
	s2 := start(t, g2, "Pop")
	all2, _ := g2.Catalog().List(ctx)
	s2.UseSongs(all2)
	s2.UseSongs(all2)
	if len(s2.Names()) != 3 {
		t.Fatalf("Names = %v, quero 3 sem repetição", s2.Names())
	}
}

func catNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("Categoria %02d", i)
	}
	return out
}

func TestDailyCategoriesSorteiaTresEmOrdem(t *testing.T) {
	cats := catNames(8)
	got := feramusicas.DailyCategories(day, cats)
	if len(got) != feramusicas.DailyCount {
		t.Fatalf("DailyCategories = %v, quero %d", got, feramusicas.DailyCount)
	}
	if !slices.IsSorted(got) {
		t.Fatalf("as categorias do dia deveriam vir em ordem alfabética: %v", got)
	}
	// Mesma resposta independente da ordem e da grafia do catálogo.
	shuffled := append([]string(nil), cats...)
	slices.Reverse(shuffled)
	for i := range shuffled {
		shuffled[i] = "  " + strings.ToUpper(shuffled[i]) + " "
	}
	again := feramusicas.DailyCategories(day, shuffled)
	for i := range got {
		if !strings.EqualFold(again[i], got[i]) {
			t.Fatalf("ordem/caixa mudou o sorteio: %v vs %v", again, got)
		}
	}
}

func TestDailyCategoriesComPoucasCategoriasUsaTodas(t *testing.T) {
	if got := feramusicas.DailyCategories(day, nil); len(got) != 0 {
		t.Fatalf("sem categorias: %v", got)
	}
	got := feramusicas.DailyCategories(day, []string{"Rock", "pop", "ROCK"})
	if len(got) != 2 || got[0] != "pop" || got[1] != "Rock" {
		t.Fatalf("2 categorias (uma repetida com outra caixa): %v", got)
	}
}

func TestDailyCategoriesVariaEntreOsDias(t *testing.T) {
	cats := catNames(8)
	seen := map[string]bool{}
	for d := 0; d < 30; d++ {
		seen[strings.Join(feramusicas.DailyCategories(day.AddDate(0, 0, d), cats), ",")] = true
	}
	if len(seen) < 5 {
		t.Fatalf("30 dias produziram só %d conjuntos de categorias", len(seen))
	}
}

func TestDailyCategoriesMudaPoucoQuandoOCatalogoMuda(t *testing.T) {
	cats := catNames(8)
	before := feramusicas.DailyCategories(day, cats)
	// Remover uma categoria que não foi sorteada não muda o dia.
	var notPicked string
	for _, c := range cats {
		if !slices.Contains(before, c) {
			notPicked = c
			break
		}
	}
	rest := slices.DeleteFunc(append([]string(nil), cats...), func(c string) bool { return c == notPicked })
	if got := feramusicas.DailyCategories(day, rest); !slices.Equal(got, before) {
		t.Fatalf("remover %q (não sorteada) mudou o dia: %v → %v", notPicked, before, got)
	}
	// Cadastrar uma categoria nova troca no máximo uma das sorteadas.
	for i := 0; i < 20; i++ {
		after := feramusicas.DailyCategories(day, append(append([]string(nil), cats...), fmt.Sprintf("Nova %d", i)))
		kept := 0
		for _, c := range before {
			if slices.Contains(after, c) {
				kept++
			}
		}
		if kept < feramusicas.DailyCount-1 {
			t.Fatalf("uma categoria nova trocou %d das sorteadas: %v → %v", feramusicas.DailyCount-kept, before, after)
		}
	}
}

func TestDailyDoJogo(t *testing.T) {
	g, _ := newGame(t)
	got, err := g.Daily(ctx, day)
	if err != nil || len(got) != 2 {
		t.Fatalf("Daily = %v, %v", got, err)
	}
	empty := feramusicas.NewGame(feramusicas.NewMemoryCatalog(), nil)
	if _, err := empty.Daily(ctx, day); !errors.Is(err, feramusicas.ErrEmptyCatalog) {
		t.Fatalf("Daily vazio = %v", err)
	}
}

func TestShareDaily(t *testing.T) {
	got := feramusicas.ShareDaily("2026-11-15", 3, []feramusicas.DailyResult{
		{Category: "Pop", Won: false, Board: []feramusicas.Outcome{"incorrect", "skipped", "incorrect", "skipped", "skipped"}},
		{Category: "Rock", Won: true, Board: []feramusicas.Outcome{"correct"}},
	})
	want := "Fera Músicas 2026-11-15 1/3\n🟥⬜🟥⬜⬜ [Pop]\n🟩⬛⬛⬛⬛ [Rock]"
	if got != want {
		t.Fatalf("ShareDaily = %q, quero %q", got, want)
	}
}

// manyCategories: 6 categorias com 1 a 4 músicas cada.
func manyCategories(t *testing.T) *feramusicas.MemoryCatalog {
	t.Helper()
	c := feramusicas.NewMemoryCatalog()
	n := 0
	for ci, cat := range []string{"Axé", "Funk", "Pop", "Rock", "Samba", "MPB"} {
		for j := 0; j <= ci%4; j++ {
			_, err := c.Add(ctx, feramusicas.Song{
				Name: fmt.Sprintf("%s %d", cat, j), Category: cat, URL: fmt.Sprintf("https://youtu.be/%011d", n),
			})
			if err != nil {
				t.Fatal(err)
			}
			n++
		}
	}
	return c
}

func TestCalendarBateComOJogo(t *testing.T) {
	c := manyCategories(t)
	g := feramusicas.NewGame(c, nil)
	songs, _ := c.List(ctx)
	cal := feramusicas.Calendar(songs, day.AddDate(0, 0, -20), 40)
	if len(cal) != 40 {
		t.Fatalf("len = %d", len(cal))
	}
	for _, d := range cal {
		cats, err := g.Daily(ctx, d.Day)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Songs) != len(cats) || len(d.Songs) != feramusicas.DailyCount {
			t.Fatalf("%s: %d músicas, quero %d", feramusicas.FormatDate(d.Day), len(d.Songs), len(cats))
		}
		for i, cat := range cats {
			s, err := g.Start(ctx, d.Day, cat)
			if err != nil {
				t.Fatal(err)
			}
			// A sessão não expõe a resposta durante a partida; perder revela.
			for range feramusicas.MaxGuesses {
				_, _ = s.Skip(ctx)
			}
			if s.Answer().ID != d.Songs[i].ID {
				t.Fatalf("%s [%s]: calendário diz %q, o jogo sorteou %q",
					feramusicas.FormatDate(d.Day), cat, d.Songs[i].Name, s.Answer().Name)
			}
		}
	}
	if feramusicas.Calendar(nil, day, 3)[0].Songs != nil {
		t.Fatal("sem músicas, nenhum dia tem sorteio")
	}
}

// TestSorteioDaChanceIgualATodas: regressão do FNV sem mistura, com o qual,
// em 4 categorias, 1 em cada 4 músicas nunca era sorteada.
func TestSorteioDaChanceIgualATodas(t *testing.T) {
	for _, cats := range []int{1, 2, 3, 4, 5, 7} {
		var songs []feramusicas.Song
		for i := 0; i < 32; i++ {
			songs = append(songs, feramusicas.Song{ID: fmt.Sprint(i), Name: "x", VideoID: fmt.Sprintf("%011d", i),
				Category: fmt.Sprintf("Categoria muito comprida número %02d", i%cats)})
		}
		count := map[string]int{}
		for _, d := range feramusicas.Calendar(songs, day, 3650) {
			for _, s := range d.Songs {
				count[s.ID]++
			}
		}
		lo, hi := 1<<30, 0
		for _, s := range songs {
			lo, hi = min(lo, count[s.ID]), max(hi, count[s.ID])
		}
		if lo == 0 || float64(hi)/float64(lo) > 1.6 {
			t.Errorf("%d categorias: música menos sorteada %d vezes, mais sorteada %d em 10 anos", cats, lo, hi)
		}
	}
}

func TestPickMudaPoucoQuandoACategoriaMuda(t *testing.T) {
	songs := make([]feramusicas.Song, 10)
	for i := range songs {
		songs[i] = feramusicas.Song{ID: fmt.Sprint(i), VideoID: fmt.Sprintf("%011d", i)}
	}
	changed := 0
	for d := 0; d < 365; d++ {
		today := day.AddDate(0, 0, d)
		before, _ := feramusicas.Pick(today, "Rock", songs)
		// Remover uma que não foi sorteada nunca muda o dia.
		var rest []feramusicas.Song
		for _, s := range songs {
			if s.ID != before.ID && len(rest) < len(songs)-2 {
				rest = append(rest, s)
			}
		}
		rest = append(rest, before)
		if after, _ := feramusicas.Pick(today, "Rock", rest); after.ID != before.ID {
			t.Fatalf("dia +%d: remover músicas não sorteadas trocou %q por %q", d, before.ID, after.ID)
		}
		// Cadastrar uma nova troca o dia só quando ela vence (~1 em 11).
		withNew := append(append([]feramusicas.Song(nil), songs...), feramusicas.Song{ID: "nova", VideoID: "zzzzzzzzzzz"})
		if after, _ := feramusicas.Pick(today, "Rock", withNew); after.ID != before.ID {
			if after.ID != "nova" {
				t.Fatalf("dia +%d: cadastrar uma música trocou para outra antiga (%q)", d, after.ID)
			}
			changed++
		}
	}
	if changed == 0 || changed > 365/5 {
		t.Fatalf("a música nova virou a do dia em %d de 365 dias; esperado perto de 365/11", changed)
	}
}

func TestPreviewBaixaOTrechoComOInicio(t *testing.T) {
	src := &feramusicastest.Source{Seconds: 1}
	g := feramusicas.NewGame(feramusicas.NewMemoryCatalog(), src)
	rc, err := g.Preview(ctx, feramusicas.Song{VideoID: "fJ9rUzIMcZQ", Start: 90 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
	if got := src.FetchedSongs(); len(got) != 1 || got[0].VideoID != "fJ9rUzIMcZQ" || got[0].Start != 90*time.Second {
		t.Fatalf("Fetched = %+v", got)
	}
	if _, err := feramusicas.NewGame(feramusicas.NewMemoryCatalog(), nil).Preview(ctx, feramusicas.Song{}); !errors.Is(err, feramusicas.ErrNoSource) {
		t.Fatalf("sem fonte: %v", err)
	}
}

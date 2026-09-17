package bandle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"fera-games/internal/game"
)

// fakeProvider serve os payloads reais de testdata/ sem rede.
type fakeProvider struct {
	puzzle  Puzzle
	catalog Catalog
	stems   []int // estágios pedidos em Stem
}

func newFake(t *testing.T) *fakeProvider {
	t.Helper()
	var ps []Puzzle
	if err := json.Unmarshal(testdata(t, "planning-2026-09-16.json"), &ps); err != nil {
		t.Fatal(err)
	}
	var cat Catalog
	if err := json.Unmarshal(testdata(t, "guesslist-songs.json"), &cat); err != nil {
		t.Fatal(err)
	}
	return &fakeProvider{puzzle: ps[0], catalog: cat}
}

func (f *fakeProvider) DailyPuzzle(context.Context, time.Time) (Puzzle, error) { return f.puzzle, nil }
func (f *fakeProvider) SongCatalog(context.Context) (Catalog, error)           { return f.catalog, nil }
func (f *fakeProvider) Stem(_ context.Context, _ Puzzle, step int) (io.ReadCloser, error) {
	f.stems = append(f.stems, step)
	return io.NopCloser(bytes.NewReader([]byte("ID3"))), nil
}

func start(t *testing.T) (*Session, *fakeProvider) {
	t.Helper()
	f := newFake(t)
	day, _ := time.Parse("2006-01-02", f.puzzle.Day)
	s, err := NewGame(f).Start(context.Background(), day)
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

const (
	answer      = "Sam Smith - I'm Not the Only One"
	sameArtist  = "Sam Smith - Stay With Me"
	otherArtist = "Adele - Hello"
)

func TestWinOnThirdTryWithSkipAndRightArtist(t *testing.T) {
	s, f := start(t)
	ctx := context.Background()

	if st := s.State(); st.Step != 1 || st.Steps != 6 || st.Number != 1491 || st.Status != game.Playing {
		t.Fatalf("estado inicial: %+v", st)
	}
	if s.AudioStep() != 1 || len(s.UnlockedInstruments()) != 1 {
		t.Fatalf("início: áudio %d, instrumentos %v", s.AudioStep(), s.UnlockedInstruments())
	}
	if _, ok := s.Clue("en"); ok {
		t.Fatal("dica não pode aparecer no início")
	}

	r, err := s.Skip(ctx)
	if err != nil || r.Correct || r.State.Step != 2 {
		t.Fatalf("pulo: %+v %v", r, err)
	}
	r, err = s.Guess(ctx, sameArtist)
	if err != nil || r.Correct || r.Hint != "Artista certo!" || r.State.Step != 3 {
		t.Fatalf("artista certo: %+v %v", r, err)
	}
	if _, err := s.Stem(ctx); err != nil || f.stems[len(f.stems)-1] != 3 {
		t.Fatalf("áudio no passo 3: %v %v", f.stems, err)
	}
	r, err = s.Guess(ctx, strings.ToUpper(answer)) // caixa não importa
	if err != nil || !r.Correct || r.State.Status != game.Won || r.State.Step != 3 {
		t.Fatalf("acerto: %+v %v", r, err)
	}

	if _, err := s.Guess(ctx, otherArtist); !errors.Is(err, ErrFinished) {
		t.Fatalf("chute após o fim: %v", err)
	}
	if _, err := s.Skip(ctx); !errors.Is(err, ErrFinished) {
		t.Fatalf("pulo após o fim: %v", err)
	}
	if s.AudioStep() != 5 || len(s.UnlockedInstruments()) != 6 {
		t.Fatalf("fim: áudio %d, instrumentos %v", s.AudioStep(), s.UnlockedInstruments())
	}
	if c, ok := s.Clue("pt"); !ok || c != "I Am Not the Sole Individual" { // pt cai para en
		t.Fatalf("dica após o fim: %q %v", c, ok)
	}

	want := "Bandle #1491 3/6\n⬛🟨🟩\n#Bandle"
	if got := s.ShareText(); got != want {
		t.Fatalf("share:\n%s\nquero:\n%s", got, want)
	}
	want = "Bandle #1491 3/6\n⬛🟨🟩\nFound: 2/3 (66.7%)\nCurrent streak: 2 (max 5)\n#Bandle"
	if got := s.ShareTextWith(Stats{Played: 3, Won: 2, CurrentStreak: 2, MaxStreak: 5}); got != want {
		t.Fatalf("share com stats:\n%s\nquero:\n%s", got, want)
	}
}

func TestLoseAfterSixTries(t *testing.T) {
	s, _ := start(t)
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		r, err := s.Guess(ctx, otherArtist)
		if err != nil || r.State.Status != game.Playing || r.State.Step != i+1 {
			t.Fatalf("tentativa %d: %+v %v", i, r.State, err)
		}
	}
	if c, ok := s.Clue("en"); !ok || c == "" {
		t.Fatal("dica deveria aparecer na 6ª tentativa")
	}
	if s.AudioStep() != 5 {
		t.Fatalf("6ª tentativa deve reutilizar o áudio 5, veio %d", s.AudioStep())
	}
	r, err := s.Skip(ctx)
	if err != nil || r.State.Status != game.Lost || r.State.Step != 6 {
		t.Fatalf("derrota: %+v %v", r.State, err)
	}
	want := "Bandle #1491 x/6\n🟥🟥🟥🟥🟥⬛\n#Bandle"
	if got := s.ShareText(); got != want {
		t.Fatalf("share:\n%s\nquero:\n%s", got, want)
	}
	if len(r.State.Guesses) != 6 || r.State.Guesses[5] != "" || r.State.Guesses[0] != otherArtist {
		t.Fatalf("histórico: %q", r.State.Guesses)
	}
}

func TestGuessMustBeInCatalog(t *testing.T) {
	s, _ := start(t)
	_, err := s.Guess(context.Background(), "Nao Existe - Essa Musica")
	if !errors.Is(err, ErrUnknownSong) {
		t.Fatalf("erro = %v", err)
	}
	if st := s.State(); st.Step != 1 || len(st.Guesses) != 0 {
		t.Fatalf("chute inválido consumiu tentativa: %+v", st)
	}
	// A resposta entra no catálogo mesmo quando o guesslist não a traz.
	if _, ok := s.Catalog().Find(answer); !ok {
		t.Fatal("resposta não está no catálogo da sessão")
	}
}

func TestGameMetadata(t *testing.T) {
	g := NewGame(newFake(t))
	if g.ID() != "bandle" || g.Name() != "Bandle" {
		t.Fatalf("%s/%s", g.ID(), g.Name())
	}
	sess, err := g.NewSession(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sess.(*Session); !ok {
		t.Fatalf("NewSession devolveu %T", sess)
	}
}

func TestSnapshotRestore(t *testing.T) {
	s, f := start(t)
	ctx := context.Background()
	_, _ = s.Skip(ctx)
	_, _ = s.Guess(ctx, sameArtist)
	snap := s.Snapshot()
	if snap.Number != 1491 || snap.Outcomes != "by" || snap.Status != "playing" || snap.Tries() != 2 || snap.Finished() {
		t.Fatalf("snapshot: %+v", snap)
	}

	day, _ := time.Parse("2006-01-02", f.puzzle.Day)
	r := NewSession(f, f.puzzle, f.catalog, day)
	if err := r.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if got := r.Snapshot(); r.State().Step != 3 || !reflect.DeepEqual(got, snap) || got.Guesses[1] != sameArtist || r.AudioStep() != 3 {
		t.Fatalf("restaurado: %+v (step %d)", r.Snapshot(), r.State().Step)
	}
	// Continua a partida de onde parou.
	res, err := r.Guess(ctx, answer)
	if err != nil || !res.Correct {
		t.Fatalf("chute após restaurar: %+v %v", res, err)
	}
	if got := r.Snapshot(); !got.Won() || got.Outcomes != "byg" || got.Tries() != 3 {
		t.Fatalf("snapshot final: %+v", got)
	}

	// Snapshot de outro puzzle, ou adulterado, é rejeitado.
	bad := snap
	bad.Number = 7
	if err := NewSession(f, f.puzzle, f.catalog, day).Restore(bad); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("número errado: %v", err)
	}
	bad = snap
	bad.Outcomes = "gggggggg"
	bad.Guesses = make([]string, 8)
	if err := NewSession(f, f.puzzle, f.catalog, day).Restore(bad); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("mais tentativas que estágios: %v", err)
	}
	// Um "g" forjado num chute errado é rejulgado como erro, não vira vitória.
	forged := Snapshot{Number: 1491, Day: snap.Day, Guesses: []string{otherArtist}, Outcomes: "g", Status: "won"}
	r2 := NewSession(f, f.puzzle, f.catalog, day)
	if err := r2.Restore(forged); err != nil {
		t.Fatal(err)
	}
	if got := r2.Snapshot(); got.Outcomes != "r" || got.Status != "playing" {
		t.Fatalf("snapshot forjado não foi rejulgado: %+v", got)
	}
}

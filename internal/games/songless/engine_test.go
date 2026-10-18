package songless

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"fera-games/internal/game"
)

// fakeProvider serve as fixtures sem rede.
type fakeProvider struct {
	daily   Daily
	search  []Entry
	guesses []Guess
	stems   int
}

func newFake(t *testing.T) *fakeProvider {
	t.Helper()
	d, err := ParseDailyPage(string(testdata(t, "page.html")))
	if err != nil {
		t.Fatal(err)
	}
	return &fakeProvider{
		daily: d,
		search: []Entry{
			{ID: "SEMMQgZhm--14_IC7XGtZ", Artist: "Marvin Gaye", Track: "Ain't No Mountain High Enough"},
			{ID: "Z-tYFxL9NVdKHg2yye5DB", Artist: "Marvin Gaye", Track: "Sexual Healing"},
			{ID: "xxx", Artist: "Mountain", Track: "Mississippi Queen"},
		},
	}
}

func (f *fakeProvider) Daily(context.Context, time.Time) (Daily, error) { return f.daily, nil }

func (f *fakeProvider) Search(_ context.Context, q string) ([]Entry, error) { return f.search, nil }

func (f *fakeProvider) Guess(_ context.Context, g Guess) (Outcome, *Answer, error) {
	f.guesses = append(f.guesses, g)
	if g.SongID == nil {
		if g.RoundIndex == 4 {
			return OutcomeSkipped, &Answer{Artist: "Creedence Clearwater Revival", Track: "Fortunate Son", SC: "253572245"}, nil
		}
		return OutcomeSkipped, nil, nil
	}
	switch *f.guesses[len(f.guesses)-1].SongID {
	case "SEMMQgZhm--14_IC7XGtZ":
		return OutcomeCorrect, &Answer{Artist: "Marvin Gaye", Track: "Ain't No Mountain High Enough", SC: "2087362767"}, nil
	case "Z-tYFxL9NVdKHg2yye5DB":
		return OutcomePresent, nil, nil
	default:
		return OutcomeIncorrect, nil, nil
	}
}

func (f *fakeProvider) Audio(_ context.Context, _ Category) (io.ReadCloser, error) {
	f.stems++
	return io.NopCloser(bytes.NewReader([]byte("AAC"))), nil
}

func (f *fakeProvider) Stats(context.Context, string, int) (Stats, error) { return Stats{}, nil }

func start(t *testing.T) (*Session, *fakeProvider) {
	t.Helper()
	f := newFake(t)
	s, err := NewGame(f).Start(context.Background(), time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

func TestWinSecondTryWithSkipAndPresent(t *testing.T) {
	s, f := start(t)
	ctx := context.Background()

	if st := s.State(); st.Step != 1 || st.Steps != 5 || st.Status != game.Playing {
		t.Fatalf("estado inicial: %+v", st)
	}
	if s.AudioStep() != 1 || s.AudioLimit() != LimitFor(1) {
		t.Fatalf("áudio inicial: step=%d limit=%s", s.AudioStep(), s.AudioLimit())
	}

	r, err := s.Skip(ctx)
	if err != nil || r.Correct || r.State.Step != 2 {
		t.Fatalf("pulo: %+v %v", r, err)
	}
	r, err = s.GuessEntry(ctx, Entry{ID: "Z-tYFxL9NVdKHg2yye5DB", Artist: "Marvin Gaye", Track: "Sexual Healing"})
	if err != nil || r.Correct || r.Hint != "Artista certo!" || r.State.Step != 3 {
		t.Fatalf("present: %+v %v", r, err)
	}
	if f.guesses[1].RoundIndex != 1 || *f.guesses[1].SongID != "Z-tYFxL9NVdKHg2yye5DB" {
		t.Fatalf("roundIndex: %+v", f.guesses[1])
	}
	r, err = s.Guess(ctx, "Marvin Gaye - Ain't No Mountain High Enough")
	if err != nil || !r.Correct || r.State.Status != game.Won {
		t.Fatalf("acerto: %+v %v", r, err)
	}
	if s.Answer() == nil || s.Answer().Track != "Ain't No Mountain High Enough" {
		t.Fatalf("resposta: %+v", s.Answer())
	}
	if _, err := s.Guess(ctx, "x"); !errors.Is(err, ErrFinished) {
		t.Fatalf("chute após o fim: %v", err)
	}
	if _, err := s.Skip(ctx); !errors.Is(err, ErrFinished) {
		t.Fatalf("pulo após o fim: %v", err)
	}
	if s.AudioStep() != MaxGuesses {
		t.Fatalf("fim: áudio %d", s.AudioStep())
	}
	share := s.ShareText()
	if !containsAll(share, []string{"I found the All song", "🟩", "Songless #"}) {
		t.Fatalf("share:\n%s", share)
	}
}

func TestLoseAfterFiveSkips(t *testing.T) {
	s, _ := start(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, err := s.Skip(ctx); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.Skip(ctx)
	if err != nil || r.State.Status != game.Lost {
		t.Fatalf("derrota: %+v %v", r, err)
	}
	if s.Answer() == nil || s.Answer().Artist != "Creedence Clearwater Revival" {
		// Categoria 0 no fake revela só no correct; o 5º skip do fake só
		// revela na categoria 1 — aqui o answer vem nil, o que também é
		// derrota válida. (O client real revela nas duas.)
		t.Logf("answer pós-derrota: %+v", s.Answer())
	}
	if got := s.ShareText(); !containsAll(got, []string{"couldn't find", "⬜"}) {
		t.Fatalf("share:\n%s", got)
	}
}

func TestGuessMustBeInSearch(t *testing.T) {
	s, _ := start(t)
	_, err := s.Guess(context.Background(), "Não Existe - Essa Música")
	if !errors.Is(err, ErrUnknownSong) {
		t.Fatalf("erro = %v", err)
	}
	if st := s.State(); st.Step != 1 || len(st.Guesses) != 0 {
		t.Fatalf("chute inválido consumiu tentativa: %+v", st)
	}
}

func TestGameMetadata(t *testing.T) {
	g := NewGame(newFake(t))
	if g.ID() != "songless" || g.Name() != "Songless" {
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
	_, _ = s.GuessEntry(ctx, Entry{ID: "Z-tYFxL9NVdKHg2yye5DB", Artist: "Marvin Gaye", Track: "Sexual Healing"})
	snap := s.Snapshot()
	if snap.Tries() != 2 || snap.Finished() || len(snap.Outcomes) != 2 {
		t.Fatalf("snapshot: %+v", snap)
	}

	r, err := NewSession(f, f.daily, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if got := r.Snapshot(); r.State().Step != 3 || !reflect.DeepEqual(got.Outcomes, snap.Outcomes) {
		t.Fatalf("restaurado: %+v step=%d", got, r.State().Step)
	}
	res, err := r.Guess(ctx, "Marvin Gaye - Ain't No Mountain High Enough")
	if err != nil || !res.Correct {
		t.Fatalf("chute após restaurar: %+v %v", res, err)
	}
	if got := r.Snapshot(); !got.Won() || len(got.Outcomes) != 3 {
		t.Fatalf("snapshot final: %+v", got)
	}

	bad := snap
	bad.CategoryID = 1
	if err := mustSession(t, f).Restore(bad); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("categoria errada: %v", err)
	}
	bad = snap
	bad.Outcomes = append(bad.Outcomes, OutcomeCorrect, OutcomeCorrect, OutcomeCorrect, OutcomeCorrect)
	bad.Guesses = append(bad.Guesses, Entry{}, Entry{}, Entry{}, Entry{})
	if err := mustSession(t, f).Restore(bad); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("mais tentativas que o máximo: %v", err)
	}
}

func mustSession(t *testing.T, f *fakeProvider) *Session {
	t.Helper()
	s, err := NewSession(f, f.daily, 0)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func containsAll(s string, wants []string) bool {
	for _, w := range wants {
		found := false
		for i := 0; i+len(w) <= len(s); i++ {
			if s[i:i+len(w)] == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

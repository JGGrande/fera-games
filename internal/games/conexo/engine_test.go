package conexo

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"fera-games/internal/game"
	"fera-games/internal/games/conexo/fixture"
)

// fakeProvider serve a fixture sem rede.
type fakeProvider struct {
	puzzle Puzzle
	ids    []int
}

func (f *fakeProvider) Puzzle(_ context.Context, lang string, id int) (Puzzle, error) {
	f.ids = append(f.ids, id)
	if lang != "pt" || id != f.puzzle.ID {
		return Puzzle{}, ErrNoPuzzle
	}
	return f.puzzle, nil
}

var day1117 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local)

func newSession(t *testing.T) *Session {
	t.Helper()
	f := &fakeProvider{puzzle: mustParse(t, fixture.Doc1117JSON)}
	s, err := NewGame(f, "pt").Start(context.Background(), day1117)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.ids, []int{1117}) {
		t.Fatalf("pediu %v", f.ids)
	}
	return s
}

// pick seleciona as palavras e devolve o resultado da última.
func pick(t *testing.T, s *Session, words ...string) Outcome {
	t.Helper()
	var out Outcome
	for _, w := range words {
		var err error
		if out, err = s.Toggle(w); err != nil {
			t.Fatalf("Toggle(%q): %v", w, err)
		}
	}
	return out
}

var (
	pesca    = []string{"molinete", "vara", "linha", "anzol"}
	moldavel = []string{"maleável", "elástico", "mole", "flexível"}
	armadura = []string{"manopla", "espaldar", "braçal", "greva"}
	truco    = []string{"manilha", "zap", "vira", "vaza"}
	errado   = []string{"molinete", "vara", "linha", "maleável"}
)

func hint(t *testing.T, s *Session) []string {
	t.Helper()
	h, err := s.Hint()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestSimulatedMatch reproduz a partida do §4.6 do protocolo.
func TestSimulatedMatch(t *testing.T) {
	s := newSession(t)
	for range 2 {
		if out := pick(t, s, errado...); !out.Guessed || out.Group != 0 {
			t.Fatalf("erro julgado como %+v", out)
		}
		if len(s.Selected()) != 0 || len(s.Items()) != 16 {
			t.Fatal("erro deveria limpar a seleção e manter o tabuleiro")
		}
	}
	if s.Tries() != 2 {
		t.Fatalf("chute repetido deveria contar: %d", s.Tries())
	}
	if h := hint(t, s); !slices.Equal(h, []string{"molinete", "vara"}) || s.HintGroup() != 1 {
		t.Fatalf("1ª dica = %v (grupo %d)", h, s.HintGroup())
	}
	if out := pick(t, s, pesca...); out.Group != 1 {
		t.Fatalf("pesca = %+v", out)
	}
	if len(s.Hints()) != 0 {
		t.Fatal("achar o grupo da dica deveria zerar a dica")
	}
	want := []string{"manilha", "espaldar", "braçal", "flexível", "maleável", "vira", "greva", "elástico", "manopla", "mole", "vaza", "zap"}
	if !slices.Equal(s.Items(), want) {
		t.Fatalf("itens depois do acerto (ordem original): %v", s.Items())
	}
	hint(t, s)
	if h := hint(t, s); !slices.Equal(h, []string{"maleável", "elástico", "mole"}) {
		t.Fatalf("2ª dica do grupo 2 = %v", h)
	}
	if _, err := s.Hint(); !errors.Is(err, ErrNoHint) {
		t.Fatalf("3ª dica do mesmo grupo deveria ser bloqueada: %v", err)
	}
	pick(t, s, armadura...)
	if s.HintGroup() != 2 {
		t.Fatal("achar outro grupo deveria manter a dica")
	}
	pick(t, s, moldavel...)
	if out := pick(t, s, truco...); out.Group != 4 || s.State().Status != game.Won {
		t.Fatalf("vitória: %+v %v", out, s.State().Status)
	}
	if s.Tries() != 6 || s.HintsUsed() != 3 {
		t.Fatalf("tentativas %d, dicas %d", s.Tries(), s.HintsUsed())
	}
	if got := s.ResultLine(); got != "Você acertou em 6 tentativas e 3 dicas." {
		t.Fatalf("ResultLine = %q", got)
	}
	if got := s.ShareText(); got != "Joguei conexo.ws 22/10/2026 e consegui em 6 tentativas e 3 dicas.\n\n❌❌💡🟧💡💡🟦🟩🟪" {
		t.Fatalf("ShareText = %q", got)
	}
	var order []int
	for _, g := range s.Found() {
		order = append(order, g.Number)
	}
	if !slices.Equal(order, []int{1, 3, 2, 4}) {
		t.Fatalf("Found = %v", order)
	}
	if _, err := s.Toggle("zap"); !errors.Is(err, ErrFinished) {
		t.Fatalf("depois da vitória: %v", err)
	}
}

func TestToggleDeselectsAndRejectsUnknown(t *testing.T) {
	s := newSession(t)
	pick(t, s, "zap", "vira")
	pick(t, s, "zap")
	if !slices.Equal(s.Selected(), []string{"vira"}) {
		t.Fatalf("seleção = %v", s.Selected())
	}
	if _, err := s.Toggle("xyz"); !errors.Is(err, ErrUnknownWord) {
		t.Fatalf("palavra de fora: %v", err)
	}
	pick(t, s, "vira") // desseleciona
	pick(t, s, pesca...)
	if _, err := s.Toggle("anzol"); !errors.Is(err, ErrUnknownWord) {
		t.Fatalf("palavra já achada não deveria ser selecionável: %v", err)
	}
}

func TestHintBlockedAfterThreeGroups(t *testing.T) {
	s := newSession(t)
	pick(t, s, pesca...)
	pick(t, s, moldavel...)
	if h := hint(t, s); !slices.Equal(h, []string{"manopla", "espaldar"}) {
		t.Fatalf("dica com 1 e 2 achados = %v", h)
	}
	pick(t, s, truco...)
	if _, err := s.Hint(); !errors.Is(err, ErrNoHint) {
		t.Fatalf("com 3 grupos achados não há dica: %v", err)
	}
}

func TestSwap(t *testing.T) {
	s := newSession(t)
	if err := s.Swap(0, 15); err != nil {
		t.Fatal(err)
	}
	if it := s.Items(); it[0] != "zap" || it[15] != "manilha" {
		t.Fatalf("troca: %v", it)
	}
	if err := s.Swap(0, 16); err == nil {
		t.Fatal("troca fora do tabuleiro deveria falhar")
	}
	if s.Tries() != 0 {
		t.Fatal("troca não é tentativa")
	}
}

func TestGameSessionAdapter(t *testing.T) {
	s := newSession(t)
	pick(t, s, "zap")
	r, err := s.Guess(context.Background(), "molinete | vara|linha|anzol")
	if err != nil || !r.Correct || r.Hint != "Equipamentos de pesca" {
		t.Fatalf("Guess = %+v, %v", r, err)
	}
	if len(s.Selected()) != 0 {
		t.Fatal("Guess deveria descartar a seleção")
	}
	if _, err := s.Guess(context.Background(), "zap|zap|vira|vaza"); !errors.Is(err, ErrUnknownWord) {
		t.Fatalf("palavra repetida no chute: %v", err)
	}
	r, err = s.Skip(context.Background())
	if err != nil || r.Hint != "maleável, elástico" {
		t.Fatalf("Skip = %+v, %v", r, err)
	}
	if st := s.State(); st.Number != 1117 || st.Steps != 0 || st.Step != 2 || st.Guesses[0] != "molinete|vara|linha|anzol" {
		t.Fatalf("State = %+v", st)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := newSession(t)
	pick(t, s, errado...)
	hint(t, s)
	pick(t, s, pesca...)
	hint(t, s)
	if err := s.Swap(0, 1); err != nil {
		t.Fatal(err)
	}
	pick(t, s, "zap") // seleção não é salva

	data, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Day != "2026-10-22" || snap.Tries() != 2 || snap.Finished() {
		t.Fatalf("snapshot: %+v", snap)
	}

	r := newSession(t)
	if err := r.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Items(), s.Items()) || !slices.Equal(r.Hints(), s.Hints()) || r.HintGroup() != 2 {
		t.Fatalf("restaurado: items %v hints %v", r.Items(), r.Hints())
	}
	if len(r.Found()) != 1 || r.Tries() != 2 || r.HintsUsed() != 2 || len(r.Selected()) != 0 {
		t.Fatalf("restaurado: %+v", r.Snapshot())
	}
}

func TestRestoreRejectsTampering(t *testing.T) {
	s := newSession(t)
	pick(t, s, errado...)
	pick(t, s, pesca...)
	good := s.Snapshot()

	two := 2
	cases := map[string]func(*Snapshot){
		"outro puzzle":      func(sn *Snapshot) { sn.Number = 1116 },
		"outra versão":      func(sn *Snapshot) { sn.Version = &two },
		"erro virou acerto": func(sn *Snapshot) { sn.Attempts[0].Group = 1 },
		"dica inventada":    func(sn *Snapshot) { sn.Attempts = append(sn.Attempts, Attempt{Hints: []string{"zap", "vira"}}) },
		"chute com 3":       func(sn *Snapshot) { sn.Attempts[1].Selected = pesca[:3] },
		"itens trocados":    func(sn *Snapshot) { sn.Items[0] = "anzol" },
		"item faltando":     func(sn *Snapshot) { sn.Items = sn.Items[1:] },
		"chute depois do fim": func(sn *Snapshot) {
			for _, g := range [][]string{moldavel, armadura, truco, truco} {
				sn.Attempts = append(sn.Attempts, Attempt{Selected: g, Group: judge(s.puzzle, g)})
			}
			sn.Items = nil
		},
	}
	for name, mutate := range cases {
		data, _ := json.Marshal(good)
		var snap Snapshot
		_ = json.Unmarshal(data, &snap)
		mutate(&snap)
		r := newSession(t)
		if err := r.Restore(snap); !errors.Is(err, ErrSnapshotMismatch) {
			t.Errorf("%s: Restore = %v", name, err)
		}
		if r.Tries() != 0 || len(r.Items()) != 16 {
			t.Errorf("%s: sessão alterada por um restore recusado", name)
		}
	}
}

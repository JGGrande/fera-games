package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "sub")} // Dir ainda não existe
	if _, ok, err := s.Load("bandle", "2026-09-16"); ok || err != nil {
		t.Fatalf("load vazio: ok=%v err=%v", ok, err)
	}
	m := Match{Day: "2026-09-16", Number: 1491, Status: "playing", Tries: 2, Data: json.RawMessage(`{"x":1}`)}
	if err := s.Save("bandle", m); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Load("bandle", "2026-09-16")
	var data struct{ X int }
	_ = json.Unmarshal(got.Data, &data) // MarshalIndent reindenta o RawMessage
	if err != nil || !ok || got.Number != 1491 || data.X != 1 || got.SavedAt.IsZero() {
		t.Fatalf("load: %+v ok=%v err=%v", got, ok, err)
	}
	// Salvar de novo substitui.
	m.Status, m.Finished, m.Tries = "won", true, 3
	if err := s.Save("bandle", m); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.Load("bandle", "2026-09-16")
	if got.Status != "won" || got.Tries != 3 {
		t.Fatalf("não substituiu: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "bandle.json")); err != nil {
		t.Fatal(err)
	}
}

func TestNilOrEmptyStoreIsNoop(t *testing.T) {
	var s *Store
	if err := s.Save("bandle", Match{Day: "d"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Load("bandle", "d"); ok || err != nil {
		t.Fatal("store nil devolveu dado")
	}
	if st, err := (&Store{}).Stats("bandle", "2026-09-16"); err != nil || st != (Stats{}) {
		t.Fatalf("stats vazio: %+v %v", st, err)
	}
}

func TestCorruptFile(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(s.Dir, "bandle.json"), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Load("bandle", "x"); err == nil {
		t.Fatal("arquivo corrompido deveria dar erro")
	}
}

func TestStatsAndStreaks(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	save := func(day, status string, finished bool) {
		if err := s.Save("bandle", Match{Day: day, Status: status, Finished: finished}); err != nil {
			t.Fatal(err)
		}
	}
	save("2026-09-10", "won", true)
	save("2026-09-11", "won", true)
	save("2026-09-12", "lost", true)
	save("2026-09-14", "won", true) // 13 não jogou
	save("2026-09-15", "won", true)
	save("2026-09-16", "won", true)
	save("2026-09-17", "playing", false) // hoje, em andamento: não conta

	st, err := s.Stats("bandle", "2026-09-17")
	if err != nil {
		t.Fatal(err)
	}
	want := Stats{Played: 6, Won: 5, CurrentStreak: 3, MaxStreak: 3}
	if st != want {
		t.Fatalf("stats = %+v, quero %+v", st, want)
	}

	// Dois dias depois sem jogar, a sequência atual zera; a máxima fica.
	st, _ = s.Stats("bandle", "2026-09-19")
	if st.CurrentStreak != 0 || st.MaxStreak != 3 {
		t.Fatalf("sequência quebrada: %+v", st)
	}

	// Vitória hoje estende a sequência.
	save("2026-09-17", "won", true)
	st, _ = s.Stats("bandle", "2026-09-17")
	if st.CurrentStreak != 4 || st.MaxStreak != 4 || st.Played != 7 {
		t.Fatalf("vitória de hoje: %+v", st)
	}
}

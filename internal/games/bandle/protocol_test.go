package bandle

import (
	"encoding/json"
	"testing"
	"time"

	"fera-games/internal/games/bandle/fixture"
)

// testdata serve as respostas de fixture pelo nome que o servidor usa.
func testdata(t *testing.T, name string) []byte {
	t.Helper()
	switch name {
	case "planning-2026-09-16.json":
		return []byte(fixture.PlanningJSON)
	case "planning-2026-09-16.txt":
		return []byte(encrypt(EncryptionKey(), []byte(fixture.PlanningJSON)))
	case "guesslist-songs.json":
		return []byte(fixture.CatalogJSON)
	}
	t.Fatalf("fixture desconhecida: %s", name)
	return nil
}

func TestEncryptionKey(t *testing.T) {
	// Valor obtido rodando getEncryptionKey() do bundle da SPA.
	if got, want := EncryptionKey(), "804204da-e727-47f6-ae12-040c46e814d7"; got != want {
		t.Fatalf("EncryptionKey() = %q, quero %q", got, want)
	}
}

func TestDecryptPlanning(t *testing.T) {
	raw := testdata(t, "planning-2026-09-16.txt")
	plain, err := Decrypt(EncryptionKey(), string(raw))
	if err != nil {
		t.Fatal(err)
	}
	var got []Puzzle
	if err := json.Unmarshal(plain, &got); err != nil {
		t.Fatalf("payload decifrado não é JSON: %v\n%s", err, plain)
	}
	var want []Puzzle
	if err := json.Unmarshal(testdata(t, "planning-2026-09-16.json"), &want); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != want[0].ID || got[0].Song != want[0].Song {
		t.Fatalf("decifrado = %+v, quero %+v", got, want)
	}
}

func TestPuzzleMatchesFormulaAndPaths(t *testing.T) {
	var ps []Puzzle
	if err := json.Unmarshal(testdata(t, "planning-2026-09-16.json"), &ps); err != nil {
		t.Fatal(err)
	}
	p := ps[0]
	day, _ := time.Parse("2006-01-02", p.Day)
	if n := PuzzleNumber(day); n != p.ID {
		t.Errorf("PuzzleNumber(%s) = %d, payload diz id=%d", p.Day, n, p.ID)
	}
	if got := PlanningFile(day); got != "/v2/planning/2026-09-16.txt" {
		t.Errorf("PlanningFile = %q", got)
	}
	if p.Stages() != 6 || p.AudioStages() != 5 || p.Instruments[5] != "clue" {
		t.Errorf("estágios: %v", p.Instruments)
	}
	if got := StemFile(p, 1); got != "/v2/files/"+p.Path+"/1.mp3" {
		t.Errorf("StemFile = %q", got)
	}
	if p.Clue["en"] == "" || p.Year == 0 || p.Par == 0 {
		t.Errorf("campos de dica vazios: %+v", p)
	}
}

func TestCatalog(t *testing.T) {
	var cat Catalog
	if err := json.Unmarshal(testdata(t, "guesslist-songs.json"), &cat); err != nil {
		t.Fatal(err)
	}
	if len(cat) == 0 {
		t.Fatal("catálogo vazio")
	}
	var ps []Puzzle
	_ = json.Unmarshal(testdata(t, "planning-2026-09-16.json"), &ps)
	// Fato observado em 2026-09-16: a resposta do dia não vinha no guesslist,
	// por isso MergeAnswer existe.
	merged := MergeAnswer(cat, ps[0])
	at := len(cat) // MergeAnswer insere em min(1000, len)
	if len(merged) != len(cat)+1 || merged[at].Title != ps[0].Song {
		t.Fatalf("MergeAnswer não inseriu a resposta na posição %d (len %d → %d)", at, len(cat), len(merged))
	}
	if again := MergeAnswer(merged, ps[0]); len(again) != len(merged) {
		t.Errorf("MergeAnswer duplicou a resposta")
	}
	if Judge(ps[0], &merged[at]) != OutcomeCorrect {
		t.Errorf("a entrada inserida não é julgada como acerto")
	}
}

func TestJudge(t *testing.T) {
	p := Puzzle{Song: "Sam Smith - I'm Not the Only One", Sources: []string{"Sam Smith"}}
	cases := []struct {
		name  string
		guess *CatalogEntry
		want  Outcome
	}{
		{"pulo", nil, OutcomeSkipped},
		{"acerto", &CatalogEntry{Title: "sam smith - i'm not the only one ", Sources: []string{"Sam Smith"}}, OutcomeCorrect},
		{"artista certo", &CatalogEntry{Title: "Sam Smith - Stay With Me", Sources: []string{"sam smith"}}, OutcomeRightArtist},
		{"errado", &CatalogEntry{Title: "Adele - Hello", Sources: []string{"Adele"}}, OutcomeWrong},
	}
	for _, c := range cases {
		if got := Judge(p, c.guess); got != c.want {
			t.Errorf("%s: Judge = %c, quero %c", c.name, got, c.want)
		}
	}
	if line := OutcomeCorrect.Emoji() + OutcomeSkipped.Emoji(); line != "🟩⬛" {
		t.Errorf("emoji = %q", line)
	}
}

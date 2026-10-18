package songless

import (
	"strings"
	"testing"
	"time"

	"fera-games/internal/games/songless/fixture"
)

// testdata serve as respostas de fixture pelo nome do arquivo que o servidor
// devolveria.
func testdata(t *testing.T, name string) []byte {
	t.Helper()
	m := map[string]string{
		"page.html":                   fixture.PageHTML,
		"search-marvin.json":          fixture.SearchMarvinJSON,
		"guess-correct.json":          fixture.GuessCorrectJSON,
		"guess-present.json":          fixture.GuessPresentJSON,
		"guess-incorrect.json":        fixture.GuessIncorrectJSON,
		"guess-skip.json":             fixture.GuessSkipJSON,
		"guess-final-skip.json":       fixture.GuessFinalSkipJSON,
		"guess-date-not-allowed.json": fixture.GuessDateNotAllowedJSON,
		"stats-2026-09-27-0.json":     fixture.StatsJSON,
	}
	if v, ok := m[name]; ok {
		return []byte(v)
	}
	if name == "A101.aac" || name == "A101-short.aac" {
		return fixture.ShortAAC()
	}
	t.Fatalf("fixture desconhecida: %s", name)
	return nil
}

func TestParseDailyPage(t *testing.T) {
	d, err := ParseDailyPage(string(testdata(t, "page.html")))
	if err != nil {
		t.Fatal(err)
	}
	if d.Date != "2026-09-27" {
		t.Fatalf("date = %q", d.Date)
	}
	if len(d.Categories) != 3 {
		t.Fatalf("categorias = %+v", d.Categories)
	}
	want := map[int]string{0: "https://cdn.lessgames.com/songs/A101.aac", 1: "https://cdn.lessgames.com/songs/B137.aac", 2: "https://cdn.lessgames.com/songs/C040.aac"}
	for _, c := range d.Categories {
		if want[c.ID] != c.FileURL {
			t.Errorf("cat %d: fileUrl=%q", c.ID, c.FileURL)
		}
		if CategoryNames[c.ID] != c.Name {
			t.Errorf("cat %d: name=%q", c.ID, c.Name)
		}
	}
	if _, err := ParseDailyPage("<html>sem nada</html>"); err == nil {
		t.Fatal("HTML sem flight-data deveria falhar")
	}
}

func TestChallengeNumber(t *testing.T) {
	n, err := ChallengeNumber("2025-08-29")
	if err != nil || n != 1 {
		t.Fatalf("início: n=%d err=%v", n, err)
	}
	n, err = ChallengeNumber("2026-09-27")
	if err != nil {
		t.Fatal(err)
	}
	// 2025-08-29 → 2026-09-27 = 394 dias +1 = 395.
	start, _ := time.Parse("2006-01-02", "2025-08-29")
	end, _ := time.Parse("2006-01-02", "2026-09-27")
	want := int(end.Sub(start).Hours()/24) + 1
	if n != want {
		t.Fatalf("n=%d quero %d", n, want)
	}
	if _, err := ChallengeNumber("não-data"); err == nil {
		t.Fatal("data inválida deveria falhar")
	}
}

func TestTimestepsAndLimit(t *testing.T) {
	if len(Timesteps) != MaxGuesses || Timesteps[0] != 0.13 || Timesteps[4] != 15 {
		t.Fatalf("timesteps = %v", Timesteps)
	}
	if LimitFor(1) != 130*time.Millisecond {
		t.Fatalf("limite 1 = %s", LimitFor(1))
	}
	if LimitFor(5) != 15*time.Second {
		t.Fatalf("limite 5 = %s", LimitFor(5))
	}
}

func TestOutcomeEmoji(t *testing.T) {
	cases := map[Outcome]string{
		OutcomeCorrect: "🟩", OutcomePresent: "🟨",
		OutcomeIncorrect: "🟥", OutcomeSkipped: "⬜", OutcomeEmpty: "⬛",
	}
	for o, want := range cases {
		if got := o.Emoji(); got != want {
			t.Errorf("%s: emoji=%q quero %q", o, got, want)
		}
	}
}

func TestShareText(t *testing.T) {
	// Vitória na 1ª em All (#395 para 2026-09-27): tempo 0.1s, emoji 🤯.
	got := ShareText("2026-09-27", "All", true, 1, []Outcome{OutcomeCorrect})
	n, _ := ChallengeNumber("2026-09-27")
	if !strings.Contains(got, "[Songless #") || !strings.Contains(got, "I found the All song") {
		t.Fatalf("share:\n%s", got)
	}
	if !strings.Contains(got, "🟩⬛⬛⬛⬛") || !strings.Contains(got, "🤯") {
		t.Fatalf("emoji/tempo:\n%s", got)
	}
	if !strings.Contains(got, "https://less.gg/songless") {
		t.Fatalf("invite:\n%s", got)
	}
	_ = n
	// Derrota com 5 skips: ⬜×5 + 💀.
	got = ShareText("2026-09-27", "Rock", false, 5,
		[]Outcome{OutcomeSkipped, OutcomeSkipped, OutcomeSkipped, OutcomeSkipped, OutcomeSkipped})
	if !strings.Contains(got, "couldn't find the Rock song") || !strings.Contains(got, "⬜⬜⬜⬜⬜") {
		t.Fatalf("derrota:\n%s", got)
	}
	multi := ShareMulti("2026-09-27", []struct {
		Category string
		Board    []Outcome
	}{{"All", []Outcome{OutcomeCorrect}}, {"Rock", []Outcome{OutcomeSkipped}}})
	if !strings.Contains(multi, "Songless #") || !strings.Contains(multi, "[All]") || !strings.Contains(multi, "[Rock]") {
		t.Fatalf("multi:\n%s", multi)
	}
}

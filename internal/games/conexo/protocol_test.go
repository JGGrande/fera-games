package conexo

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"fera-games/internal/games/conexo/fixture"
)

func mustParse(t *testing.T, doc string) Puzzle {
	t.Helper()
	p, err := ParseDocument([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseDocument(t *testing.T) {
	p := mustParse(t, fixture.Doc1117JSON)
	if p.ID != 1117 || p.Version != nil || p.Corrections != nil {
		t.Fatalf("id/version/corrections: %+v", p)
	}
	want := []Group{
		{1, "Equipamentos de pesca", [4]string{"molinete", "vara", "linha", "anzol"}},
		{2, "Moldável", [4]string{"maleável", "elástico", "mole", "flexível"}},
		{3, "Componentes de uma armadura medieval", [4]string{"manopla", "espaldar", "braçal", "greva"}},
		{4, "Truco", [4]string{"manilha", "zap", "vira", "vaza"}},
	}
	if !slices.Equal(p.Groups[:], want) {
		t.Fatalf("grupos:\n%+v", p.Groups)
	}
	if p.StartingBoard[0] != "manilha" || p.StartingBoard[15] != "zap" {
		t.Fatalf("tabuleiro: %v", p.StartingBoard)
	}
	if p.Credits != (Credits{"Anna Duzzi", "Elysa Marques"}) {
		t.Fatalf("créditos: %+v", p.Credits)
	}
	if p.GroupOf("zap") != 4 || p.GroupOf("xyz") != 0 {
		t.Fatal("GroupOf")
	}
}

func TestParseDocumentCorrectionsAndVersion(t *testing.T) {
	p := mustParse(t, fixture.Doc1117Corrected)
	if p.Version == nil || *p.Version != 2 {
		t.Fatalf("version = %v", p.Version)
	}
	if p.Display("vaza") != "vasa" || p.Display("Truco") != "Truco!" || p.Display("zap") != "zap" {
		t.Fatalf("Display: %v", p.Corrections)
	}
	// A correção é só de exibição: o julgamento usa o original.
	if p.GroupOf("vaza") != 4 {
		t.Fatal("palavra corrigida perdeu o grupo")
	}
}

func TestParseDocumentRejectsBadShape(t *testing.T) {
	cases := map[string]string{
		"tabuleiro com palavra repetida": strings.Replace(fixture.Doc1117JSON,
			`{"stringValue": "zap"}
    ]}}`, `{"stringValue": "vaza"}
    ]}}`, 1),
		"grupo com 3 palavras": strings.Replace(fixture.Doc1117JSON,
			`, {"stringValue": "anzol"}]`, `]`, 1),
		"number repetido": strings.Replace(fixture.Doc1117JSON,
			`"number": {"integerValue": "2"}`, `"number": {"integerValue": "1"}`, 1),
		"integerValue inválido": strings.Replace(fixture.Doc1117JSON,
			`"id": {"integerValue": "1117"}`, `"id": {"integerValue": "abc"}`, 1),
		"sem fields": `{"name": "x"}`,
	}
	for name, doc := range cases {
		if doc == fixture.Doc1117JSON {
			t.Fatalf("%s: o Replace não achou o trecho", name)
		}
		if _, err := ParseDocument([]byte(doc)); err == nil {
			t.Errorf("%s: deveria falhar", name)
		}
	}
}

func TestNumber(t *testing.T) {
	day := time.Date(2026, 10, 22, 23, 30, 0, 0, time.Local)
	for lang, want := range map[string]int{"pt": 1117, "en": 994, "es": 873} {
		if got, err := Number(lang, day); err != nil || got != want {
			t.Errorf("Number(%s) = %d, %v; quer %d", lang, got, err, want)
		}
	}
	if n, err := Number("pt", time.Date(2023, 10, 1, 8, 0, 0, 0, time.Local)); err != nil || n != 0 {
		t.Errorf("data de início = %d, %v", n, err)
	}
	if _, err := Number("pt", time.Date(2023, 9, 30, 8, 0, 0, 0, time.Local)); err == nil {
		t.Error("antes do início deveria falhar")
	}
	if _, err := Number("fr", day); err == nil {
		t.Error("idioma desconhecido deveria falhar")
	}
}

func TestParseToken(t *testing.T) {
	exp := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	got, err := ParseToken(fixture.Token(exp))
	if err != nil || !got.Equal(exp) {
		t.Fatalf("ParseToken = %v, %v", got, err)
	}
	for _, bad := range []string{"", "abc", "a.b", "a.!!!.c", "a." + "eyJmb28iOjF9" + ".c"} {
		if _, err := ParseToken(bad); !errors.Is(err, ErrBadToken) {
			t.Errorf("ParseToken(%q) = %v", bad, err)
		}
	}
}

func TestExtractToken(t *testing.T) {
	jwt := fixture.Token(time.Now().Add(time.Hour))
	for _, raw := range []string{
		jwt, "  " + jwt + "\n", `"` + jwt + `"`,
		`{"ttl":"604800s","token":"` + jwt + `"}`,
		// Registro do IndexedDB como o DevTools copia (objeto JS, não JSON).
		`{compositeKey: '1:689008611670:web:45c4d4dbce26e56deb46f7-[DEFAULT]', value: {token: '` + jwt +
			`', expireTimeMillis: 1793275200000, issuedAtTimeMillis: 1792670400000}}`,
		`token: "` + jwt + `"`,
	} {
		if got := extractToken(raw); got != jwt {
			t.Errorf("extractToken(%q) = %q", raw, got)
		}
	}
}

func TestEmojisBreakEveryTen(t *testing.T) {
	var attempts []Attempt
	for range 12 {
		attempts = append(attempts, Attempt{Selected: []string{"a", "b", "c", "d"}})
	}
	attempts[10] = Attempt{Hints: []string{"x", "y"}}
	attempts[11] = Attempt{Selected: []string{"a", "b", "c", "d"}, Group: 3}
	want := strings.Repeat("❌", 10) + "\n💡🟦"
	if got := Emojis(attempts); got != want {
		t.Fatalf("Emojis = %q", got)
	}
}

func TestShareTextLanguages(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	a := []Attempt{{Selected: []string{"a"}, Group: 1}, {Hints: []string{"x"}}}
	cases := map[string]string{
		"pt": "Joguei conexo.ws 22/10/2026 e consegui em 4 tentativas e 1 dica.\n\n🟧💡",
		"en": "I played conexo.ws 10/22/2026 and got it in 4 guesses and 1 hint.\n\n🟧💡",
		"es": "Jugué conexo.ws 22/10/2026 y lo conseguí en 4 intentos y 1 pista.\n\n🟧💡",
	}
	for lang, want := range cases {
		if got := ShareText(lang, day, 4, 1, a); got != want {
			t.Errorf("%s:\n%q\nquer\n%q", lang, got, want)
		}
	}
	if got := ShareText("pt", day, 5, 0, nil); got != "Joguei conexo.ws 22/10/2026 e consegui em 5 tentativas.\n\n" {
		t.Errorf("sem dicas: %q", got)
	}
	if got := ResultLine(6, 3); got != "Você acertou em 6 tentativas e 3 dicas." {
		t.Errorf("ResultLine = %q", got)
	}
}

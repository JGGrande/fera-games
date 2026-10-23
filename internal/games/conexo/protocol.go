// Package conexo contém a engine e o provider do Conexo (https://conexo.ws).
package conexo

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Contrato mapeado na Fase 0 (docs/conexo-protocol.md).
//
//	Puzzle: GET <DocumentsURL>/conexo-daily-<lang>/<id>
//	        X-Firebase-AppCheck: <JWT do App Check, 7 dias>
//	        → documento tipado do Firestore; 403 = token ruim; 404 = sem puzzle
//
// A resposta vem no payload e a validação é toda local: não há endpoint de chute.
const (
	// DocumentsURL é o prefixo REST dos documentos do projeto daydashgames.
	DocumentsURL = "https://firestore.googleapis.com/v1/projects/daydashgames/databases/(default)/documents"
	// GroupSize é o tamanho de cada grupo (e de cada chute).
	GroupSize = 4
	// Groups é o número de grupos por puzzle.
	Groups = 4
	// emojisPerLine é a quebra da grade de emojis do resultado.
	emojisPerLine = 10
)

// StartDates é o dia do puzzle 0 em cada idioma (config He do bundle).
var StartDates = map[string]time.Time{
	"pt": time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
	"en": time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
	"es": time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
}

// Collection é a coleção do Firestore com os puzzles do idioma.
func Collection(lang string) string { return "conexo-daily-" + lang }

// FormatDate é a chave do dia no store: YYYY-MM-DD.
func FormatDate(t time.Time) string { return t.Format("2006-01-02") }

// DisplayDate é a data como o site mostra e compartilha.
func DisplayDate(lang string, t time.Time) string {
	if lang == "en" {
		return t.Format("01/02/2006")
	}
	return t.Format("02/01/2006")
}

// Number é o id do puzzle do dia civil de day (no fuso de day): dias desde
// StartDates[lang]. As datas viram meia-noite UTC para o horário de verão
// não tirar uma hora da conta.
func Number(lang string, day time.Time) (int, error) {
	start, ok := StartDates[lang]
	if !ok {
		return 0, fmt.Errorf("conexo: idioma %q desconhecido", lang)
	}
	y, m, d := day.Date()
	n := int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Sub(start).Hours() / 24)
	if n < 0 {
		return 0, fmt.Errorf("conexo: %s é antes do primeiro puzzle em %s", FormatDate(day), lang)
	}
	return n, nil
}

// Group é um dos 4 grupos. Number (1..4) define a cor e a ordem das dicas.
type Group struct {
	Number int
	Theme  string
	Words  [GroupSize]string
}

// Credits é o rodapé "Redação/Revisão".
type Credits struct {
	Author   string `json:"author"`
	Reviewer string `json:"reviewer"`
}

// Puzzle é o documento do dia já convertido. Groups[i].Number == i+1.
type Puzzle struct {
	ID            int
	Groups        [Groups]Group
	StartingBoard [Groups * GroupSize]string
	Credits       Credits
	Corrections   map[string]string // só exibição: original → mostrado
	Version       *int              // se mudar, o progresso salvo é descartado
}

// Display aplica as correções de exibição a uma palavra ou tema.
func (p Puzzle) Display(s string) string {
	if c, ok := p.Corrections[s]; ok && c != "" {
		return c
	}
	return s
}

// GroupOf é o número do grupo da palavra (0 se não é do puzzle).
func (p Puzzle) GroupOf(word string) int {
	for _, g := range p.Groups {
		if slices.Contains(g.Words[:], word) {
			return g.Number
		}
	}
	return 0
}

// Group devolve o grupo de número n (1..4).
func (p Puzzle) Group(n int) Group { return p.Groups[n-1] }

// fsValue é um valor tipado do Firestore REST.
type fsValue struct {
	StringValue  *string  `json:"stringValue"`
	IntegerValue *string  `json:"integerValue"` // int64 vem como string
	DoubleValue  *float64 `json:"doubleValue"`
	BooleanValue *bool    `json:"booleanValue"`
	ArrayValue   *struct {
		Values []fsValue `json:"values"` // ausente quando vazia
	} `json:"arrayValue"`
	MapValue *struct {
		Fields map[string]fsValue `json:"fields"`
	} `json:"mapValue"`
}

// plain converte para os tipos do encoding/json (nullValue e o resto viram nil).
func (v fsValue) plain() (any, error) {
	switch {
	case v.StringValue != nil:
		return *v.StringValue, nil
	case v.IntegerValue != nil:
		if _, err := strconv.ParseInt(*v.IntegerValue, 10, 64); err != nil {
			return nil, fmt.Errorf("integerValue %q", *v.IntegerValue)
		}
		return json.Number(*v.IntegerValue), nil
	case v.DoubleValue != nil:
		return *v.DoubleValue, nil
	case v.BooleanValue != nil:
		return *v.BooleanValue, nil
	case v.ArrayValue != nil:
		out := make([]any, 0, len(v.ArrayValue.Values))
		for _, e := range v.ArrayValue.Values {
			p, err := e.plain()
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		}
		return out, nil
	case v.MapValue != nil:
		return plainFields(v.MapValue.Fields)
	}
	return nil, nil
}

func plainFields(fields map[string]fsValue) (map[string]any, error) {
	out := make(map[string]any, len(fields))
	for k, f := range fields {
		p, err := f.plain()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = p
	}
	return out, nil
}

// rawPuzzle é o documento em JSON simples (§3.3 do protocolo).
type rawPuzzle struct {
	ID     int `json:"id"`
	Groups []struct {
		Number int      `json:"number"`
		Theme  string   `json:"theme"`
		Words  []string `json:"words"`
	} `json:"groups"`
	StartingBoard []string          `json:"startingBoard"`
	Credits       Credits           `json:"credits"`
	Corrections   map[string]string `json:"corrections"`
	Version       *int              `json:"version"`
}

// ParseDocument converte o documento tipado do Firestore e valida a forma:
// 4 grupos numerados 1..4 de 4 palavras distintas e um tabuleiro inicial que é
// permutação exata delas.
func ParseDocument(data []byte) (Puzzle, error) {
	var doc struct {
		Fields map[string]fsValue `json:"fields"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return Puzzle{}, fmt.Errorf("documento do Conexo: %w", err)
	}
	if len(doc.Fields) == 0 {
		return Puzzle{}, errors.New("documento do Conexo sem fields")
	}
	plain, err := plainFields(doc.Fields)
	if err != nil {
		return Puzzle{}, fmt.Errorf("documento do Conexo: %w", err)
	}
	buf, err := json.Marshal(plain)
	if err != nil {
		return Puzzle{}, err
	}
	var raw rawPuzzle
	if err := json.Unmarshal(buf, &raw); err != nil {
		return Puzzle{}, fmt.Errorf("documento do Conexo: %w", err)
	}
	return raw.puzzle()
}

func (raw rawPuzzle) puzzle() (Puzzle, error) {
	p := Puzzle{ID: raw.ID, Credits: raw.Credits, Corrections: raw.Corrections, Version: raw.Version}
	if len(raw.Groups) != Groups {
		return Puzzle{}, fmt.Errorf("conexo #%d: %d grupos", raw.ID, len(raw.Groups))
	}
	seen := map[string]bool{}
	for _, g := range raw.Groups {
		if g.Number < 1 || g.Number > Groups || p.Groups[g.Number-1].Number != 0 {
			return Puzzle{}, fmt.Errorf("conexo #%d: grupo com number %d", raw.ID, g.Number)
		}
		if len(g.Words) != GroupSize {
			return Puzzle{}, fmt.Errorf("conexo #%d: grupo %d com %d palavras", raw.ID, g.Number, len(g.Words))
		}
		for _, w := range g.Words {
			if w == "" || seen[w] {
				return Puzzle{}, fmt.Errorf("conexo #%d: palavra vazia ou repetida %q", raw.ID, w)
			}
			seen[w] = true
		}
		p.Groups[g.Number-1] = Group{Number: g.Number, Theme: g.Theme, Words: [GroupSize]string(g.Words)}
	}
	if len(raw.StartingBoard) != len(p.StartingBoard) {
		return Puzzle{}, fmt.Errorf("conexo #%d: tabuleiro com %d palavras", raw.ID, len(raw.StartingBoard))
	}
	for i, w := range raw.StartingBoard {
		if !seen[w] {
			return Puzzle{}, fmt.Errorf("conexo #%d: tabuleiro com %q fora dos grupos ou repetida", raw.ID, w)
		}
		delete(seen, w)
		p.StartingBoard[i] = w
	}
	return p, nil
}

// Attempt é uma linha do histórico: um chute (Selected, com Group 0 = erro)
// ou um uso de dica (Hints).
type Attempt struct {
	Selected []string `json:"selected,omitempty"` // 4 palavras na ordem da seleção
	Group    int      `json:"group,omitempty"`    // 1..4; 0 = erro
	Hints    []string `json:"hints,omitempty"`    // palavras reveladas
}

// IsHint diz se a linha é um uso de dica.
func (a Attempt) IsHint() bool { return len(a.Hints) > 0 }

var groupEmoji = [Groups + 1]string{"", "🟧", "🟩", "🟦", "🟪"}

// Emoji é a marca da linha no resultado: cor do grupo, 💡 ou ❌.
func (a Attempt) Emoji() string {
	switch {
	case a.IsHint():
		return "💡"
	case a.Group >= 1 && a.Group <= Groups:
		return groupEmoji[a.Group]
	}
	return "❌"
}

// Emojis é a grade do resultado, com quebra a cada 10.
func Emojis(attempts []Attempt) string {
	var b strings.Builder
	for i, a := range attempts {
		if i > 0 && i%emojisPerLine == 0 {
			b.WriteString("\n")
		}
		b.WriteString(a.Emoji())
	}
	return b.String()
}

// judge devolve o grupo que contém as 4 palavras, ou 0.
func judge(p Puzzle, selected []string) int {
	if len(selected) != GroupSize {
		return 0
	}
	for _, g := range p.Groups {
		if !slices.ContainsFunc(selected, func(w string) bool { return !slices.Contains(g.Words[:], w) }) {
			return g.Number
		}
	}
	return 0
}

// nextHint é a próxima dica (§4.3): 2 palavras do grupo não achado de menor
// number, depois 3 do mesmo grupo; nada com 3 grupos achados ou 3 dicas.
func nextHint(p Puzzle, found []int, hints []string) []string {
	if len(found) >= Groups-1 || len(hints) == 3 {
		return nil
	}
	if len(hints) == 2 {
		g := p.Group(p.GroupOf(hints[0]))
		return slices.Clone(g.Words[:3])
	}
	for _, g := range p.Groups {
		if !slices.Contains(found, g.Number) {
			return slices.Clone(g.Words[:2])
		}
	}
	return nil
}

// shareTexts são os modelos do i18next (ZZ no bundle) por idioma.
var shareTexts = map[string]struct{ played, hintOne, hintMany string }{
	"pt": {"Joguei conexo.ws %s e consegui em %d tentativas%s.", " e %d dica", " e %d dicas"},
	"en": {"I played conexo.ws %s and got it in %d guesses%s.", " and %d hint", " and %d hints"},
	"es": {"Jugué conexo.ws %s y lo conseguí en %d intentos%s.", " y %d pista", " y %d pistas"},
}

// ShareText é o texto de compartilhamento do site. Não há singular para as
// tentativas: o mínimo é 4.
func ShareText(lang string, day time.Time, tries, hints int, attempts []Attempt) string {
	t, ok := shareTexts[lang]
	if !ok {
		t = shareTexts["pt"]
	}
	return fmt.Sprintf(t.played, DisplayDate(lang, day), tries, hintSuffix(hints, t.hintOne, t.hintMany)) +
		"\n\n" + Emojis(attempts)
}

// ResultLine é a mensagem final da tela (pt).
func ResultLine(tries, hints int) string {
	return fmt.Sprintf("Você acertou em %d tentativas%s.", tries, hintSuffix(hints, " e %d dica", " e %d dicas"))
}

func hintSuffix(n int, one, many string) string {
	switch n {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf(one, n)
	}
	return fmt.Sprintf(many, n)
}

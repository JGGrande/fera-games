package conexo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"fera-games/internal/game"
)

// Game é o Conexo registrado no hub, num idioma.
type Game struct {
	provider Provider
	lang     string
}

// NewGame cria o jogo sobre um Provider (Client em produção). lang vazio = pt.
func NewGame(p Provider, lang string) *Game {
	if lang == "" {
		lang = "pt"
	}
	return &Game{provider: p, lang: lang}
}

func (g *Game) ID() string   { return "conexo" }
func (g *Game) Name() string { return "Conexo" }

// Lang é o idioma dos puzzles.
func (g *Game) Lang() string { return g.lang }

// NewSession implementa game.Game.
func (g *Game) NewSession(ctx context.Context, day time.Time) (game.Session, error) {
	s, err := g.Start(ctx, day)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Start baixa o puzzle do dia civil de day e começa uma partida nova.
func (g *Game) Start(ctx context.Context, day time.Time) (*Session, error) {
	id, err := Number(g.lang, day)
	if err != nil {
		return nil, err
	}
	p, err := g.provider.Puzzle(ctx, g.lang, id)
	if err != nil {
		return nil, err
	}
	return NewSession(p, g.lang, day), nil
}

var (
	// ErrFinished é devolvido pelas jogadas depois da vitória.
	ErrFinished = errors.New("a partida já terminou")
	// ErrUnknownWord é uma palavra que não está solta no tabuleiro.
	ErrUnknownWord = errors.New("palavra fora do tabuleiro")
	// ErrNoHint indica que não há dica agora (3 grupos achados ou 3 dicas
	// do mesmo grupo).
	ErrNoHint = errors.New("nenhuma dica disponível agora")
)

// Outcome é o efeito de Toggle: só (des)selecionou, ou chutou (Group 0 = erro).
type Outcome struct {
	Guessed bool
	Group   int
}

// Session é uma partida do Conexo. Não é segura para uso concorrente.
type Session struct {
	puzzle Puzzle
	lang   string
	day    time.Time

	items    []string // palavras soltas, na ordem do tabuleiro
	found    []int    // grupos achados, na ordem em que foram achados
	selected []string
	hints    []string
	attempts []Attempt
	status   game.Status
}

var _ game.Session = (*Session)(nil)

// NewSession monta a partida a partir de um puzzle já baixado.
func NewSession(p Puzzle, lang string, day time.Time) *Session {
	return &Session{puzzle: p, lang: lang, day: day, items: slices.Clone(p.StartingBoard[:]), status: game.Playing}
}

// Toggle seleciona ou desseleciona uma palavra solta. A 4ª seleção já é o
// chute, como no site (não há botão de enviar).
func (s *Session) Toggle(word string) (Outcome, error) {
	if s.status != game.Playing {
		return Outcome{}, ErrFinished
	}
	if i := slices.Index(s.selected, word); i >= 0 {
		s.selected = slices.Delete(s.selected, i, i+1)
		return Outcome{}, nil
	}
	if !slices.Contains(s.items, word) {
		return Outcome{}, fmt.Errorf("%w: %q", ErrUnknownWord, word)
	}
	s.selected = append(s.selected, word)
	if len(s.selected) < GroupSize {
		return Outcome{}, nil
	}
	sel := s.selected
	s.selected = nil
	return Outcome{Guessed: true, Group: s.guess(sel)}, nil
}

// checkGuess exige 4 palavras distintas e soltas.
func (s *Session) checkGuess(words []string) error {
	if len(words) != GroupSize {
		return fmt.Errorf("%w: o chute precisa de %d palavras", ErrUnknownWord, GroupSize)
	}
	for i, w := range words {
		if !slices.Contains(s.items, w) || slices.Contains(words[:i], w) {
			return fmt.Errorf("%w: %q", ErrUnknownWord, w)
		}
	}
	return nil
}

// guess julga 4 palavras já validadas e registra a tentativa (§4.2).
func (s *Session) guess(sel []string) int {
	n := judge(s.puzzle, sel)
	s.attempts = append(s.attempts, Attempt{Selected: sel, Group: n})
	if n == 0 {
		return 0
	}
	s.items = slices.DeleteFunc(s.items, func(w string) bool { return slices.Contains(sel, w) })
	s.found = append(s.found, n)
	if len(s.hints) > 0 && s.puzzle.GroupOf(s.hints[0]) == n {
		s.hints = nil
	}
	if len(s.found) == Groups {
		s.status = game.Won
	}
	return n
}

// Hint revela a próxima dica (§4.3). Dicas não contam como tentativa.
func (s *Session) Hint() ([]string, error) {
	if s.status != game.Playing {
		return nil, ErrFinished
	}
	h := nextHint(s.puzzle, s.found, s.hints)
	if h == nil {
		return nil, ErrNoHint
	}
	s.hints = h
	s.attempts = append(s.attempts, Attempt{Hints: h})
	return slices.Clone(h), nil
}

// Swap troca duas posições do tabuleiro (o arrastar do site). É salvo, mas
// não é tentativa.
func (s *Session) Swap(i, j int) error {
	if s.status != game.Playing {
		return ErrFinished
	}
	if i < 0 || j < 0 || i >= len(s.items) || j >= len(s.items) {
		return fmt.Errorf("conexo: troca fora do tabuleiro (%d, %d)", i, j)
	}
	s.items[i], s.items[j] = s.items[j], s.items[i]
	return nil
}

// Puzzle expõe o puzzle. Os grupos são a resposta: a TUI só mostra os achados.
func (s *Session) Puzzle() Puzzle { return s.puzzle }

// Day é o dia da partida.
func (s *Session) Day() time.Time { return s.day }

// Items são as palavras soltas, na ordem do tabuleiro.
func (s *Session) Items() []string { return slices.Clone(s.items) }

// Found são os grupos achados, na ordem em que foram achados.
func (s *Session) Found() []Group {
	out := make([]Group, len(s.found))
	for i, n := range s.found {
		out[i] = s.puzzle.Group(n)
	}
	return out
}

// Selected são as palavras selecionadas, na ordem da seleção.
func (s *Session) Selected() []string { return slices.Clone(s.selected) }

// Hints são as palavras da dica ativa.
func (s *Session) Hints() []string { return slices.Clone(s.hints) }

// HintGroup é o grupo da dica ativa (0 = nenhuma).
func (s *Session) HintGroup() int {
	if len(s.hints) == 0 {
		return 0
	}
	return s.puzzle.GroupOf(s.hints[0])
}

// Attempts é o histórico de chutes e dicas.
func (s *Session) Attempts() []Attempt { return slices.Clone(s.attempts) }

// Tries conta os chutes (dicas não entram).
func (s *Session) Tries() int { return tries(s.attempts) }

// HintsUsed conta os usos de dica.
func (s *Session) HintsUsed() int { return len(s.attempts) - tries(s.attempts) }

func tries(attempts []Attempt) int {
	n := 0
	for _, a := range attempts {
		if !a.IsHint() {
			n++
		}
	}
	return n
}

// State implementa game.Session. Steps = 0: tentativas ilimitadas, sem derrota.
func (s *Session) State() game.State {
	var guesses []string
	for _, a := range s.attempts {
		if !a.IsHint() {
			guesses = append(guesses, strings.Join(a.Selected, "|"))
		}
	}
	step := len(guesses) + 1
	if s.status != game.Playing {
		step = len(guesses)
	}
	return game.State{Day: s.day, Number: s.puzzle.ID, Step: step, Guesses: guesses, Status: s.status}
}

// Guess implementa game.Session: answer são as 4 palavras separadas por "|".
// A seleção em andamento é descartada.
func (s *Session) Guess(_ context.Context, answer string) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false, ""), ErrFinished
	}
	words := strings.Split(answer, "|")
	for i := range words {
		words[i] = strings.TrimSpace(words[i])
	}
	if err := s.checkGuess(words); err != nil {
		return s.result(false, ""), err
	}
	s.selected = nil
	n := s.guess(words)
	if n == 0 {
		return s.result(false, ""), nil
	}
	return s.result(true, s.puzzle.Display(s.puzzle.Group(n).Theme)), nil
}

// Skip implementa game.Session como uso de dica.
func (s *Session) Skip(context.Context) (game.Result, error) {
	h, err := s.Hint()
	return s.result(false, strings.Join(h, ", ")), err
}

func (s *Session) result(correct bool, hint string) game.Result {
	return game.Result{Correct: correct, Hint: hint, State: s.State()}
}

// ShareText é o texto de compartilhamento do site.
func (s *Session) ShareText() string {
	return ShareText(s.lang, s.day, s.Tries(), s.HintsUsed(), s.attempts)
}

// ResultLine é a mensagem final ("Você acertou em N tentativas…").
func (s *Session) ResultLine() string { return ResultLine(s.Tries(), s.HintsUsed()) }

// Snapshot é o estado serializável da partida: o histórico e a ordem do
// tabuleiro (trocas). A seleção não entra, como no site.
type Snapshot struct {
	Number   int       `json:"number"`
	Day      string    `json:"day"`
	Version  *int      `json:"version"`
	Attempts []Attempt `json:"attempts"`
	Items    []string  `json:"items"`
	Status   string    `json:"status"` // playing, won
}

// Finished é verdadeiro quando a partida acabou (no Conexo, só vencendo).
func (s Snapshot) Finished() bool { return s.Status == "won" }

// Tries conta os chutes.
func (s Snapshot) Tries() int { return tries(s.Attempts) }

// Snapshot fotografa a partida.
func (s *Session) Snapshot() Snapshot {
	status := "playing"
	if s.status == game.Won {
		status = "won"
	}
	return Snapshot{
		Number: s.puzzle.ID, Day: FormatDate(s.day), Version: s.puzzle.Version,
		Attempts: s.Attempts(), Items: s.Items(), Status: status,
	}
}

// ErrSnapshotMismatch indica um snapshot de outro puzzle ou inconsistente.
var ErrSnapshotMismatch = errors.New("estado salvo não corresponde ao puzzle")

// Restore reaplica um Snapshot (§5 do protocolo). O histórico é rejulgado
// contra o puzzle, então um arquivo editado não vira vitória; a ordem salva
// do tabuleiro só vale se for das mesmas palavras soltas.
func (s *Session) Restore(snap Snapshot) error {
	if snap.Number != s.puzzle.ID || !sameVersion(snap.Version, s.puzzle.Version) {
		return ErrSnapshotMismatch
	}
	r := NewSession(s.puzzle, s.lang, s.day)
	for _, a := range snap.Attempts {
		if r.status != game.Playing {
			return ErrSnapshotMismatch
		}
		if a.IsHint() {
			if h, err := r.Hint(); err != nil || !slices.Equal(h, a.Hints) {
				return ErrSnapshotMismatch
			}
			continue
		}
		if r.checkGuess(a.Selected) != nil || r.guess(slices.Clone(a.Selected)) != a.Group {
			return ErrSnapshotMismatch
		}
	}
	saved, now := slices.Clone(snap.Items), slices.Clone(r.items)
	slices.Sort(saved)
	slices.Sort(now)
	if !slices.Equal(saved, now) {
		return ErrSnapshotMismatch
	}
	r.items = slices.Clone(snap.Items)
	*s = *r
	return nil
}

func sameVersion(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

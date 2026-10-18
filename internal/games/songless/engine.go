package songless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"fera-games/internal/game"
)

// Game é o Songless registrado no hub. Cada categoria (All/Rock/Hip Hop) é
// uma partida independente de 5 tentativas sobre o mesmo Daily.
type Game struct {
	provider Provider
}

// NewGame cria o jogo sobre um Provider (Client em produção).
func NewGame(p Provider) *Game { return &Game{provider: p} }

func (g *Game) ID() string   { return "songless" }
func (g *Game) Name() string { return "Songless" }

// NewSession implementa game.Game: abre a categoria padrão (All, 0).
func (g *Game) NewSession(ctx context.Context, day time.Time) (game.Session, error) {
	return g.Start(ctx, day, 0)
}

// Search expõe o autocomplete remoto para a TUI.
func (g *Game) Search(ctx context.Context, query string) ([]Entry, error) {
	return g.provider.Search(ctx, query)
}

// Start abre a partida do dia na categoria dada.
func (g *Game) Start(ctx context.Context, day time.Time, categoryID int) (*Session, error) {
	d, err := g.provider.Daily(ctx, day)
	if err != nil {
		return nil, err
	}
	cat, ok := d.Category(categoryID)
	if !ok {
		return nil, fmt.Errorf("categoria %d não existe hoje", categoryID)
	}
	n, err := ChallengeNumber(d.Date)
	if err != nil {
		return nil, err
	}
	return &Session{provider: g.provider, daily: d, category: cat, number: n}, nil
}

var (
	// ErrFinished é devolvido por Guess/Skip depois da partida acabar.
	ErrFinished = errors.New("a partida já terminou")
	// ErrUnknownSong é devolvido quando o chute não resolve para uma música da busca.
	ErrUnknownSong = errors.New("música não está na lista")
)

// Cell é uma linha da grade.
type Cell struct {
	Entry Entry
	State Outcome
}

// Session é uma partida do Songless (1 categoria). Não é segura para uso concorrente.
type Session struct {
	provider Provider
	daily    Daily
	category Category
	number   int // challengeNumber, para o share

	board  []Cell
	status game.Status
	answer *Answer
}

var _ game.Session = (*Session)(nil)

// NewSession monta a partida a partir de dados já baixados (útil em testes e
// ao restaurar estado).
func NewSession(p Provider, d Daily, categoryID int) (*Session, error) {
	cat, ok := d.Category(categoryID)
	if !ok {
		return nil, fmt.Errorf("categoria %d não existe", categoryID)
	}
	n, err := ChallengeNumber(d.Date)
	if err != nil {
		return nil, err
	}
	return &Session{provider: p, daily: d, category: cat, number: n, status: game.Playing}, nil
}

// Snapshot é o estado serializável de uma partida. Como o julgamento é no
// servidor, Restore confia nos states salvos (validados quanto a tamanho e
// fim) — nunca rejulga.
type Snapshot struct {
	Date       string    `json:"date"`
	CategoryID int       `json:"categoryId"`
	Guesses    []Entry   `json:"guesses"`  // um por tentativa; ID vazio = skip
	Outcomes   []Outcome `json:"outcomes"` // mesmo tamanho de Guesses
	Status     string    `json:"status"`   // playing, won, lost
	Answer     *Answer   `json:"answer,omitempty"`
}

// Finished é verdadeiro quando a partida acabou.
func (s Snapshot) Finished() bool { return s.Status != "playing" }

// Won é verdadeiro quando a partida foi ganha.
func (s Snapshot) Won() bool { return s.Status == "won" }

// Tries é o número de tentativas usadas.
func (s Snapshot) Tries() int { return len(s.Outcomes) }

// Snapshot fotografa a partida.
func (s *Session) Snapshot() Snapshot {
	status := "playing"
	switch s.status {
	case game.Won:
		status = "won"
	case game.Lost:
		status = "lost"
	}
	guesses := make([]Entry, len(s.board))
	outcomes := make([]Outcome, len(s.board))
	for i, c := range s.board {
		guesses[i], outcomes[i] = c.Entry, c.State
	}
	return Snapshot{
		Date: s.daily.Date, CategoryID: s.category.ID,
		Guesses: guesses, Outcomes: outcomes, Status: status, Answer: s.answer,
	}
}

// ErrSnapshotMismatch indica um snapshot de outro dia/categoria ou inconsistente.
var ErrSnapshotMismatch = errors.New("estado salvo não corresponde à partida")

// Restore reaplica um Snapshot sobre uma sessão recém-criada. Os states são
// confiáveis (vieram do servidor na hora de jogar); aqui só validamos forma.
func (s *Session) Restore(snap Snapshot) error {
	if snap.Date != s.daily.Date || snap.CategoryID != s.category.ID {
		return ErrSnapshotMismatch
	}
	if len(snap.Guesses) != len(snap.Outcomes) || len(snap.Outcomes) > MaxGuesses {
		return ErrSnapshotMismatch
	}
	for _, o := range snap.Outcomes {
		switch o {
		case OutcomeCorrect, OutcomePresent, OutcomeIncorrect, OutcomeSkipped:
		default:
			return ErrSnapshotMismatch
		}
	}
	status := game.Playing
	switch snap.Status {
	case "playing":
	case "won":
		status = game.Won
	case "lost":
		status = game.Lost
	default:
		return ErrSnapshotMismatch
	}
	// Consistência: vitória termina no correct (nada depois); derrota tem 5.
	if status == game.Won {
		if len(snap.Outcomes) != countNeeded(status, snap.Outcomes) {
			return ErrSnapshotMismatch
		}
		// O correct precisa ser o último.
		if snap.Outcomes[len(snap.Outcomes)-1] != OutcomeCorrect {
			return ErrSnapshotMismatch
		}
	}
	if status == game.Lost && len(snap.Outcomes) != MaxGuesses {
		return ErrSnapshotMismatch
	}
	if status != game.Playing && len(snap.Outcomes) == 0 {
		return ErrSnapshotMismatch
	}
	s.board = make([]Cell, len(snap.Outcomes))
	for i := range snap.Outcomes {
		s.board[i] = Cell{Entry: snap.Guesses[i], State: snap.Outcomes[i]}
	}
	s.status, s.answer = status, snap.Answer
	return nil
}

func countNeeded(status game.Status, outcomes []Outcome) int {
	if status == game.Lost {
		return MaxGuesses
	}
	for i, o := range outcomes {
		if o == OutcomeCorrect {
			return i + 1
		}
	}
	return len(outcomes)
}

// Daily expõe o dia (data do servidor + categorias).
func (s *Session) Daily() Daily { return s.daily }

// Provider expõe o provider para a TUI criar outras categorias do mesmo dia.
func (s *Session) Provider() Provider { return s.provider }

// Category expõe a categoria desta partida.
func (s *Session) Category() Category { return s.category }

// Answer expõe a resposta revelada (nil enquanto joga).
func (s *Session) Answer() *Answer { return s.answer }

// Board é a grade até aqui.
func (s *Session) Board() []Cell {
	out := make([]Cell, len(s.board))
	copy(out, s.board)
	return out
}

// Outcomes é o resultado de cada tentativa, para o share.
func (s *Session) Outcomes() []Outcome {
	out := make([]Outcome, len(s.board))
	for i, c := range s.board {
		out[i] = c.State
	}
	return out
}

// State implementa game.Session.
func (s *Session) State() game.State {
	day, _ := time.Parse("2006-01-02", s.daily.Date)
	guesses := make([]string, len(s.board))
	for i, c := range s.board {
		if c.State == OutcomeSkipped {
			guesses[i] = ""
		} else {
			guesses[i] = c.Entry.Display()
		}
	}
	step := len(s.board) + 1
	if s.status != game.Playing {
		step = len(s.board)
		if step == 0 {
			step = 1
		}
	}
	return game.State{
		Day: day, Number: s.number, Step: step, Steps: MaxGuesses,
		Guesses: guesses, Status: s.status,
	}
}

// AudioStep é o round atual (1-based), para o limite de reprodução.
// O arquivo é sempre o mesmo; o que muda é até onde pode tocar.
func (s *Session) AudioStep() int {
	if s.status != game.Playing {
		return MaxGuesses
	}
	return min(len(s.board)+1, MaxGuesses)
}

// AudioLimit é até onde o player pode tocar agora.
func (s *Session) AudioLimit() time.Duration { return LimitFor(s.AudioStep()) }

// Audio abre o .aac da categoria (ver AudioStep para o limite).
func (s *Session) Audio(ctx context.Context) (io.ReadCloser, error) {
	return s.provider.Audio(ctx, s.category)
}

// Guess implementa game.Session: resolve answer via busca (texto exato
// "Artista - Faixa", ignorando caixa) e chuta o ID encontrado.
func (s *Session) Guess(ctx context.Context, answer string) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false, ""), ErrFinished
	}
	entries, err := s.provider.Search(ctx, answer)
	if err != nil {
		return s.result(false, ""), err
	}
	var found *Entry
	for i, e := range entries {
		if strings.EqualFold(strings.TrimSpace(e.Display()), strings.TrimSpace(answer)) {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		return s.result(false, ""), fmt.Errorf("%w: %q", ErrUnknownSong, answer)
	}
	return s.GuessEntry(ctx, *found)
}

// GuessEntry chuta uma música vinda da busca (o caminho que a TUI usa).
func (s *Session) GuessEntry(ctx context.Context, e Entry) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false, ""), ErrFinished
	}
	id := e.ID
	outcome, answer, err := s.provider.Guess(ctx, Guess{
		Date: s.daily.Date, CategoryID: s.category.ID,
		RoundIndex: len(s.board), SongID: &id,
	})
	if err != nil {
		return s.result(false, ""), err
	}
	s.record(e, outcome, answer)
	hint := ""
	if outcome == OutcomePresent {
		hint = "Artista certo!"
	}
	return s.result(outcome == OutcomeCorrect, hint), nil
}

// Skip pula a tentativa atual.
func (s *Session) Skip(ctx context.Context) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false, ""), ErrFinished
	}
	outcome, answer, err := s.provider.Guess(ctx, Guess{
		Date: s.daily.Date, CategoryID: s.category.ID,
		RoundIndex: len(s.board), SongID: nil,
	})
	if err != nil {
		return s.result(false, ""), err
	}
	s.record(Entry{}, outcome, answer)
	return s.result(false, ""), nil
}

func (s *Session) record(e Entry, o Outcome, a *Answer) {
	s.board = append(s.board, Cell{Entry: e, State: o})
	if a != nil {
		cp := *a
		s.answer = &cp
	}
	switch {
	case o == OutcomeCorrect:
		s.status = game.Won
	case len(s.board) >= MaxGuesses:
		s.status = game.Lost
	}
}

func (s *Session) result(correct bool, hint string) game.Result {
	return game.Result{Correct: correct, Hint: hint, State: s.State()}
}

// ShareText é o resultado no formato do site (1 categoria).
func (s *Session) ShareText() string {
	guesses := len(s.board)
	if s.status == game.Playing {
		guesses = 0
	}
	return ShareText(s.daily.Date, s.category.Name, s.status == game.Won, guesses, s.Outcomes())
}

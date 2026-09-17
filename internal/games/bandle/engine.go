package bandle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"fera-games/internal/game"
)

// Game é o Bandle registrado no hub.
type Game struct {
	provider Provider
}

// NewGame cria o jogo sobre um Provider (Client em produção).
func NewGame(p Provider) *Game { return &Game{provider: p} }

func (g *Game) ID() string   { return "bandle" }
func (g *Game) Name() string { return "Bandle" }

// NewSession baixa o puzzle e o catálogo do dia e começa uma partida nova.
func (g *Game) NewSession(ctx context.Context, day time.Time) (game.Session, error) {
	s, err := g.Start(ctx, day)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Start é NewSession com o tipo concreto, para quem precisa do que é só do
// Bandle (áudio, dicas, catálogo).
func (g *Game) Start(ctx context.Context, day time.Time) (*Session, error) {
	p, err := g.provider.DailyPuzzle(ctx, day)
	if err != nil {
		return nil, err
	}
	cat, err := g.provider.SongCatalog(ctx)
	if err != nil {
		return nil, err
	}
	return NewSession(g.provider, p, cat, day), nil
}

var (
	// ErrFinished é devolvido por Guess/Skip depois da partida acabar.
	ErrFinished = errors.New("a partida já terminou")
	// ErrUnknownSong é devolvido quando o chute não está no catálogo.
	ErrUnknownSong = errors.New("música não está na lista")
)

// Session é uma partida do Bandle. Não é segura para uso concorrente.
type Session struct {
	provider Provider
	puzzle   Puzzle
	catalog  Catalog
	day      time.Time

	step     int // tentativa atual, 1-based
	guesses  []string
	outcomes []Outcome
	status   game.Status
}

var _ game.Session = (*Session)(nil)

// NewSession monta a partida a partir de dados já baixados (útil em testes e
// ao restaurar estado). O catálogo recebe a resposta via MergeAnswer.
func NewSession(p Provider, puzzle Puzzle, catalog Catalog, day time.Time) *Session {
	return &Session{
		provider: p,
		puzzle:   puzzle,
		catalog:  MergeAnswer(catalog, puzzle),
		day:      day,
		step:     1,
		status:   game.Playing,
	}
}

// Snapshot é o estado serializável de uma partida, sem a resposta. Basta
// para restaurar a Session com o mesmo puzzle e catálogo.
type Snapshot struct {
	Number   int      `json:"number"`
	Day      string   `json:"day"`
	Guesses  []string `json:"guesses"`
	Outcomes string   `json:"outcomes"` // um código por tentativa: g, y, r, b
	Status   string   `json:"status"`   // playing, won, lost
}

// Finished é verdadeiro quando a partida acabou.
func (s Snapshot) Finished() bool { return s.Status != "playing" }

// Won é verdadeiro quando a partida foi ganha.
func (s Snapshot) Won() bool { return s.Status == "won" }

// Tries é o número de tentativas usadas.
func (s Snapshot) Tries() int { return len(s.Outcomes) }

// Snapshot fotografa a partida.
func (s *Session) Snapshot() Snapshot {
	out := make([]byte, len(s.outcomes))
	for i, o := range s.outcomes {
		out[i] = byte(o)
	}
	status := "playing"
	switch s.status {
	case game.Won:
		status = "won"
	case game.Lost:
		status = "lost"
	}
	return Snapshot{
		Number:   s.puzzle.ID,
		Day:      FormatDate(s.day),
		Guesses:  append([]string(nil), s.guesses...),
		Outcomes: string(out),
		Status:   status,
	}
}

// ErrSnapshotMismatch indica um snapshot de outro puzzle ou inconsistente.
var ErrSnapshotMismatch = errors.New("estado salvo não corresponde ao puzzle")

// Restore reaplica um Snapshot sobre uma sessão recém-criada. Os chutes são
// rejulgados contra o puzzle, então um arquivo editado não vira vitória.
func (s *Session) Restore(snap Snapshot) error {
	if snap.Number != s.puzzle.ID || len(snap.Guesses) != len(snap.Outcomes) {
		return ErrSnapshotMismatch
	}
	if len(snap.Outcomes) > s.puzzle.Stages() {
		return ErrSnapshotMismatch
	}
	for i, code := range []byte(snap.Outcomes) {
		if s.status != game.Playing {
			return ErrSnapshotMismatch
		}
		if Outcome(code) == OutcomeSkipped {
			s.record("", OutcomeSkipped)
			continue
		}
		entry, ok := s.catalog.Find(snap.Guesses[i])
		if !ok {
			// Música que saiu do catálogo: mantém o texto, mas nunca como acerto.
			entry = CatalogEntry{Title: snap.Guesses[i]}
		}
		s.record(entry.Title, Judge(s.puzzle, &entry))
	}
	return nil
}

// Puzzle expõe o puzzle (dicas, instrumentos, par). A resposta está em
// Puzzle.Song: a TUI não deve mostrá-la antes do fim.
func (s *Session) Puzzle() Puzzle { return s.puzzle }

// Catalog é a lista para o autocomplete, já com a resposta incluída.
func (s *Session) Catalog() Catalog { return s.catalog }

// Outcomes é o resultado de cada tentativa já feita.
func (s *Session) Outcomes() []Outcome { return append([]Outcome(nil), s.outcomes...) }

// State implementa game.Session.
func (s *Session) State() game.State {
	return game.State{
		Day:     s.day,
		Number:  s.puzzle.ID,
		Step:    s.step,
		Steps:   s.puzzle.Stages(),
		Guesses: append([]string(nil), s.guesses...),
		Status:  s.status,
	}
}

// AudioStep é o estágio de áudio a tocar agora: acompanha step até o último
// mp3; o estágio da dica reutiliza o último. Depois do fim, toca tudo.
func (s *Session) AudioStep() int {
	if s.status != game.Playing {
		return s.puzzle.AudioStages()
	}
	return min(s.step, s.puzzle.AudioStages())
}

// Stem abre o áudio do estágio atual (ver AudioStep).
func (s *Session) Stem(ctx context.Context) (io.ReadCloser, error) {
	return s.StemAt(ctx, s.AudioStep())
}

// StemAt abre o áudio de um estágio específico. Só lê dados imutáveis, então
// pode ser chamado de outra goroutine (um tea.Cmd) enquanto a partida anda.
func (s *Session) StemAt(ctx context.Context, step int) (io.ReadCloser, error) {
	return s.provider.Stem(ctx, s.puzzle, step)
}

// UnlockedInstruments são os nomes dos instrumentos já liberados, na ordem
// em que entraram; inclui "clue" no último estágio.
func (s *Session) UnlockedInstruments() []string {
	n := s.step
	if s.status != game.Playing {
		n = s.puzzle.Stages()
	}
	return append([]string(nil), s.puzzle.Instruments[:min(n, s.puzzle.Stages())]...)
}

// Clue é a dica textual, só disponível no último estágio ou após o fim.
func (s *Session) Clue(lang string) (string, bool) {
	if s.status == game.Playing && s.step < s.puzzle.Stages() {
		return "", false
	}
	if c, ok := s.puzzle.Clue[lang]; ok {
		return c, true
	}
	return s.puzzle.Clue["en"], s.puzzle.Clue["en"] != ""
}

// Guess registra um chute pelo título do catálogo ("Artista - Título").
func (s *Session) Guess(_ context.Context, answer string) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false, ""), ErrFinished
	}
	entry, ok := s.catalog.Find(answer)
	if !ok {
		return s.result(false, ""), fmt.Errorf("%w: %q", ErrUnknownSong, answer)
	}
	outcome := Judge(s.puzzle, &entry)
	s.record(entry.Title, outcome)
	hint := ""
	if outcome == OutcomeRightArtist {
		hint = "Artista certo!"
	}
	return s.result(outcome == OutcomeCorrect, hint), nil
}

// Skip pula a tentativa atual e libera o próximo instrumento.
func (s *Session) Skip(context.Context) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false, ""), ErrFinished
	}
	s.record("", OutcomeSkipped)
	return s.result(false, ""), nil
}

func (s *Session) record(text string, o Outcome) {
	s.guesses = append(s.guesses, text)
	s.outcomes = append(s.outcomes, o)
	switch {
	case o == OutcomeCorrect:
		s.status = game.Won
	case s.step >= s.puzzle.Stages():
		s.status = game.Lost
	default:
		s.step++
	}
}

func (s *Session) result(correct bool, hint string) game.Result {
	return game.Result{Correct: correct, Hint: hint, State: s.State()}
}

// ShareText é o resultado no formato da SPA. As linhas de estatísticas
// (Found, streak) dependem do histórico local e entram com Stats.
func (s *Session) ShareText() string { return s.ShareTextWith(Stats{}) }

// Stats é o histórico que o texto de compartilhamento mostra.
type Stats struct {
	Played, Won, CurrentStreak, MaxStreak int
}

// ShareTextWith monta o texto com as estatísticas dadas (zeradas = omitidas).
func (s *Session) ShareTextWith(st Stats) string {
	score := fmt.Sprint(s.step)
	if s.status == game.Lost {
		score = "x"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Bandle #%d %s/%d\n", s.puzzle.ID, score, s.puzzle.Stages())
	for _, o := range s.outcomes {
		b.WriteString(o.Emoji())
	}
	b.WriteString("\n")
	if st.Played > 0 {
		// Mesmo arredondamento da SPA: uma casa, sem zero à direita (100%, 66.7%).
		pct := math.Round(float64(st.Won)/float64(st.Played)*1000) / 10
		fmt.Fprintf(&b, "Found: %d/%d (%s%%)\n", st.Won, st.Played, strconv.FormatFloat(pct, 'f', -1, 64))
	}
	if st.MaxStreak > 0 {
		fmt.Fprintf(&b, "Current streak: %d (max %d)\n", st.CurrentStreak, st.MaxStreak)
	}
	b.WriteString("#Bandle")
	return b.String()
}

// Package conexoui é a tela de jogo do Conexo: grade 4×4 navegada com as
// setas, seleção com espaço, troca de blocos com shift+setas e faixas dos
// grupos achados, sobre a engine de internal/games/conexo.
package conexoui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"fera-games/internal/game"
	"fera-games/internal/games/conexo"
	"fera-games/internal/store"
	"fera-games/internal/tui/clip"
)

// Deps é o que a tela precisa de fora.
type Deps struct {
	Game    *conexo.Game
	Tokens  *conexo.TokenStore // onde o token colado fica; nil = não pede token
	Now     func() time.Time
	Timeout time.Duration // por requisição
	// Store persiste a partida do dia; nil desliga.
	Store *store.Store
}

const gameID = "conexo"

// BackMsg pede ao app para voltar ao menu.
type BackMsg struct{}

type phase int

const (
	phaseLoading phase = iota
	phaseToken
	phasePlaying
	phaseFinished
	phaseError
)

const (
	cols         = conexo.GroupSize
	flashFor     = time.Second // peças vermelhas depois de um erro
	animEvery    = 40 * time.Millisecond
	winStagger   = 3 // quadros entre o brilho de uma faixa e o da próxima
	loadingLabel = "baixando o puzzle de hoje…"
	missNotice   = "não formam um grupo"
	tokenWarnAt  = 24 * time.Hour
)

// animKind é a animação em curso; ela só enfeita (o texto nunca some).
type animKind int

const (
	animNone   animKind = iota
	animShake           // erro: as 4 peças tremem
	animReveal          // acerto: brilho atravessa a faixa nova
	animWin             // vitória: brilho em cascata pelas 4 faixas
)

// shakeOffsets são as colunas da peça a cada quadro da tremida (1 = parada).
var shakeOffsets = []int{0, 2, 0, 2, 0, 2, 1}

type anim struct {
	kind         animKind
	frame, total int
	seq          int
}

// Mensagens internas.
type (
	loadedMsg    struct{ session *conexo.Session }
	errMsg       struct{ err error }
	retryMsg     struct{}
	flashDoneMsg struct{ seq int }
	animMsg      struct{ seq int }
)

// Model é a tela.
type Model struct {
	deps    Deps
	phase   phase
	session *conexo.Session
	err     error

	spinner spinner.Model
	input   textinput.Model // token
	help    help.Model
	keys    KeyMap

	cursor   int      // índice em Session.Items()
	flash    []string // palavras do último erro, enquanto a borda está vermelha
	flashSeq int
	anim     anim
	notice   string
	copied   bool
	restored bool // partida de hoje veio do disco
	tokenExp time.Time

	width, height int
}

// New cria a tela pronta para Init carregar o puzzle.
func New(d Deps) Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Timeout == 0 {
		d.Timeout = 30 * time.Second
	}
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "cole o token aqui"
	in.CharLimit = 4096
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	return Model{deps: d, spinner: sp, input: in, help: help.New(), keys: DefaultKeyMap}
}

// Session expõe a partida (nil enquanto carrega).
func (m Model) Session() *conexo.Session { return m.session }

// Init dispara o carregamento.
func (m Model) Init() tea.Cmd { return tea.Batch(m.load(), m.spinner.Tick) }

// Resume refaz o que se perdeu enquanto o menu estava na frente (o app não
// encaminha mensagens a telas inativas): a carga em andamento, o fim do
// destaque de erro e a animação.
func (m Model) Resume() tea.Cmd {
	if m.phase == phaseLoading {
		return m.Init()
	}
	seq := m.flashSeq
	return tea.Batch(func() tea.Msg { return flashDoneMsg{seq} }, m.animTick())
}

// startAnim troca a animação em curso e devolve o primeiro tick.
func (m *Model) startAnim(kind animKind, total int) tea.Cmd {
	m.anim = anim{kind: kind, total: total, seq: m.anim.seq + 1}
	return m.animTick()
}

func (m Model) animTick() tea.Cmd {
	if m.anim.kind == animNone {
		return nil
	}
	seq := m.anim.seq
	return tea.Tick(animEvery, func(time.Time) tea.Msg { return animMsg{seq} })
}

// Quadros de cada animação: o brilho anda shineStep colunas por quadro até
// sair da faixa; na vitória, a última faixa começa winStagger*3 depois.
var (
	revealFrames = (bandFront+shineWidth)/shineStep + 2
	winFrames    = revealFrames + winStagger*(conexo.Groups-1)
)

func (m Model) load() tea.Cmd {
	d := m.deps
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), d.Timeout)
		defer cancel()
		s, err := d.Game.Start(ctx, d.Now())
		if err != nil {
			return errMsg{err}
		}
		return loadedMsg{s}
	}
}

// Update trata mensagens.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.input.SetWidth(min(msg.Width-8, 70))
		return m, nil

	case spinner.TickMsg:
		if m.phase != phaseLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case errMsg:
		if m.deps.Tokens != nil && (errors.Is(msg.err, conexo.ErrNoToken) || errors.Is(msg.err, conexo.ErrAppCheck)) {
			m.phase, m.err = phaseToken, msg.err
			m.input.Reset()
			return m, m.input.Focus()
		}
		m.phase, m.err = phaseError, msg.err
		return m, nil

	case retryMsg:
		m.phase, m.err = phaseLoading, nil
		return m, m.Init()

	case loadedMsg:
		m.session, m.phase, m.cursor, m.err = msg.session, phasePlaying, 0, nil
		m.tokenExp, _ = m.deps.Tokens.Expiry()
		m.restore()
		if m.session.State().Status != game.Playing {
			m.finish()
		}
		return m, nil

	case flashDoneMsg:
		if msg.seq == m.flashSeq {
			m.flash = nil
		}
		return m, nil

	case animMsg:
		if msg.seq != m.anim.seq || m.anim.kind == animNone {
			return m, nil
		}
		if m.anim.frame++; m.anim.frame >= m.anim.total {
			m.anim = anim{seq: m.anim.seq}
			return m, nil
		}
		return m, m.animTick()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	if m.phase == phaseToken {
		// Colar (tea.PasteMsg), blink do cursor e afins.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if m.phase == phaseToken {
		return m.handleTokenKey(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Menu):
		return m, func() tea.Msg { return BackMsg{} }
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	}
	switch m.phase {
	case phaseError:
		if key.Matches(msg, m.keys.Retry) {
			return m, func() tea.Msg { return retryMsg{} }
		}
		return m, nil
	case phaseFinished:
		if key.Matches(msg, m.keys.Copy) {
			m.copied = true
			return m, clip.Copy(m.session.ShareText())
		}
		return m, nil
	case phasePlaying:
	default:
		return m, nil
	}

	for _, d := range []struct {
		cursor, swap key.Binding
		dx, dy       int
	}{
		{m.keys.Left, m.keys.MoveLeft, -1, 0},
		{m.keys.Right, m.keys.MoveRight, 1, 0},
		{m.keys.Up, m.keys.MoveUp, 0, -1},
		{m.keys.Down, m.keys.MoveDown, 0, 1},
	} {
		switch {
		case key.Matches(msg, d.cursor):
			if i, ok := m.neighbor(d.dx, d.dy); ok {
				m.cursor = i
			}
			return m, nil
		case key.Matches(msg, d.swap):
			return m.swap(d.dx, d.dy), nil
		}
	}
	switch {
	case key.Matches(msg, m.keys.Select):
		return m.toggle()
	case key.Matches(msg, m.keys.Hint):
		return m.hint(), nil
	}
	return m, nil
}

// neighbor é a célula vizinha na grade (sem dar a volta nas bordas).
func (m Model) neighbor(dx, dy int) (int, bool) {
	row, col := m.cursor/cols+dy, m.cursor%cols+dx
	if row < 0 || col < 0 || col >= cols {
		return 0, false
	}
	i := row*cols + col
	return i, i < len(m.session.Items())
}

// swap troca o bloco do cursor com o vizinho; o cursor acompanha o bloco.
func (m Model) swap(dx, dy int) Model {
	i, ok := m.neighbor(dx, dy)
	if !ok {
		return m
	}
	if err := m.session.Swap(m.cursor, i); err != nil {
		m.notice = err.Error()
		return m
	}
	m.cursor = i
	m.save()
	return m
}

// toggle (des)seleciona a palavra do cursor; a 4ª seleção é julgada.
func (m Model) toggle() (Model, tea.Cmd) {
	items := m.session.Items()
	if m.cursor >= len(items) {
		return m, nil
	}
	out, err := m.session.Toggle(items[m.cursor])
	if err != nil {
		m.notice = err.Error()
		return m, nil
	}
	m.notice = ""
	if !out.Guessed {
		return m, nil
	}
	if out.Group == 0 {
		attempts := m.session.Attempts()
		m.flash = attempts[len(attempts)-1].Selected
		m.flashSeq++
		m.notice = missNotice
		m.save()
		seq := m.flashSeq
		return m, tea.Batch(
			tea.Tick(flashFor, func(time.Time) tea.Msg { return flashDoneMsg{seq} }),
			m.startAnim(animShake, len(shakeOffsets)),
		)
	}
	m.flash = nil
	m.cursor = min(m.cursor, max(len(m.session.Items())-1, 0))
	if m.session.State().Status != game.Playing {
		m.finish()
		return m, m.startAnim(animWin, winFrames)
	}
	m.save()
	return m, m.startAnim(animReveal, revealFrames)
}

// hint pede a próxima dica; as palavras ganham a borda na cor do grupo.
func (m Model) hint() Model {
	h, err := m.session.Hint()
	switch {
	case errors.Is(err, conexo.ErrNoHint) && len(m.session.Found()) >= conexo.Groups-1:
		m.notice = "sem dica para o último grupo"
	case errors.Is(err, conexo.ErrNoHint):
		m.notice = "dica esgotada: ache primeiro o grupo das palavras marcadas"
	case err != nil:
		m.notice = err.Error()
	default:
		p := m.session.Puzzle()
		for i, w := range h {
			h[i] = strings.ToUpper(p.Display(w))
		}
		m.notice = "dica: " + strings.Join(h, ", ")
		m.save()
	}
	return m
}

func (m Model) handleTokenKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Cancel):
		return m, func() tea.Msg { return BackMsg{} }
	case key.Matches(msg, m.keys.Submit):
		if _, err := m.deps.Tokens.Set(m.input.Value()); err != nil {
			m.notice = err.Error()
			return m, nil
		}
		m.notice = ""
		m.input.Reset()
		m.input.Blur()
		return m, func() tea.Msg { return retryMsg{} }
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// finish entra na fase final e grava a vitória.
func (m *Model) finish() {
	m.phase, m.notice, m.flash = phaseFinished, "", nil
	m.save()
}

// save grava o estado atual da partida; falhas viram aviso, não erro fatal.
func (m *Model) save() {
	if m.deps.Store == nil || m.session == nil {
		return
	}
	snap := m.session.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	err = m.deps.Store.Save(gameID, store.Match{
		Day: snap.Day, Number: snap.Number, Status: snap.Status, Tries: snap.Tries(),
		Finished: snap.Finished(), Data: data,
	})
	if err != nil {
		m.notice = "não deu para salvar a partida: " + err.Error()
	}
}

// restore reaplica a partida de hoje salva em disco, se houver.
func (m *Model) restore() {
	if m.deps.Store == nil {
		return
	}
	match, ok, err := m.deps.Store.Load(gameID, conexo.FormatDate(m.deps.Now()))
	if err != nil {
		m.notice = "histórico ilegível: " + err.Error()
		return
	}
	if !ok {
		return
	}
	var snap conexo.Snapshot
	if json.Unmarshal(match.Data, &snap) != nil || m.session.Restore(snap) != nil {
		return // estado de outro puzzle ou corrompido: começa do zero
	}
	m.restored = len(snap.Attempts) > 0
}

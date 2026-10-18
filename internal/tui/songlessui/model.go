// Package songlessui é a tela de jogo do Songless: player com limite por
// round, grade de 5 tentativas, autocomplete remoto e tabs de categoria.
package songlessui

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

	"fera-games/internal/audio"
	"fera-games/internal/game"
	"fera-games/internal/games/songless"
	"fera-games/internal/store"
	"fera-games/internal/tui/clip"
)

// Deps é o que a tela precisa de fora.
type Deps struct {
	Game    *songless.Game
	Player  audio.Player
	Now     func() time.Time
	Timeout time.Duration // por requisição
	// AudioNotice é mostrado quando o áudio não pôde ser aberto (ex.: --mute).
	AudioNotice string
	// Store persiste a partida do dia por categoria; nil desliga.
	Store *store.Store
}

const gameID = "songless"

// BackMsg pede ao app para voltar ao menu.
type BackMsg struct{}

type phase int

const (
	phaseLoading phase = iota
	phasePlaying
	phaseFinished
	phaseError
)

const (
	seekStep     = 1 * time.Second
	tickEvery    = 100 * time.Millisecond
	maxMatches   = 6
	loadingLabel = "baixando a música de hoje…"
	searchDelay  = 250 * time.Millisecond
)

// Mensagens internas.
type (
	loadedMsg struct {
		daily    songless.Daily
		sessions map[int]*songless.Session
		current  int
	}
	errMsg   struct{ err error }
	trackMsg struct {
		category int
		track    *audio.Track
		err      error
	}
	tickMsg   struct{}
	retryMsg  struct{}
	searchMsg struct {
		seq     int
		results []songless.Entry
		err     error
	}
	guessMsg struct {
		category int
		entry    *songless.Entry // nil = skip
		result   game.Result
		answer   *songless.Answer
		err      error
	}
)

// Model é a tela.
type Model struct {
	deps     Deps
	phase    phase
	daily    songless.Daily
	session  *songless.Session
	sessions map[int]*songless.Session
	category int
	err      error

	spinner spinner.Model
	input   textinput.Model
	help    help.Model
	keys    KeyMap

	typing    bool
	matches   []songless.Entry
	sel       int
	searching bool
	searchSeq int
	notice    string
	copied    bool

	audioReady   bool // faixa da categoria atual carregada
	loadingAudio bool
	loadedCat    int // categoria carregada no player (-1 = nenhuma)

	restored bool

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
	in.Placeholder = "busque a música…"
	in.CharLimit = 120
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	return Model{deps: d, spinner: sp, input: in, help: help.New(), keys: DefaultKeyMap, loadedCat: -1}
}

// Session expõe a partida atual (nil enquanto carrega).
func (m Model) Session() *songless.Session { return m.session }

// Init dispara o carregamento e o tick do player.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.spinner.Tick, tick())
}

func tick() tea.Cmd { return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} }) }

// Resume religa o tick do player ao voltar do menu.
func (m Model) Resume() tea.Cmd { return tick() }

func (m Model) load() tea.Cmd {
	d := m.deps
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), d.Timeout)
		defer cancel()
		day := d.Now()
		// Carrega a categoria atual (All) e guarda o Daily para as outras.
		s0, err := d.Game.Start(ctx, day, 0)
		if err != nil {
			return errMsg{err}
		}
		daily := s0.Daily()
		sessions := map[int]*songless.Session{0: s0}
		return loadedMsg{daily: daily, sessions: sessions, current: 0}
	}
}

func (m Model) fetchAudio(cat int) tea.Cmd {
	s := m.sessions[cat]
	if s == nil {
		s = m.session
	}
	timeout := m.deps.Timeout
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		rc, err := s.Audio(ctx)
		if err != nil {
			return trackMsg{category: cat, err: err}
		}
		defer rc.Close()
		t, err := audio.DecodeAAC(rc)
		return trackMsg{category: cat, track: t, err: err}
	}
}

func (m Model) searchCmd(seq int, query string) tea.Cmd {
	timeout := m.deps.Timeout
	g := m.deps.Game
	return func() tea.Msg {
		// Debounce simples: espera antes de bater na API.
		time.Sleep(searchDelay)
		if query == "" {
			return searchMsg{seq: seq}
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		results, err := g.Search(ctx, query)
		return searchMsg{seq: seq, results: results, err: err}
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

	case tickMsg:
		// Pausa no limite do round, como o setTimeout do site.
		if m.session != nil && m.phase == phasePlaying && m.audioReady {
			if lim := m.session.AudioLimit(); lim > 0 {
				if m.deps.Player.Position() >= lim {
					m.deps.Player.Pause()
					m.deps.Player.SeekTo(lim)
				}
			}
		}
		return m, tick()

	case spinner.TickMsg:
		if m.phase != phaseLoading && !m.searching {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case errMsg:
		m.phase, m.err = phaseError, msg.err
		return m, nil

	case retryMsg:
		m.phase, m.err = phaseLoading, nil
		m.loadedCat, m.audioReady, m.loadingAudio = -1, false, false
		return m, tea.Batch(m.load(), m.spinner.Tick)

	case loadedMsg:
		m.daily = msg.daily
		m.sessions = msg.sessions
		m.category = msg.current
		m.session = m.sessions[m.category]
		m.phase = phasePlaying
		m.restore()
		if m.session.State().Status != game.Playing {
			m.finish()
		}
		return m, m.ensureAudio()

	case trackMsg:
		if msg.category != m.category {
			return m, nil // resposta atrasada de outra categoria
		}
		m.loadingAudio = false
		if msg.err != nil {
			m.notice = "áudio indisponível: " + msg.err.Error()
			return m, nil
		}
		m.audioReady, m.loadedCat = true, msg.category
		m.deps.Player.Restart(msg.track)
		return m, nil

	case searchMsg:
		if msg.seq != m.searchSeq {
			return m, nil // busca antiga
		}
		m.searching = false
		if msg.err != nil {
			m.notice = "busca falhou: " + msg.err.Error()
			return m, nil
		}
		m.matches = msg.results[:min(len(msg.results), maxMatches)]
		if m.sel >= len(m.matches) {
			m.sel = 0
		}
		return m, nil

	case guessMsg:
		if msg.category != m.category {
			return m, nil
		}
		return m.applyGuess(msg), m.ensureAudio()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	if m.typing {
		var cmd tea.Cmd
		before := m.input.Value()
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.refreshSearch()
			// O blink do cursor gera cmd lento; o teste descarta.
			return m, tea.Batch(cmd, m.pendingSearchCmd())
		}
		return m, cmd
	}
	return m, nil
}

// pendingSearchCmd dispara a busca remota com debounce (seq atual).
func (m *Model) pendingSearchCmd() tea.Cmd {
	m.searchSeq++
	seq, q := m.searchSeq, strings.TrimSpace(m.input.Value())
	if q == "" {
		m.matches, m.sel, m.searching = nil, 0, false
		return nil
	}
	m.searching = true
	return m.searchCmd(seq, q)
}

func (m *Model) refreshSearch() {}

// ensureAudio pede o .aac da categoria atual se ainda não é o carregado.
func (m *Model) ensureAudio() tea.Cmd {
	if m.session == nil {
		return nil
	}
	if m.audioReady && m.loadedCat == m.category {
		return nil
	}
	if m.loadingAudio {
		return nil
	}
	m.loadingAudio = true
	return m.fetchAudio(m.category)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if m.typing {
		return m.handleTypingKey(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Menu):
		m.deps.Player.Pause()
		return m, func() tea.Msg { return BackMsg{} }
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	}
	if m.phase == phaseError && key.Matches(msg, m.keys.Retry) {
		return m, func() tea.Msg { return retryMsg{} }
	}
	if m.phase != phasePlaying && m.phase != phaseFinished {
		return m, nil
	}
	// Troca de categoria (1/2/3) funciona jogando ou no fim.
	switch {
	case key.Matches(msg, m.keys.CatAll):
		return m.switchCategory(0)
	case key.Matches(msg, m.keys.CatRock):
		return m.switchCategory(1)
	case key.Matches(msg, m.keys.CatHipHop):
		return m.switchCategory(2)
	}
	if m.phase != phasePlaying {
		switch {
		case key.Matches(msg, m.keys.Copy):
			m.copied = true
			return m, clip.Copy(m.copyText())
		case key.Matches(msg, m.keys.PlayPause):
			m.togglePlay()
		}
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.PlayPause):
		m.togglePlay()
	case key.Matches(msg, m.keys.Back5):
		m.seekClamped(-seekStep)
	case key.Matches(msg, m.keys.Fwd5):
		m.seekClamped(seekStep)
	case key.Matches(msg, m.keys.Copy):
		// Só copia no fim; fora dele ignora.
		return m, nil
	case key.Matches(msg, m.keys.Skip):
		return m.doSkip()
	case key.Matches(msg, m.keys.Type):
		return m.startTyping("")
	case msg.Text != "" && msg.Mod == 0 && isTypingRune(msg.Text):
		return m.startTyping(msg.Text)
	}
	return m, nil
}

// togglePlay alterna tocar/pausar; se o áudio já chegou ao fim (do round ou da
// faixa), recomeça do início.
func (m *Model) togglePlay() {
	p := m.deps.Player
	if !p.Playing() {
		end := p.Duration()
		if m.session != nil {
			if lim := m.session.AudioLimit(); lim > 0 && (end == 0 || lim < end) {
				end = lim
			}
		}
		if end > 0 && p.Position() >= end {
			p.SeekTo(0)
		}
	}
	p.Toggle()
}

// seekClamped move ±1s sem passar do limite do round.
func (m *Model) seekClamped(delta time.Duration) {
	m.deps.Player.Seek(delta)
	if m.session != nil {
		if lim := m.session.AudioLimit(); lim > 0 && m.deps.Player.Position() > lim {
			m.deps.Player.SeekTo(lim)
			m.deps.Player.Pause()
		}
	}
}

func isTypingRune(s string) bool {
	if s == "" {
		return false
	}
	// 1/2/3 são categorias no modo player, não digitação.
	if s == "1" || s == "2" || s == "3" {
		return false
	}
	r := []rune(s)[0]
	return r >= ' ' && r != 127
}

func (m Model) startTyping(initial string) (Model, tea.Cmd) {
	m.typing, m.notice = true, ""
	m.input.SetValue(initial)
	m.input.CursorEnd()
	m.matches, m.sel, m.searching = nil, 0, false
	if strings.TrimSpace(initial) != "" {
		m.searchSeq++
		seq, q := m.searchSeq, strings.TrimSpace(initial)
		m.searching = true
		return m, tea.Batch(m.input.Focus(), m.searchCmd(seq, q))
	}
	return m, m.input.Focus()
}

func (m Model) stopTyping() Model {
	m.typing = false
	m.input.Blur()
	m.input.Reset()
	m.matches, m.sel, m.searching = nil, 0, false
	return m
}

func (m Model) handleTypingKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Cancel):
		return m.stopTyping(), nil
	case key.Matches(msg, m.keys.Up):
		if m.sel > 0 {
			m.sel--
		}
		return m, nil
	case key.Matches(msg, m.keys.Down):
		if m.sel < len(m.matches)-1 {
			m.sel++
		}
		return m, nil
	case key.Matches(msg, m.keys.Complete):
		if len(m.matches) > 0 {
			m.input.SetValue(m.matches[m.sel].Display())
			m.input.CursorEnd()
			m.matches, m.sel = nil, 0
		}
		return m, nil
	case key.Matches(msg, m.keys.Submit):
		return m.submit()
	}
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.searchSeq++
		seq, q := m.searchSeq, strings.TrimSpace(m.input.Value())
		if q == "" {
			m.matches, m.sel, m.searching = nil, 0, false
			return m, cmd
		}
		m.searching = true
		return m, tea.Batch(cmd, m.searchCmd(seq, q))
	}
	return m, cmd
}

// submit chuta a sugestão selecionada e volta ao modo player.
func (m Model) submit() (Model, tea.Cmd) {
	if len(m.matches) == 0 {
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m.stopTyping(), nil
		}
		// Sem resultado remoto: tenta o texto exato via engine (que busca).
		return m.guessText(text)
	}
	pick := m.matches[m.sel]
	m = m.stopTyping()
	return m.doGuess(pick)
}

func (m Model) guessText(text string) (Model, tea.Cmd) {
	s, cat, timeout := m.session, m.category, m.deps.Timeout
	return m.stopTyping(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		r, err := s.Guess(ctx, text)
		if errors.Is(err, songless.ErrUnknownSong) {
			return guessMsg{category: cat, err: err}
		}
		if err != nil {
			return guessMsg{category: cat, err: err}
		}
		return guessMsg{category: cat, result: r, answer: s.Answer()}
	}
}

func (m Model) doGuess(e songless.Entry) (Model, tea.Cmd) {
	s, cat, timeout := m.session, m.category, m.deps.Timeout
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		r, err := s.GuessEntry(ctx, e)
		return guessMsg{category: cat, entry: &e, result: r, answer: s.Answer(), err: err}
	}
}

func (m Model) doSkip() (Model, tea.Cmd) {
	s, cat, timeout := m.session, m.category, m.deps.Timeout
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		r, err := s.Skip(ctx)
		return guessMsg{category: cat, result: r, answer: s.Answer(), err: err}
	}
}

// applyGuess trata o resultado de um chute ou pulo assíncrono.
func (m Model) applyGuess(msg guessMsg) Model {
	if msg.err != nil {
		if errors.Is(msg.err, songless.ErrUnknownSong) {
			m.notice = "escolha uma música da lista"
		} else {
			m.notice = msg.err.Error()
		}
		return m
	}
	m.notice = msg.result.Hint
	if msg.result.State.Status != game.Playing {
		m.finish()
	}
	m.save()
	return m
}

// switchCategory salva a atual e abre (ou cria) a categoria pedida.
func (m Model) switchCategory(id int) (Model, tea.Cmd) {
	if id == m.category || m.daily.Categories == nil {
		return m, nil
	}
	if _, ok := m.daily.Category(id); !ok {
		m.notice = "categoria indisponível hoje"
		return m, nil
	}
	m.save()
	if s, ok := m.sessions[id]; ok {
		m.category, m.session = id, s
		m.audioReady = m.loadedCat == id
	} else {
		// Cria sem rede: a sessão nova usa o mesmo Daily (provider interno
		// vem da sessão atual).
		s, err := songless.NewSession(m.session.Provider(), m.daily, id)
		if err != nil {
			m.notice = err.Error()
			return m, nil
		}
		if m.sessions == nil {
			m.sessions = map[int]*songless.Session{}
		}
		m.sessions[id], m.category, m.session = s, id, s
		m.audioReady = false
	}
	m = m.stopTyping()
	m.copied = false
	m.restore()
	if m.session.State().Status != game.Playing {
		m.finish()
	} else if m.phase == phaseFinished {
		m.phase = phasePlaying
	}
	return m, m.ensureAudio()
}

// finish entra na fase final.
func (m *Model) finish() {
	m.phase, m.notice = phaseFinished, ""
	m.save()
}

// shareText é o resultado da categoria atual.
func (m Model) shareText() string {
	if m.session == nil {
		return ""
	}
	return m.session.ShareText()
}

// copyText é o que o c copia: com mais de uma categoria concluída hoje, o
// resumo do site com todas (inclusive as salvas de execuções anteriores);
// com uma só, o texto dela.
func (m Model) copyText() string {
	var rows []struct {
		Category string
		Board    []songless.Outcome
	}
	for _, c := range m.daily.Categories {
		if board, ok := m.finishedBoard(c.ID); ok {
			rows = append(rows, struct {
				Category string
				Board    []songless.Outcome
			}{c.Name, board})
		}
	}
	if len(rows) < 2 {
		return m.shareText()
	}
	return songless.ShareMulti(m.daily.Date, rows)
}

// finishedBoard devolve as tentativas de uma categoria já concluída, vindas da
// sessão em memória ou do disco.
func (m Model) finishedBoard(cat int) ([]songless.Outcome, bool) {
	if s := m.sessions[cat]; s != nil && s.State().Status != game.Playing {
		return s.Outcomes(), true
	}
	if m.deps.Store == nil || m.daily.Date == "" {
		return nil, false
	}
	match, ok, err := m.deps.Store.Load(gameID, m.dayKey(cat))
	if err != nil || !ok {
		return nil, false
	}
	var snap songless.Snapshot
	if json.Unmarshal(match.Data, &snap) != nil || !snap.Finished() {
		return nil, false
	}
	return snap.Outcomes, true
}

// dayKey é a chave por categoria: YYYY-MM-DD#cat.
func (m Model) dayKey(cat int) string {
	return m.daily.Date + "#" + itoa(cat)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}

// save grava a categoria atual; falhas viram aviso.
func (m *Model) save() {
	if m.deps.Store == nil || m.session == nil || m.daily.Date == "" {
		return
	}
	snap := m.session.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	err = m.deps.Store.Save(gameID, store.Match{
		Day: m.dayKey(m.category), Number: snap.Tries(),
		Status: snap.Status, Tries: snap.Tries(),
		Finished: snap.Finished(), Data: data,
	})
	if err != nil {
		m.notice = "não deu para salvar a partida: " + err.Error()
	}
}

// restore reaplica a categoria atual salva em disco, se houver.
func (m *Model) restore() {
	if m.deps.Store == nil || m.session == nil || m.daily.Date == "" {
		return
	}
	match, ok, err := m.deps.Store.Load(gameID, m.dayKey(m.category))
	if err != nil {
		m.notice = "histórico ilegível: " + err.Error()
		return
	}
	if !ok {
		return
	}
	var snap songless.Snapshot
	if json.Unmarshal(match.Data, &snap) != nil || m.session.Restore(snap) != nil {
		return
	}
	m.restored = snap.Tries() > 0
}

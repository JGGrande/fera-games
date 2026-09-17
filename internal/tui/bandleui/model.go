// Package bandleui é a tela de jogo do Bandle: player, grade de tentativas,
// dicas e input com autocomplete sobre a engine de internal/games/bandle.
package bandleui

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sahilm/fuzzy"

	"fera-games/internal/audio"
	"fera-games/internal/game"
	"fera-games/internal/games/bandle"
	"fera-games/internal/store"
)

// Deps é o que a tela precisa de fora.
type Deps struct {
	Game    *bandle.Game
	Player  audio.Player
	Lang    string // idioma da dica textual (de, en, es, fr)
	Now     func() time.Time
	Timeout time.Duration // por requisição
	// AudioNotice é mostrado quando o áudio não pôde ser aberto (ex.: --mute).
	AudioNotice string
	// Store persiste a partida do dia e o histórico; nil desliga.
	Store *store.Store
}

const gameID = "bandle"

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
	seekStep     = 5 * time.Second
	tickEvery    = 100 * time.Millisecond
	maxMatches   = 6
	loadingLabel = "baixando o puzzle de hoje…"
)

// Mensagens internas.
type (
	loadedMsg struct{ session *bandle.Session }
	errMsg    struct{ err error }
	trackMsg  struct {
		step  int
		track *audio.Track
		err   error
	}
	tickMsg  struct{}
	retryMsg struct{}
)

// Model é a tela.
type Model struct {
	deps    Deps
	phase   phase
	session *bandle.Session
	err     error

	spinner spinner.Model
	input   textinput.Model
	help    help.Model
	keys    KeyMap

	typing  bool
	matches []string
	sel     int
	notice  string
	copied  bool

	audioStep   int // estágio carregado no player (0 = nenhum)
	loadingStem int // estágio sendo baixado (0 = nenhum)

	stats    bandle.Stats
	restored bool // partida de hoje veio do disco

	width, height int
}

// New cria a tela pronta para Init carregar o puzzle.
func New(d Deps) Model {
	if d.Lang == "" {
		d.Lang = "en"
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Timeout == 0 {
		d.Timeout = 30 * time.Second
	}
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "digite a música…"
	in.CharLimit = 120
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	return Model{deps: d, spinner: sp, input: in, help: help.New(), keys: DefaultKeyMap}
}

// Session expõe a partida (para o app persistir depois; nil enquanto carrega).
func (m Model) Session() *bandle.Session { return m.session }

// Init dispara o carregamento e o tick do player.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.spinner.Tick, tick())
}

func tick() tea.Cmd { return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} }) }

// Resume religa o tick do player ao voltar do menu (o app não encaminha
// mensagens para telas inativas, então a cadeia de ticks para).
func (m Model) Resume() tea.Cmd { return tick() }

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

func (m Model) fetchStem(step int) tea.Cmd {
	s, timeout := m.session, m.deps.Timeout
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		rc, err := s.StemAt(ctx, step)
		if err != nil {
			return trackMsg{step: step, err: err}
		}
		defer rc.Close()
		t, err := audio.Decode(rc)
		return trackMsg{step: step, track: t, err: err}
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
		return m, tick()

	case spinner.TickMsg:
		if m.phase != phaseLoading {
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
		return m, tea.Batch(m.load(), m.spinner.Tick)

	case loadedMsg:
		m.session = msg.session
		m.phase = phasePlaying
		m.restore()
		if m.session.State().Status != game.Playing {
			m.finish()
		}
		return m, m.ensureAudio()

	case trackMsg:
		if msg.step != m.loadingStem {
			return m, nil // resposta atrasada de um estágio que já passou
		}
		m.loadingStem = 0
		if msg.err != nil {
			m.notice = "áudio indisponível: " + msg.err.Error()
			return m, nil
		}
		// Como no site, cada estágio novo recomeça do início com o arranjo
		// completo até ali; o player faz um fade curto para não estalar.
		m.audioStep = msg.step
		m.deps.Player.Restart(msg.track)
		// Se o jogo avançou enquanto baixava, buscar o estágio certo.
		return m, m.ensureAudio()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	if m.typing {
		// Mensagens do cursor (blink) e afins.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ensureAudio pede o mp3 do estágio atual se ainda não é o carregado.
func (m *Model) ensureAudio() tea.Cmd {
	if m.session == nil {
		return nil
	}
	want := m.session.AudioStep()
	if want == 0 || want == m.audioStep || m.loadingStem == want {
		return nil
	}
	m.loadingStem = want
	return m.fetchStem(want)
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
	switch {
	case key.Matches(msg, m.keys.PlayPause):
		m.deps.Player.Toggle()
	case key.Matches(msg, m.keys.Back5):
		m.deps.Player.Seek(-seekStep)
	case key.Matches(msg, m.keys.Fwd5):
		m.deps.Player.Seek(seekStep)
	case m.phase == phaseFinished && key.Matches(msg, m.keys.Copy):
		m.copied = true
		return m, tea.SetClipboard(m.shareText())
	case m.phase == phasePlaying && key.Matches(msg, m.keys.Skip):
		return m.apply(m.session.Skip(context.Background()))
	case m.phase == phasePlaying && key.Matches(msg, m.keys.Type):
		return m.startTyping("")
	case m.phase == phasePlaying && msg.Text != "" && msg.Mod == 0:
		// Qualquer letra começa a digitar, sem exigir enter antes.
		return m.startTyping(msg.Text)
	}
	return m, nil
}

func (m Model) startTyping(initial string) (Model, tea.Cmd) {
	m.typing, m.notice = true, ""
	m.input.SetValue(initial)
	m.input.CursorEnd()
	m.refreshMatches()
	return m, m.input.Focus()
}

func (m Model) stopTyping() Model {
	m.typing = false
	m.input.Blur()
	m.input.Reset()
	m.matches, m.sel = nil, 0
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
			m.input.SetValue(m.matches[m.sel])
			m.input.CursorEnd()
			m.refreshMatches()
		}
		return m, nil
	case key.Matches(msg, m.keys.Submit):
		return m.submit()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.refreshMatches()
	return m, cmd
}

// submit chuta a sugestão selecionada (ou o texto exato) e volta ao modo player.
func (m Model) submit() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" && len(m.matches) == 0 {
		return m.stopTyping(), nil
	}
	if _, exact := m.session.Catalog().Find(text); !exact && len(m.matches) > 0 {
		text = m.matches[m.sel]
	}
	r, err := m.session.Guess(context.Background(), text)
	if errors.Is(err, bandle.ErrUnknownSong) {
		m.notice = "escolha uma música da lista"
		return m, nil
	}
	m = m.stopTyping()
	return m.apply(r, err)
}

// apply trata o resultado de um chute ou pulo.
func (m Model) apply(r game.Result, err error) (Model, tea.Cmd) {
	if err != nil {
		m.notice = err.Error()
		return m, nil
	}
	m.notice = r.Hint
	if r.State.Status != game.Playing {
		m.finish()
	}
	m.save()
	return m, m.ensureAudio()
}

// finish entra na fase final e carrega as estatísticas para o compartilhamento.
func (m *Model) finish() {
	m.phase, m.notice = phaseFinished, ""
	if m.deps.Store != nil {
		m.save() // a partida terminada precisa estar no histórico antes de contar
		if st, err := m.deps.Store.Stats(gameID, bandle.FormatDate(m.deps.Now())); err == nil {
			m.stats = bandle.Stats{Played: st.Played, Won: st.Won, CurrentStreak: st.CurrentStreak, MaxStreak: st.MaxStreak}
		}
	}
}

// shareText é o resultado com as estatísticas do histórico.
func (m Model) shareText() string { return m.session.ShareTextWith(m.stats) }

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
	match, ok, err := m.deps.Store.Load(gameID, bandle.FormatDate(m.deps.Now()))
	if err != nil {
		m.notice = "histórico ilegível: " + err.Error()
		return
	}
	if !ok {
		return
	}
	var snap bandle.Snapshot
	if json.Unmarshal(match.Data, &snap) != nil || m.session.Restore(snap) != nil {
		return // estado de outro puzzle ou corrompido: começa do zero
	}
	m.restored = snap.Tries() > 0
}

// refreshMatches recalcula as sugestões para o texto atual.
func (m *Model) refreshMatches() {
	q := strings.TrimSpace(m.input.Value())
	if q == "" || m.session == nil {
		m.matches, m.sel = nil, 0
		return
	}
	m.matches = suggest(q, m.session.Catalog().Titles(), maxMatches)
	if m.sel >= len(m.matches) {
		m.sel = 0
	}
}

// suggest ordena: primeiro quem contém o texto literalmente (o mais curto
// antes, que costuma ser o mais específico), depois o fuzzy para completar.
func suggest(q string, titles []string, limit int) []string {
	lq := strings.ToLower(q)
	var exact []string
	for _, t := range titles {
		if strings.Contains(strings.ToLower(t), lq) {
			exact = append(exact, t)
		}
	}
	sort.SliceStable(exact, func(i, j int) bool { return len(exact[i]) < len(exact[j]) })
	out := exact[:min(len(exact), limit)]
	if len(out) < limit {
		seen := make(map[string]bool, len(out))
		for _, t := range out {
			seen[t] = true
		}
		for _, f := range fuzzy.Find(q, titles) {
			if len(out) >= limit {
				break
			}
			if !seen[f.Str] {
				out = append(out, f.Str)
			}
		}
	}
	return out
}

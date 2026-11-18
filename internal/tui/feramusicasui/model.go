// Package feramusicasui é a tela de jogo do Fera Músicas: player com limite
// por round, grade de 5 tentativas, autocomplete local e abas com as 3
// categorias do dia. O cadastro fica no Admin (feramusicasadmin), fora do
// jogo, porque a lista de músicas entrega as respostas.
package feramusicasui

import (
	"context"
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
	"fera-games/internal/games/feramusicas"
	"fera-games/internal/tui/clip"
	"fera-games/internal/tui/suggest"
)

// Deps é o que a tela precisa de fora.
type Deps struct {
	Game   *feramusicas.Game
	Player audio.Player
	Now    func() time.Time
	// Timeout vale para cada download de áudio (yt-dlp + ffmpeg); o catálogo
	// e a engine usam um prazo curto fixo.
	Timeout time.Duration
	// AudioNotice é mostrado quando o áudio não pôde ser aberto (ex.: --mute).
	AudioNotice string
	// CatalogNotice é mostrado quando o catálogo está vazio (ex.: "só em memória").
	CatalogNotice string
}

// BackMsg pede ao app para voltar ao menu.
type BackMsg struct{}

type screen int

const (
	screenLoading screen = iota // lendo o catálogo
	screenGame
	screenEmpty // catálogo sem músicas: cadastre no Admin
	screenError
)

const (
	seekStep       = time.Second
	tickEvery      = 100 * time.Millisecond
	maxMatches     = 6
	catalogTimeout = 10 * time.Second
	loadingLabel   = "abrindo o catálogo…"
)

// Mensagens internas.
type (
	catalogMsg struct {
		songs []feramusicas.Song
		cats  []string
		err   error
	}
	sessionMsg struct {
		key     string
		session *feramusicas.Session
		err     error
	}
	trackMsg struct {
		session *feramusicas.Session
		track   *audio.Track
		err     error
	}
	tickMsg struct{}
)

// Model é a tela.
type Model struct {
	deps   Deps
	screen screen
	err    error // erro da tela screenError

	// Catálogo.
	cats  []string // todas as categorias
	daily []string // as categorias do dia (abas), ver feramusicas.DailyCategories

	// Jogo: uma sessão por categoria (a música do dia fica congelada nela).
	sessions map[string]*feramusicas.Session
	cur      string // CategoryKey da categoria aberta
	session  *feramusicas.Session
	starting bool // esperando a sessão da categoria

	// Áudio, por sessão e não por categoria: se a categoria some e volta, a
	// sessão nova sorteia outra música, e um download atrasado da antiga não
	// pode ocupar o lugar dela. O player tem uma faixa por vez (inPlayer).
	tracks    map[*feramusicas.Session]*audio.Track // faixas inteiras já decodificadas
	loading   map[*feramusicas.Session]bool         // downloads em andamento
	audioErrs map[*feramusicas.Session]string
	inPlayer  *feramusicas.Session // de quem é a faixa no player

	// Digitação do chute.
	typing  bool
	input   textinput.Model
	matches []string
	sel     int

	notice string
	copied bool

	spinner spinner.Model
	help    help.Model
	keys    KeyMap

	width, height int
}

// New cria a tela pronta para Init carregar o catálogo.
func New(d Deps) Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Timeout == 0 {
		d.Timeout = 60 * time.Second
	}
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "busque a música…"
	in.CharLimit = feramusicas.MaxNameLen

	m := Model{
		deps: d, screen: screenLoading, input: in,
		sessions: map[string]*feramusicas.Session{},
		tracks:   map[*feramusicas.Session]*audio.Track{}, loading: map[*feramusicas.Session]bool{},
		audioErrs: map[*feramusicas.Session]string{},
		spinner:   spinner.New(spinner.WithSpinner(spinner.Dot)), help: help.New(), keys: DefaultKeyMap,
	}
	return m
}

// Session expõe a partida atual (nil enquanto carrega).
func (m Model) Session() *feramusicas.Session { return m.session }

// Init carrega o catálogo e liga o tick do player.
func (m Model) Init() tea.Cmd { return tea.Batch(m.reload(), m.spinner.Tick, tick()) }

func tick() tea.Cmd { return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} }) }

// Resume religa o tick do player e recarrega o catálogo ao voltar do menu: o
// Admin (ou outra pessoa, no Firestore) pode ter mexido nele. As partidas em
// andamento continuam; só a lista de chutes e as categorias do dia mudam.
func (m Model) Resume() tea.Cmd { return tea.Batch(tick(), m.reload()) }

// --- comandos assíncronos ---------------------------------------------------

func (m Model) reload() tea.Cmd {
	cat := m.deps.Game.Catalog()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), catalogTimeout)
		defer cancel()
		songs, err := cat.List(ctx)
		if err != nil {
			return catalogMsg{err: err}
		}
		cats, err := cat.Categories(ctx)
		return catalogMsg{songs: songs, cats: cats, err: err}
	}
}

func (m Model) startSession(category string) tea.Cmd {
	g, day := m.deps.Game, m.deps.Now()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), catalogTimeout)
		defer cancel()
		s, err := g.Start(ctx, day, category)
		return sessionMsg{key: feramusicas.CategoryKey(category), session: s, err: err}
	}
}

func (m Model) fetchAudio(s *feramusicas.Session) tea.Cmd {
	timeout := m.deps.Timeout
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		rc, err := s.Audio(ctx)
		if err != nil {
			return trackMsg{session: s, err: err}
		}
		defer rc.Close()
		t, err := audio.DecodePCM(rc)
		return trackMsg{session: s, track: t, err: err}
	}
}

// --- Update -----------------------------------------------------------------

// Update trata mensagens.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width - 4) // o Frame tira 2 colunas de cada lado
		w := min(msg.Width-8, 70)
		m.input.SetWidth(w)
		return m, nil

	case tickMsg:
		// Só redesenha (posição e barra). O corte no limite do round é exato
		// porque o player recebe a faixa já cortada (loadRound).
		return m, tick()

	case spinner.TickMsg:
		if m.screen != screenLoading && !m.starting && len(m.loading) == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case catalogMsg:
		return m.onCatalog(msg)

	case sessionMsg:
		return m.onSession(msg)

	case trackMsg:
		return m.onTrack(msg), nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Outras mensagens (blink do cursor) vão para o input do chute.
	if m.typing {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// onCatalog recebe a lista atual do catálogo.
func (m Model) onCatalog(msg catalogMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.screen, m.err = screenError, msg.err
		return m, nil
	}
	m.cats = msg.cats
	m.daily = feramusicas.DailyCategories(m.deps.Now(), m.cats)

	// Sessões de categorias que saíram do dia (esvaziadas, ou trocadas por uma
	// categoria nova que entrou no sorteio) não voltam com a música velha. As
	// que seguem passam a aceitar como chute as músicas cadastradas depois de
	// abertas (só a resposta do dia fica congelada).
	live := map[string]bool{}
	for _, c := range m.daily {
		live[feramusicas.CategoryKey(c)] = true
	}
	for k, s := range m.sessions {
		if !live[k] {
			delete(m.sessions, k)
			m.forgetAudio(s)
			continue
		}
		s.UseSongs(msg.songs)
	}
	if m.typing {
		m.refreshMatches()
	}

	// Categoria pode ter sumido do dia, ou o catálogo ter ganhado músicas.
	return m.enterGame()
}

// enterGame abre o jogo na categoria atual (ou na primeira); sem músicas,
// mostra a tela que manda cadastrar no Admin.
func (m Model) enterGame() (Model, tea.Cmd) {
	m.err = nil
	if len(m.daily) == 0 {
		m = m.stopTyping()
		m.screen, m.session, m.cur, m.starting, m.notice = screenEmpty, nil, "", false, ""
		return m, nil
	}
	if m.screen != screenGame {
		m.typing, m.notice = false, ""
	}
	m.screen = screenGame
	if !m.isDaily(m.cur) {
		m.cur = feramusicas.CategoryKey(m.daily[0])
	}
	return m.openCategory(m.cur)
}

// isDaily diz se a categoria é uma das do dia.
func (m Model) isDaily(key string) bool {
	for _, c := range m.daily {
		if feramusicas.CategoryKey(c) == key {
			return key != ""
		}
	}
	return false
}

func (m Model) categoryName(key string) string {
	for _, c := range m.cats {
		if feramusicas.CategoryKey(c) == key {
			return c
		}
	}
	return key
}

// openCategory ativa a categoria: usa a sessão em memória ou pede uma nova.
func (m Model) openCategory(key string) (Model, tea.Cmd) {
	m.cur, m.copied = key, false
	if s, ok := m.sessions[key]; ok {
		m.session, m.starting = s, false
		return m, m.ensureAudio()
	}
	m.session, m.starting = nil, true
	return m, tea.Batch(m.startSession(m.categoryName(key)), m.spinner.Tick)
}

func (m Model) onSession(msg sessionMsg) (Model, tea.Cmd) {
	if msg.key != m.cur {
		return m, nil // resposta atrasada de outra categoria
	}
	m.starting = false
	if msg.err != nil {
		m.notice = "não deu para abrir a categoria: " + msg.err.Error()
		return m, nil
	}
	m.sessions[msg.key] = msg.session
	m.session = msg.session
	return m, m.ensureAudio()
}

// ensureAudio garante a faixa da sessão atual no player: já carregada, já
// decodificada (só recarrega) ou ainda por baixar. Um download por sessão:
// trocar de aba no meio dele não dispara outro.
func (m *Model) ensureAudio() tea.Cmd {
	s := m.session
	if s == nil || m.inPlayer == s {
		return nil
	}
	if _, ok := m.tracks[s]; ok {
		m.inPlayer = s
		m.loadRound()
		return nil
	}
	if m.loading[s] {
		return nil
	}
	m.loading[s] = true
	delete(m.audioErrs, s)
	return tea.Batch(m.fetchAudio(s), m.spinner.Tick)
}

func (m Model) onTrack(msg trackMsg) Model {
	s := msg.session
	delete(m.loading, s)
	if !m.isOpen(s) {
		return m // categoria removida enquanto baixava
	}
	if msg.err != nil {
		m.audioErrs[s] = friendlyAudio(msg.err)
		return m
	}
	m.tracks[s] = msg.track
	if s == m.session && m.screen == screenGame {
		m.inPlayer = s
		m.loadRound()
	}
	return m
}

// isOpen diz se a sessão ainda é a de alguma categoria.
func (m Model) isOpen(s *feramusicas.Session) bool {
	for _, x := range m.sessions {
		if x == s {
			return true
		}
	}
	return false
}

// forgetAudio descarta o áudio de uma sessão que deixou de existir.
func (m *Model) forgetAudio(s *feramusicas.Session) {
	delete(m.tracks, s)
	delete(m.loading, s)
	delete(m.audioErrs, s)
	if m.inPlayer == s {
		m.deps.Player.Pause()
		m.inPlayer = nil
	}
}

// loadRound põe no player o trecho do round atual, cortado no limite. Como o
// player para sozinho no fim da faixa, o corte é exato: não depende do tick
// de 100 ms nem do buffer do dispositivo (com Pause, o que já estava
// enfileirado ainda tocaria, e o round 1 de 0,13 s chegaria a ~0,3 s).
// Partida em andamento toca do início; terminada, só carrega o trecho todo.
func (m *Model) loadRound() {
	full := m.tracks[m.inPlayer]
	if full == nil || m.inPlayer != m.session {
		return
	}
	cut := full.Head(m.session.AudioLimit())
	if m.session.State().Status != game.Playing {
		m.deps.Player.Load(cut)
		return
	}
	m.deps.Player.Restart(cut)
}

// --- teclas -----------------------------------------------------------------

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	switch m.screen {
	case screenGame:
		if m.typing {
			return m.handleTypingKey(msg)
		}
		return m.handleGameKey(msg)
	case screenEmpty:
		if key.Matches(msg, m.keys.Menu) {
			return m, m.back()
		}
	case screenError:
		switch {
		case key.Matches(msg, m.keys.Retry):
			m.screen = screenLoading
			return m, tea.Batch(m.reload(), m.spinner.Tick)
		case key.Matches(msg, m.keys.Menu):
			return m, m.back()
		}
	case screenLoading:
		if key.Matches(msg, m.keys.Menu) {
			return m, m.back()
		}
	}
	return m, nil
}

// back pausa o áudio e volta ao menu do hub.
func (m Model) back() tea.Cmd {
	m.deps.Player.Pause()
	return func() tea.Msg { return BackMsg{} }
}

func (m Model) handleGameKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Menu):
		return m, m.back()
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	}
	if i, ok := categoryIndex(msg); ok {
		if i < len(m.daily) {
			k := feramusicas.CategoryKey(m.daily[i])
			if k != m.cur {
				m.deps.Player.Pause()
				m.notice = ""
				return m.openCategory(k)
			}
		}
		return m, nil
	}
	if m.session == nil {
		return m, nil
	}
	finished := m.session.State().Status != game.Playing
	switch {
	case key.Matches(msg, m.keys.PlayPause):
		m.togglePlay()
	case finished && key.Matches(msg, m.keys.Copy):
		m.copied = true
		return m, clip.Copy(m.copyText())
	case finished:
		// Terminada a partida só sobra tocar o trecho inteiro e copiar.
	case key.Matches(msg, m.keys.Back):
		m.seek(-seekStep)
	case key.Matches(msg, m.keys.Fwd):
		m.seek(seekStep)
	case key.Matches(msg, m.keys.Skip):
		res, err := m.session.Skip(context.Background())
		return m.afterGuess(res, err), nil
	case key.Matches(msg, m.keys.Type):
		return m.startTyping()
	}
	return m, nil
}

// finishedToday são os resultados das músicas do dia já concluídas, na ordem das abas.
func (m Model) finishedToday() []feramusicas.DailyResult {
	var out []feramusicas.DailyResult
	for _, c := range m.daily {
		s := m.sessions[feramusicas.CategoryKey(c)]
		if s == nil {
			continue
		}
		if st := s.State().Status; st != game.Playing {
			out = append(out, feramusicas.DailyResult{Category: s.Category(), Won: st == game.Won, Board: s.Outcomes()})
		}
	}
	return out
}

// copyText é o que o c copia: com mais de uma música do dia concluída, o resumo
// de todas; com uma só, o resultado dela.
func (m Model) copyText() string {
	if done := m.finishedToday(); len(done) > 1 {
		return feramusicas.ShareDaily(m.session.Date(), len(m.daily), done)
	}
	return m.session.ShareText()
}

// categoryIndex traduz as teclas 1..9 em índice 0..8 (só 1..3 têm aba).
func categoryIndex(msg tea.KeyPressMsg) (int, bool) {
	s := msg.String()
	if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		return int(s[0] - '1'), true
	}
	return 0, false
}

// togglePlay alterna tocar/pausar; se o trecho do round já terminou, recomeça.
func (m *Model) togglePlay() {
	if m.inPlayer != m.session || m.session == nil {
		return
	}
	p := m.deps.Player
	if !p.Playing() && p.Position() >= p.Duration() {
		p.SeekTo(0)
	}
	p.Toggle()
}

// seek move ±1 s; o player já prende no fim do trecho cortado do round.
func (m *Model) seek(delta time.Duration) {
	if m.inPlayer != m.session || m.session == nil {
		return
	}
	m.deps.Player.Seek(delta)
}

// afterGuess aplica o resultado de um chute ou pulo: o round seguinte toca do
// início, com o trecho maior; ao fim da partida o player carrega o trecho
// inteiro, pausado.
func (m Model) afterGuess(res game.Result, err error) Model {
	if err != nil {
		if errors.Is(err, feramusicas.ErrUnknownSong) {
			m.notice = "escolha uma música da lista"
		} else {
			m.notice = err.Error()
		}
		return m
	}
	m.notice = res.Hint
	m.loadRound()
	return m
}

// --- digitação do chute -----------------------------------------------------

func (m Model) startTyping() (Model, tea.Cmd) {
	m.typing, m.notice = true, ""
	m.input.Reset()
	m.matches, m.sel = nil, 0
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
		m.refreshMatches()
	}
	return m, cmd
}

// refreshMatches recalcula as sugestões para o texto atual (tudo local).
func (m *Model) refreshMatches() {
	q := strings.TrimSpace(m.input.Value())
	if q == "" || m.session == nil {
		m.matches, m.sel = nil, 0
		return
	}
	m.matches = suggest.Match(q, m.session.Names(), maxMatches)
	if m.sel >= len(m.matches) {
		m.sel = 0
	}
}

// submit chuta a sugestão selecionada, ou o texto digitado se não houver.
func (m Model) submit() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if len(m.matches) > 0 {
		text = m.matches[m.sel]
	}
	if text == "" {
		return m.stopTyping(), nil
	}
	res, err := m.session.Guess(context.Background(), text)
	if errors.Is(err, feramusicas.ErrUnknownSong) {
		m.notice = "escolha uma música da lista" // segue digitando
		return m, nil
	}
	return m.stopTyping().afterGuess(res, err), nil
}

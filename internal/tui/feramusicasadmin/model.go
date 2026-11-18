// Package feramusicasadmin é o menu administrativo do Fera Músicas (tecla a no
// menu inicial): lista todas as músicas cadastradas e cadastra, edita e remove
// músicas, com prévia do trecho e confirmação antes de salvar. Fica fora do
// jogo porque a lista entrega as respostas.
package feramusicasadmin

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"fera-games/internal/audio"
	"fera-games/internal/games/feramusicas"
)

// Deps é o que a tela precisa de fora.
type Deps struct {
	Game *feramusicas.Game
	Now  func() time.Time
	// CatalogNotice é mostrado no topo da lista (ex.: "só em memória").
	CatalogNotice string
	// Player toca a prévia do trecho no cadastro. Deve ser um player só do
	// Admin, para a prévia não mexer no trecho que o jogo deixou carregado.
	// nil = mudo.
	Player audio.Player
	// AudioNotice é mostrado na prévia quando não há som (ex.: --mute).
	AudioNotice string
	// Timeout vale para cada download de prévia (padrão 60 s).
	Timeout time.Duration
}

// BackMsg pede ao app para voltar ao menu.
type BackMsg struct{}

type screen int

const (
	screenLoading screen = iota
	screenList
	screenForm
	screenError
)

const (
	catalogTimeout = 10 * time.Second
	tickEvery      = 100 * time.Millisecond
)

// Campos do formulário de cadastro.
const (
	fName = iota
	fCategory
	fURL
	fStart // opcional: de onde o trecho começa no vídeo
	numFields
)

type (
	catalogMsg struct {
		songs []feramusicas.Song
		cats  []string
		err   error
	}
	saveMsg struct {
		song   feramusicas.Song
		edited bool // Update (edição) em vez de Add (cadastro)
		err    error
	}
	removeMsg  struct{ err error }
	previewMsg struct {
		seq   int
		key   string
		start time.Duration
		track *audio.Track
		err   error
	}
	tickMsg struct{}
)

// Model é a tela.
type Model struct {
	deps   Deps
	screen screen
	err    error

	songs []feramusicas.Song // ordenadas por categoria e nome (ver sortSongs)
	cats  []string

	cursor     int
	confirmDel bool
	selectID   string // música para pôr o cursor depois do próximo reload (a editada)

	fields  [numFields]textinput.Model
	focus   int
	formErr error
	saving  bool
	formMsg string
	// editing é a música sendo editada; nil = cadastro de uma nova.
	editing *feramusicas.Song
	// confirmSave: os campos foram validados e a tela pergunta "Salvar…?".
	confirmSave bool
	toSave      feramusicas.Song

	// Prévia do trecho no cadastro.
	previewing   bool   // baixando
	previewSeq   int    // descarta respostas de prévias canceladas
	previewKey   string // vídeo+início da faixa em previewTrack
	previewStart time.Duration
	previewTrack *audio.Track
	previewErr   string
	ticking      bool

	notice  string
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
	if d.Player == nil {
		d.Player = audio.NewSilent()
	}
	if d.Timeout == 0 {
		d.Timeout = 60 * time.Second
	}
	m := Model{
		deps: d, screen: screenLoading,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)), help: help.New(), keys: DefaultKeyMap,
	}
	labels := [numFields]struct {
		placeholder string
		limit       int
	}{
		fName:     {"Artista - Música", feramusicas.MaxNameLen},
		fCategory: {"Rock, Pop, Anos 80…", feramusicas.MaxCategoryLen},
		fURL:      {"https://www.youtube.com/watch?v=…", 300},
		fStart:    {"opcional: 45 ou 1:30", 12},
	}
	for i := range m.fields {
		f := textinput.New()
		f.Prompt = "> "
		f.Placeholder, f.CharLimit = labels[i].placeholder, labels[i].limit
		m.fields[i] = f
	}
	return m
}

// Init carrega o catálogo.
func (m Model) Init() tea.Cmd { return tea.Batch(m.reload(), m.spinner.Tick) }

// Resume recarrega ao voltar do menu: outra pessoa pode ter mexido no catálogo
// (Firestore), e o dia pode ter virado.
func (m Model) Resume() tea.Cmd { return m.reload() }

// reload lê o catálogo fora da thread da tela.
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

// saveSong cadastra (editID vazio) ou edita a música.
func (m Model) saveSong(editID string, s feramusicas.Song) tea.Cmd {
	cat := m.deps.Game.Catalog()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), catalogTimeout)
		defer cancel()
		if editID != "" {
			got, err := cat.Update(ctx, editID, s)
			return saveMsg{song: got, edited: true, err: err}
		}
		got, err := cat.Add(ctx, s)
		return saveMsg{song: got, err: err}
	}
}

// previewKey identifica a faixa da prévia: o mesmo vídeo com outro início é
// outro trecho.
func previewKey(s feramusicas.Song) string {
	return s.VideoID + "@" + s.Start.String()
}

func (m Model) fetchPreview(seq int, song feramusicas.Song) tea.Cmd {
	g, timeout := m.deps.Game, m.deps.Timeout
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		msg := previewMsg{seq: seq, key: previewKey(song), start: song.Start}
		rc, err := g.Preview(ctx, song)
		if err != nil {
			msg.err = err
			return msg
		}
		defer rc.Close()
		msg.track, msg.err = audio.DecodePCM(rc)
		return msg
	}
}

func tick() tea.Cmd { return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} }) }

// startTicking liga o redesenho do progresso da prévia, se ainda não estiver ligado.
func (m *Model) startTicking() tea.Cmd {
	if m.ticking {
		return nil
	}
	m.ticking = true
	return tick()
}

// formSong lê o link e o início do formulário, como a prévia e o cadastro usam.
func (m Model) formSong() (feramusicas.Song, error) {
	id, err := feramusicas.ParseVideoID(m.fields[fURL].Value())
	if err != nil {
		return feramusicas.Song{}, err
	}
	start, err := feramusicas.ParseStart(m.fields[fStart].Value())
	if err != nil {
		return feramusicas.Song{}, err
	}
	return feramusicas.Song{VideoID: id, Start: start}, nil
}

// togglePreview toca (ou para) o trecho do link a partir do início digitado:
// o mesmo que o jogo vai usar, para conferir o corte antes de salvar.
func (m Model) togglePreview() (Model, tea.Cmd) {
	p := m.deps.Player
	if p.Playing() {
		p.Pause()
		return m, nil
	}
	if m.previewing {
		return m, nil // já está baixando
	}
	song, err := m.formSong()
	if err != nil {
		m.formErr = err
		var ve *feramusicas.ValidationError
		if errors.As(err, &ve) {
			return m, m.focusField(fieldIndex(ve.Field))
		}
		return m, nil
	}
	m.formErr, m.previewErr = nil, ""
	if previewKey(song) == m.previewKey && m.previewTrack != nil {
		p.Restart(m.previewTrack) // já baixada: toca de novo do começo
		return m, m.startTicking()
	}
	m.previewing = true
	m.previewSeq++
	return m, tea.Batch(m.fetchPreview(m.previewSeq, song), m.spinner.Tick)
}

// stopPreview para o som e descarta uma prévia que ainda esteja baixando.
func (m *Model) stopPreview() {
	m.deps.Player.Pause()
	if m.previewing {
		m.previewing = false
		m.previewSeq++
	}
}

func (m Model) onPreview(msg previewMsg) (Model, tea.Cmd) {
	if msg.seq != m.previewSeq {
		return m, nil // prévia cancelada
	}
	m.previewing = false
	if msg.err != nil {
		m.previewErr = friendlyPreview(msg.err)
		return m, nil
	}
	m.previewKey, m.previewStart, m.previewTrack = msg.key, msg.start, msg.track
	if m.screen != screenForm {
		return m, nil
	}
	m.deps.Player.Restart(msg.track)
	return m, m.startTicking()
}

// friendlyPreview diz o que fazer quando a prévia não baixa.
func friendlyPreview(err error) string {
	switch {
	case errors.Is(err, feramusicas.ErrMissingTools):
		return "faltam o yt-dlp e/ou o ffmpeg: instale (veja o README) e reabra o programa"
	case errors.Is(err, feramusicas.ErrVideoUnavailable):
		return "vídeo indisponível: confira o link"
	case errors.Is(err, feramusicas.ErrAgeRestricted):
		return "vídeo com restrição de idade: escolha outro link"
	case errors.Is(err, feramusicas.ErrStartPastEnd):
		return "não há áudio a partir desse início: o vídeo é mais curto"
	case errors.Is(err, feramusicas.ErrNoAudio):
		return "o vídeo não tem áudio: escolha outro link"
	case errors.Is(err, feramusicas.ErrBotCheck):
		return "o YouTube pediu verificação; atualize o yt-dlp (yt-dlp -U)"
	case errors.Is(err, context.DeadlineExceeded):
		return "o download demorou demais; tente de novo"
	}
	first, _, _ := strings.Cut(strings.TrimSpace(err.Error()), "\n")
	return "download falhou: " + first
}

func (m Model) removeSong(id string) tea.Cmd {
	cat := m.deps.Game.Catalog()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), catalogTimeout)
		defer cancel()
		return removeMsg{err: cat.Remove(ctx, id)}
	}
}

// Update trata mensagens.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width - 4) // o Frame tira 2 colunas de cada lado
		w := min(msg.Width-8, 70)
		for i := range m.fields {
			m.fields[i].SetWidth(w - 14)
		}
		return m, nil

	case spinner.TickMsg:
		if m.screen != screenLoading && !m.previewing {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case catalogMsg:
		if msg.err != nil {
			m.screen, m.err = screenError, msg.err
			return m, nil
		}
		m.songs, m.cats = sortSongs(msg.songs), msg.cats
		if m.selectID != "" {
			for i, s := range m.songs {
				if s.ID == m.selectID {
					m.cursor = i
				}
			}
			m.selectID = ""
		}
		m.cursor = max(0, min(m.cursor, len(m.songs)-1))
		if m.screen == screenLoading || m.screen == screenError {
			m.screen = screenList
		}
		return m, nil

	case saveMsg:
		return m.onSaved(msg)

	case previewMsg:
		return m.onPreview(msg)

	case tickMsg:
		// Redesenha o progresso enquanto a prévia toca; parada, desliga.
		if m.deps.Player.Playing() {
			return m, tick()
		}
		m.ticking = false
		return m, nil

	case removeMsg:
		m.confirmDel = false
		if msg.err != nil && !errors.Is(msg.err, feramusicas.ErrNotFound) {
			m.notice = "não deu para remover: " + msg.err.Error()
		}
		return m, m.reload()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	// Outras mensagens (blink do cursor) vão para o campo em foco.
	if m.screen == screenForm {
		var cmd tea.Cmd
		m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) back() tea.Cmd {
	m.stopPreview()
	return func() tea.Msg { return BackMsg{} }
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	switch m.screen {
	case screenList:
		return m.handleListKey(msg)
	case screenForm:
		return m.handleFormKey(msg)
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

// --- lista ------------------------------------------------------------------

// sortSongs ordena por categoria (sem caixa) e depois por nome.
func sortSongs(in []feramusicas.Song) []feramusicas.Song {
	out := append([]feramusicas.Song(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		ci, cj := feramusicas.CategoryKey(out[i].Category), feramusicas.CategoryKey(out[j].Category)
		if ci != cj {
			return ci < cj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (m Model) handleListKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.confirmDel {
		switch {
		case key.Matches(msg, m.keys.Confirm):
			if m.cursor < len(m.songs) {
				return m, m.removeSong(m.songs[m.cursor].ID)
			}
			m.confirmDel = false
		case key.Matches(msg, m.keys.Deny):
			m.confirmDel = false
		}
		return m, nil
	}
	m.notice = ""
	switch {
	case key.Matches(msg, m.keys.Up):
		m.cursor = max(m.cursor-1, 0)
	case key.Matches(msg, m.keys.Down):
		m.cursor = min(m.cursor+1, max(len(m.songs)-1, 0))
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	case key.Matches(msg, m.keys.Add):
		return m.openForm()
	case key.Matches(msg, m.keys.Edit):
		if m.cursor < len(m.songs) {
			return m.openEdit(m.songs[m.cursor])
		}
	case key.Matches(msg, m.keys.Delete):
		m.confirmDel = len(m.songs) > 0
	case key.Matches(msg, m.keys.Menu):
		return m, m.back()
	}
	return m, nil
}

// --- cadastro ---------------------------------------------------------------

// openForm abre o cadastro de uma música nova. O rascunho de um cadastro
// anterior fica; o de uma edição, não.
func (m Model) openForm() (Model, tea.Cmd) {
	if m.editing != nil {
		m.clearFields()
		m.editing = nil
	}
	m.screen, m.formErr, m.formMsg, m.saving, m.previewErr, m.confirmSave = screenForm, nil, "", false, "", false
	return m, m.focusField(fName)
}

// openEdit abre o formulário preenchido com a música, para editar.
func (m Model) openEdit(s feramusicas.Song) (Model, tea.Cmd) {
	song := s
	m.editing = &song
	m.fields[fName].SetValue(s.Name)
	m.fields[fCategory].SetValue(s.Category)
	m.fields[fURL].SetValue(s.URL)
	m.fields[fStart].SetValue(feramusicas.FormatStart(s.Start))
	for i := range m.fields {
		m.fields[i].CursorEnd()
	}
	m.screen, m.formErr, m.formMsg, m.saving, m.previewErr, m.confirmSave = screenForm, nil, "", false, "", false
	return m, m.focusField(fName)
}

// clearFields esvazia os quatro campos.
func (m *Model) clearFields() {
	for i := range m.fields {
		m.fields[i].Reset()
	}
}

// focusField dá foco a um campo e tira dos outros.
func (m *Model) focusField(i int) tea.Cmd {
	m.focus = (i%numFields + numFields) % numFields
	var cmd tea.Cmd
	for j := range m.fields {
		if j == m.focus {
			cmd = m.fields[j].Focus()
		} else {
			m.fields[j].Blur()
		}
	}
	return cmd
}

func (m Model) handleFormKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.confirmSave {
		return m.handleConfirmSave(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Close):
		for i := range m.fields {
			m.fields[i].Blur()
		}
		m.stopPreview()
		if m.editing != nil { // desistir de editar descarta as mudanças
			m.clearFields()
			m.editing = nil
		}
		m.screen = screenList
		return m, nil
	case key.Matches(msg, m.keys.Preview):
		return m.togglePreview()
	case key.Matches(msg, m.keys.Clear):
		m.stopPreview()
		m.clearFields()
		m.formErr, m.formMsg, m.previewErr = nil, "", ""
		return m, m.focusField(fName)
	case key.Matches(msg, m.keys.Save):
		if m.saving {
			return m, nil
		}
		return m.askSave()
	case key.Matches(msg, m.keys.NextField):
		if m.focus == fCategory && m.completeCategory() {
			return m, nil
		}
		return m, m.focusField(m.focus + 1)
	case key.Matches(msg, m.keys.PrevField):
		return m, m.focusField(m.focus - 1)
	}
	var cmd tea.Cmd
	m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
	m.formErr, m.formMsg = nil, ""
	return m, cmd
}

// askSave valida os campos (como o catálogo vai validar) e, se estiverem
// certos, pede confirmação antes de gravar.
func (m Model) askSave() (Model, tea.Cmd) {
	m.stopPreview()
	start, err := feramusicas.ParseStart(m.fields[fStart].Value())
	if err != nil {
		m.formErr, m.formMsg = err, ""
		return m, m.focusField(fStart)
	}
	song, err := feramusicas.PrepareSong(feramusicas.Song{
		Name: m.fields[fName].Value(), Category: m.fields[fCategory].Value(), URL: m.fields[fURL].Value(),
		Start: start,
	})
	if err != nil {
		m.formErr, m.formMsg = err, ""
		var ve *feramusicas.ValidationError
		if errors.As(err, &ve) {
			return m, m.focusField(fieldIndex(ve.Field))
		}
		return m, nil
	}
	m.formErr, m.formMsg = nil, ""
	m.toSave, m.confirmSave = song, true
	for i := range m.fields {
		m.fields[i].Blur() // nada de digitar enquanto pergunta
	}
	return m, nil
}

// handleConfirmSave trata a pergunta "Salvar…?": y/enter grava, n/esc volta
// a editar os campos.
func (m Model) handleConfirmSave(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Confirm):
		m.confirmSave, m.saving = false, true
		editID := ""
		if m.editing != nil {
			editID = m.editing.ID
		}
		return m, tea.Batch(m.saveSong(editID, m.toSave), m.focusField(m.focus))
	case key.Matches(msg, m.keys.Deny):
		m.confirmSave = false
		return m, m.focusField(m.focus)
	}
	return m, nil
}

func (m Model) onSaved(msg saveMsg) (Model, tea.Cmd) {
	m.saving = false
	if msg.err != nil {
		m.formErr = msg.err
		var ve *feramusicas.ValidationError
		switch {
		case errors.As(msg.err, &ve):
			m.focus = fieldIndex(ve.Field)
		case errors.Is(msg.err, feramusicas.ErrDuplicate):
			m.focus = fURL
		case errors.Is(msg.err, feramusicas.ErrNotFound):
			m.formErr = errors.New("essa música não existe mais (outra pessoa removeu?); esc volta à lista")
		}
		return m, m.focusField(m.focus)
	}
	if msg.edited {
		// Edição: volta à lista, com o cursor na música editada.
		m.clearFields()
		m.editing, m.selectID = nil, msg.song.ID
		m.screen, m.notice = screenList, "✓ alterada: "+msg.song.Name
		for i := range m.fields {
			m.fields[i].Blur()
		}
		return m, m.reload()
	}
	// Cadastro: limpa tudo menos a categoria, para cadastrar várias músicas da
	// mesma categoria em sequência.
	m.formErr, m.formMsg = nil, "✓ cadastrada: "+msg.song.Name
	m.fields[fName].Reset()
	m.fields[fURL].Reset()
	m.fields[fStart].Reset()
	return m, tea.Batch(m.focusField(fName), m.reload())
}

func fieldIndex(field string) int {
	switch field {
	case feramusicas.FieldCategory:
		return fCategory
	case feramusicas.FieldURL:
		return fURL
	case feramusicas.FieldStart:
		return fStart
	}
	return fName
}

// categoryHints são as categorias existentes que casam com o texto digitado.
func (m Model) categoryHints() []string {
	q := strings.ToLower(strings.TrimSpace(m.fields[fCategory].Value()))
	var out []string
	for _, c := range m.cats {
		if q == "" || strings.Contains(strings.ToLower(c), q) {
			out = append(out, c)
		}
	}
	return out
}

// completeCategory completa o campo com a 1ª categoria existente que começa
// com o texto digitado. Devolve false (e o tab segue para o próximo campo) se
// não há o que completar ou se o texto já é uma categoria existente.
func (m *Model) completeCategory() bool {
	typed := feramusicas.CleanText(m.fields[fCategory].Value())
	if typed == "" {
		return false
	}
	for _, c := range m.cats {
		if strings.EqualFold(c, typed) {
			return false
		}
	}
	for _, c := range m.cats {
		if strings.HasPrefix(strings.ToLower(c), strings.ToLower(typed)) {
			m.fields[fCategory].SetValue(c)
			m.fields[fCategory].CursorEnd()
			return true
		}
	}
	return false
}

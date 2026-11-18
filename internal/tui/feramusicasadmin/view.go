package feramusicasadmin

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/games/feramusicas"
	"fera-games/internal/tui/theme"
)

const (
	contentW = 76 // 80 colunas menos o padding do Frame
	listRows = 10 // linhas visíveis da lista (com os cabeçalhos de categoria)
	nameW    = 48 // 3 + 48 + 2 + vídeo (11) + 1 + início ("11:59:59.5") = 75 colunas
)

var (
	dim   = theme.Subtitle
	bold  = lipgloss.NewStyle().Bold(true)
	match = lipgloss.NewStyle().Foreground(theme.Accent)
	label = lipgloss.NewStyle().Foreground(theme.Muted).Width(11)
)

func title(sub string) string {
	return theme.Title.Render(feramusicas.GameName) + " " + bold.Render("Admin") + " " + dim.Render(sub)
}

// View desenha a tela ativa; cabe em 80x24.
func (m Model) View() string {
	var b strings.Builder
	switch m.screen {
	case screenLoading:
		b.WriteString(title("") + "\n\n" + m.spinner.View() + " abrindo o catálogo…\n")
	case screenError:
		b.WriteString(title("") + "\n\n" + theme.Error.Render("Não deu para abrir o catálogo: "+m.err.Error()) + "\n")
	case screenList:
		m.viewList(&b)
	case screenForm:
		m.viewForm(&b)
	}
	b.WriteString("\n")
	switch m.screen {
	case screenForm:
		b.WriteString(m.help.View(formHelp{m.keys, m.confirmSave}))
	case screenError, screenLoading:
		b.WriteString(m.help.View(errorHelp{m.keys}))
	default:
		b.WriteString(m.help.View(listHelp{m.keys, m.confirmDel}))
	}
	return theme.Frame.Render(b.String())
}

// --- lista ------------------------------------------------------------------

type listRow struct {
	header string // categoria (cabeçalho) quando song < 0
	song   int    // índice em m.songs
}

// listRows intercala cabeçalhos de categoria com as músicas (já ordenadas).
func (m Model) listRows() []listRow {
	var rows []listRow
	last := ""
	for i, s := range m.songs {
		if k := feramusicas.CategoryKey(s.Category); k != last {
			rows = append(rows, listRow{header: m.categoryName(k), song: -1})
			last = k
		}
		rows = append(rows, listRow{song: i})
	}
	return rows
}

func (m Model) categoryName(key string) string {
	for _, c := range m.cats {
		if feramusicas.CategoryKey(c) == key {
			return c
		}
	}
	return key
}

func pad(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

func (m Model) viewList(b *strings.Builder) {
	b.WriteString(title(fmt.Sprintf("· músicas cadastradas (%d)", len(m.songs))) + "\n")
	if n := m.deps.CatalogNotice; n != "" {
		b.WriteString(theme.Notice.Render(ansi.Truncate(n, contentW, "…")) + "\n")
	}

	if len(m.songs) == 0 {
		b.WriteString("\nNenhuma música cadastrada ainda.\n")
		b.WriteString(dim.Render("Tecle a para cadastrar a primeira: nome, categoria e link do YouTube.") + "\n")
		return
	}
	b.WriteString("\n")

	rows := m.listRows()
	at := 0
	for i, r := range rows {
		if r.song == m.cursor {
			at = i
		}
	}
	// Janela de listRows linhas que mantém o cursor visível (com o cabeçalho da
	// categoria junto, quando ele é a linha logo acima).
	top := max(0, min(at-listRows/2, len(rows)-listRows))
	if top > 0 && at == top && rows[top-1].song < 0 {
		top--
	}
	end := min(top+listRows, len(rows))
	for _, r := range rows[top:end] {
		if r.song < 0 {
			b.WriteString(bold.Render(ansi.Truncate(r.header, contentW, "…")) + "\n")
			continue
		}
		b.WriteString(m.songRow(r.song) + "\n")
	}
	if top > 0 || end < len(rows) {
		b.WriteString(dim.Render(fmt.Sprintf("   … %d/%d", m.cursor+1, len(m.songs))) + "\n")
	}
	switch {
	case m.confirmDel && m.cursor < len(m.songs):
		b.WriteString("\n" + theme.Error.Render("Remover “"+ansi.Truncate(m.songs[m.cursor].Name, 50, "…")+"”?") + "\n")
	case m.notice != "":
		b.WriteString("\n" + theme.Notice.Render(m.notice) + "\n")
	}
}

// songRow é "▸ nome  vídeo [início]".
func (m Model) songRow(i int) string {
	s := m.songs[i]
	video := s.VideoID
	if s.Start > 0 {
		video += " " + feramusicas.FormatStart(s.Start) // de onde o trecho começa
	}
	if i == m.cursor {
		return " " + match.Render("▸ "+pad(s.Name, nameW)) + "  " + dim.Render(video)
	}
	return "   " + pad(s.Name, nameW) + "  " + dim.Render(video)
}

// --- cadastro ---------------------------------------------------------------

func (m Model) viewForm(b *strings.Builder) {
	sub := "· cadastrar música"
	if m.editing != nil {
		sub = "· editar música"
	}
	b.WriteString(title(sub) + "\n\n")

	var ve *feramusicas.ValidationError
	haveField := errors.As(m.formErr, &ve)
	names := [numFields]string{"Nome", "Categoria", "Link", "Início"}
	for i := range m.fields {
		b.WriteString(label.Render(names[i]) + m.fields[i].View() + "\n")
		if haveField && fieldIndex(ve.Field) == i {
			b.WriteString(label.Render("") + theme.Error.Render(ve.Msg) + "\n")
		} else if errors.Is(m.formErr, feramusicas.ErrDuplicate) && i == fURL {
			b.WriteString(label.Render("") + theme.Error.Render(m.formErr.Error()) + "\n")
		}
		b.WriteString("\n")
	}
	if m.formErr != nil && !haveField && !errors.Is(m.formErr, feramusicas.ErrDuplicate) {
		b.WriteString(theme.Error.Render(m.formErr.Error()) + "\n")
	}
	m.viewPreview(b)
	if m.focus == fStart && !haveField && !m.confirmSave {
		b.WriteString(dim.Render("de onde o trecho começa no vídeo, para pular a introdução; vazio = do começo") + "\n")
	}
	if m.focus == fCategory && !m.confirmSave {
		if hints := m.categoryHints(); len(hints) > 0 {
			b.WriteString(dim.Render(ansi.Truncate("categorias: "+strings.Join(hints, " · ")+"   (tab completa)", contentW, "…")) + "\n")
		}
	}
	if m.confirmSave {
		// Quebra em linhas em vez de truncar: o "(y/n)" não pode sumir.
		b.WriteString(theme.Notice.Width(contentW).Render(m.confirmQuestion()) + "\n")
	}
	if m.saving {
		b.WriteString(dim.Render("salvando…") + "\n")
	}
	if m.formMsg != "" {
		b.WriteString(theme.Notice.Render(ansi.Truncate(m.formMsg, contentW, "…")) + "\n")
	}
}

// --- prévia -----------------------------------------------------------------

// viewPreview mostra o estado da prévia do trecho (ctrl+p).
func (m Model) viewPreview(b *strings.Builder) {
	switch {
	case m.previewing:
		b.WriteString(m.spinner.View() + dim.Render(" baixando o trecho do YouTube…") + "\n")
		return
	case m.previewErr != "":
		b.WriteString(theme.Error.Render(ansi.Truncate("prévia: "+m.previewErr, contentW, "…")) + "\n")
		return
	case m.previewTrack == nil:
		b.WriteString(dim.Render("ctrl+p ouve o trecho a partir do início, do jeito que o jogo vai tocar") + "\n")
		return
	}
	p := m.deps.Player
	pos, dur := p.Position(), p.Duration()
	icon := "▶ "
	if p.Playing() {
		icon = "❚❚"
	}
	from := feramusicas.FormatStart(m.previewStart)
	if from == "" {
		from = "0:00"
	}
	line := fmt.Sprintf("%s %s / %s · a partir de %s do vídeo · %s", icon, clock(pos), clock(dur), from, roundAt(pos))
	if p.Silent() {
		line += " (mudo)"
	}
	b.WriteString(match.Render(ansi.Truncate(line, contentW, "…")) + "\n")
	if cur, err := m.formSong(); err != nil || previewKey(cur) != m.previewKey {
		b.WriteString(dim.Render("o link ou o início mudou desde a prévia: ctrl+p para ouvir de novo") + "\n")
	}
	if n := m.deps.AudioNotice; n != "" {
		b.WriteString(theme.Error.Render(ansi.Truncate(n, contentW, "…")) + "\n")
	}
}

// roundAt diz em que round do jogo o trecho tocado até pos já aparece: o
// round 1 só mostra os primeiros 0,13 s, o 5º mostra 15 s.
func roundAt(pos time.Duration) string {
	for r := 1; r <= feramusicas.MaxGuesses; r++ {
		if pos <= feramusicas.LimitFor(r) {
			return fmt.Sprintf("revelado no round %d", r)
		}
	}
	return "depois do round 5"
}

func clock(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// confirmQuestion é a pergunta antes de gravar, com o que vai ser salvo.
func (m Model) confirmQuestion() string {
	s := m.toSave
	what := fmt.Sprintf("“%s” em %s", s.Name, s.Category)
	if s.Start > 0 {
		what += ", a partir de " + feramusicas.FormatStart(s.Start)
	}
	if m.editing != nil {
		return "Salvar as alterações: " + what + "? (y/n)"
	}
	return "Cadastrar " + what + "? (y/n)"
}

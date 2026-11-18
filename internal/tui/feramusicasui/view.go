package feramusicasui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/game"
	"fera-games/internal/games/feramusicas"
	"fera-games/internal/tui/theme"
)

const (
	barCells    = 48 // células da barra, sem contar os vãos entre trechos
	boxInner    = 60 // largura útil da caixa de tentativas
	contentW    = 76 // 80 colunas menos o padding do Frame
	listRows    = 11 // linhas visíveis da lista do catálogo (com os cabeçalhos)
	maxTabLabel = 20 // 3 abas × (20 + "n " + padding) cabem em 76 colunas
)

var (
	tabOn   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(theme.Accent).Padding(0, 1)
	tabOff  = lipgloss.NewStyle().Foreground(theme.Muted).Padding(0, 1)
	dim     = theme.Subtitle
	current = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	answer  = lipgloss.NewStyle().Bold(true).Foreground(theme.Green)
	match   = lipgloss.NewStyle().Foreground(theme.Accent)
)

// View desenha a tela ativa; cabe em 80x24.
func (m Model) View() string {
	var b strings.Builder
	switch m.screen {
	case screenLoading:
		b.WriteString(theme.Title.Render(feramusicas.GameName))
		b.WriteString("\n\n" + m.spinner.View() + " " + loadingLabel + "\n")
	case screenError:
		b.WriteString(theme.Title.Render(feramusicas.GameName))
		b.WriteString("\n\n" + theme.Error.Render("Não deu para abrir o catálogo: "+m.err.Error()) + "\n")
	case screenGame:
		m.viewGame(&b)
	case screenEmpty:
		b.WriteString(theme.Title.Render(feramusicas.GameName) + "\n\n")
		b.WriteString("Nenhuma música cadastrada ainda.\n")
		b.WriteString(dim.Render("Cadastre no Admin: tecla a no menu inicial.") + "\n")
		if n := m.deps.CatalogNotice; n != "" {
			b.WriteString("\n" + theme.Notice.Render(ansi.Truncate(n, contentW, "…")) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.helpView())
	return theme.Frame.Render(b.String())
}

func (m Model) helpView() string {
	switch m.screen {
	case screenError:
		return m.help.View(errorHelp{m.keys})
	case screenEmpty:
		return m.help.View(menuHelp{m.keys})
	case screenGame:
		if m.typing {
			return m.help.View(typingHelp{m.keys})
		}
		finished := m.session != nil && m.session.State().Status != game.Playing
		return m.help.View(playerHelp{k: m.keys, finished: finished})
	}
	return m.help.View(errorHelp{m.keys})
}

// --- jogo -------------------------------------------------------------------

func (m Model) viewGame(b *strings.Builder) {
	header := theme.Title.Render(feramusicas.GameName)
	if m.session != nil {
		header += " " + dim.Render(m.session.Date())
	}
	header += " " + dim.Render(fmt.Sprintf("· %d/%d músicas do dia", len(m.finishedToday()), len(m.daily)))
	b.WriteString(header + "\n")
	b.WriteString(ansi.Truncate(m.categoryTabs(), contentW, "…") + "\n")

	if m.session == nil {
		if m.starting {
			b.WriteString("\n" + m.spinner.View() + " sorteando a música do dia…\n")
		}
		if m.notice != "" {
			b.WriteString("\n" + theme.Notice.Render(m.notice) + "\n")
		}
		return
	}
	st := m.session.State()
	b.WriteString("\n" + m.playerView() + "\n")

	outcomes := m.session.Outcomes()
	rows := make([]string, 0, feramusicas.MaxGuesses)
	for i := 0; i < feramusicas.MaxGuesses; i++ {
		step := i + 1
		var mark, text string
		var style lipgloss.Style
		switch {
		case i < len(outcomes):
			mark, style = outcomes[i].Emoji(), outcomeStyle(outcomes[i])
			if outcomes[i] == feramusicas.OutcomeSkipped {
				text = "pulou"
			} else {
				text = st.Guesses[i]
			}
		case step == st.Step && st.Status == game.Playing:
			mark, text, style = current.Render("▸ "), "sua vez", current
		default:
			mark, style = "  ", dim
		}
		rows = append(rows, gridRow(step, mark, text, shortLimit(feramusicas.LimitFor(step)), style))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(boxBorder(st.Status)).Padding(0, 1)
	b.WriteString(box.Render(strings.Join(rows, "\n")) + "\n")

	switch {
	case st.Status != game.Playing:
		m.viewFinished(b, st)
	case m.typing:
		b.WriteString(m.input.View() + "\n")
		for i, name := range m.matches {
			if i == m.sel {
				b.WriteString("  " + match.Render("▸ "+name) + "\n")
			} else {
				b.WriteString("    " + name + "\n")
			}
		}
		if len(m.matches) == 0 && strings.TrimSpace(m.input.Value()) != "" {
			b.WriteString(dim.Render("  nenhuma música parecida") + "\n")
		}
	default:
		b.WriteString(dim.Render("> enter para chutar; tab pula; 1-3 categoria") + "\n")
	}
	if m.notice != "" {
		b.WriteString(theme.Notice.Render(m.notice) + "\n")
	}
}

// categoryTabs desenha as abas das categorias do dia (até 3); o rótulo encolhe
// para todas caberem em uma linha, até maxTabLabel.
func (m Model) categoryTabs() string {
	n := len(m.daily)
	if n == 0 {
		return ""
	}
	labelW := min(max(contentW/n-4, 3), maxTabLabel) // 4 = "n " + padding da aba
	parts := make([]string, 0, n)
	for i, c := range m.daily {
		text := fmt.Sprintf("%d %s", i+1, ansi.Truncate(c, labelW, "…"))
		if feramusicas.CategoryKey(c) == m.cur {
			parts = append(parts, tabOn.Render(text))
		} else {
			parts = append(parts, tabOff.Render(text))
		}
	}
	return strings.Join(parts, "")
}

// outcomeStyle é a cor de uma tentativa.
func outcomeStyle(o feramusicas.Outcome) lipgloss.Style {
	switch o {
	case feramusicas.OutcomeCorrect:
		return lipgloss.NewStyle().Bold(true).Foreground(theme.Green)
	case feramusicas.OutcomeIncorrect:
		return lipgloss.NewStyle().Foreground(theme.Red)
	}
	return dim
}

func boxBorder(s game.Status) color.Color {
	switch s {
	case game.Won:
		return theme.Green
	case game.Lost:
		return theme.Red
	}
	return theme.Muted
}

// gridRow monta "n  marca texto ........ limite" com largura fixa.
func gridRow(n int, mark, text, limit string, style lipgloss.Style) string {
	left := fmt.Sprintf("%d  %s ", n, mark)
	room := boxInner - lipgloss.Width(left) - lipgloss.Width(limit) - 1
	text = ansi.Truncate(text, max(room, 1), "…")
	pad := max(boxInner-lipgloss.Width(left)-lipgloss.Width(text)-lipgloss.Width(limit), 1)
	return dim.Render(fmt.Sprintf("%d", n)) + "  " + mark + " " + style.Render(text) +
		strings.Repeat(" ", pad) + dim.Render(limit)
}

func shortLimit(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func (m Model) viewFinished(b *strings.Builder, st game.State) {
	name := ""
	if a := m.session.Answer(); a != nil {
		name = a.Name
	}
	if st.Status == game.Won {
		fmt.Fprintf(b, "%s %s\n", answer.Render("Acertou!"), answer.Render(name))
	} else {
		fmt.Fprintf(b, "%s era %s\n", theme.Error.Render("Não foi dessa vez."), answer.Render(name))
	}
	if share := strings.Split(m.session.ShareText(), "\n"); len(share) > 1 {
		b.WriteString(share[1] + "\n")
	}
	if m.copied {
		b.WriteString(theme.Notice.Render("resultado copiado") + "\n")
	}
}

func clock(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

func (m Model) playerView() string {
	if m.inPlayer != m.session {
		switch {
		case m.audioErrs[m.session] != "":
			return theme.Error.Render(ansi.Truncate(" áudio: "+m.audioErrs[m.session], contentW, "…"))
		case m.loading[m.session]:
			return " " + m.spinner.View() + dim.Render(" baixando o trecho do YouTube…")
		}
		return dim.Render(" (sem áudio)")
	}
	pl := m.deps.Player
	pos, dur := pl.Position(), pl.Duration()
	icon := "▶"
	if pl.Playing() {
		icon = "❚❚"
	}
	s := fmt.Sprintf(" %-2s %s %s %s", icon, clock(pos), m.segmentBar(pos), clock(min(dur, 15*time.Second)))
	if pl.Silent() {
		s += dim.Render("  (mudo)")
	}
	if m.deps.AudioNotice != "" {
		s += "\n " + theme.Error.Render(m.deps.AudioNotice)
	}
	return s
}

// segmentBar desenha a barra do Songless: um trecho por round, largura ∝ ao
// limite. Rounds jogados ganham a cor do resultado; o atual mostra o avanço
// do player; os futuros ficam apagados.
func (m Model) segmentBar(pos time.Duration) string {
	var outcomes []feramusicas.Outcome
	step := 0
	if m.session != nil {
		outcomes = m.session.Outcomes()
		if st := m.session.State(); st.Status == game.Playing {
			step = st.Step
		}
	}
	full := time.Duration(feramusicas.Timesteps[len(feramusicas.Timesteps)-1] * float64(time.Second))
	var parts []string
	prevEnd, prevT := 0, time.Duration(0)
	for i := 0; i < feramusicas.MaxGuesses; i++ {
		t := feramusicas.LimitFor(i + 1)
		end := int(float64(barCells)*float64(t)/float64(full) + 0.5)
		end = max(end, prevEnd+1)
		w := end - prevEnd
		if i < len(outcomes) || i+1 == step {
			base := current
			if i < len(outcomes) {
				base = outcomeStyle(outcomes[i])
			}
			f := int(float64(w)*float64(pos-prevT)/float64(t-prevT) + 0.5)
			f = max(0, min(f, w))
			parts = append(parts, base.Render(strings.Repeat("█", f))+
				base.Faint(true).Render(strings.Repeat("░", w-f)))
		} else {
			parts = append(parts, dim.Render(strings.Repeat("▒", w)))
		}
		prevEnd, prevT = end, t
	}
	return strings.Join(parts, " ")
}

// friendlyAudio traduz uma falha de download numa frase curta e acionável.
// Não há tecla de "tentar de novo": sair da categoria e voltar (outra aba, ou
// q e enter no menu) já baixa de novo. As ferramentas são procuradas na abertura do
// programa, então depois de instalá-las é preciso reabri-lo.
func friendlyAudio(err error) string {
	switch {
	case errors.Is(err, feramusicas.ErrMissingTools):
		detail := strings.TrimPrefix(err.Error(), feramusicas.ErrMissingTools.Error())
		detail = strings.TrimPrefix(detail, ": ")
		if detail == "" {
			detail = "yt-dlp e ffmpeg"
		}
		return "faltando " + detail + "; instale (veja o README) e reabra o programa"
	case errors.Is(err, feramusicas.ErrVideoUnavailable):
		return "vídeo indisponível; remova essa música no Admin"
	case errors.Is(err, feramusicas.ErrAgeRestricted):
		return "vídeo com restrição de idade; remova essa música no Admin"
	case errors.Is(err, feramusicas.ErrStartPastEnd):
		return "não há áudio a partir do início cadastrado; corrija o início no Admin"
	case errors.Is(err, feramusicas.ErrNoAudio):
		return "o vídeo não tem áudio; remova essa música no Admin"
	case errors.Is(err, feramusicas.ErrBotCheck):
		return "o YouTube pediu verificação; atualize o yt-dlp (yt-dlp -U)"
	case errors.Is(err, context.DeadlineExceeded):
		return "o download demorou demais; saia (q) e entre de novo"
	}
	first, _, _ := strings.Cut(strings.TrimSpace(err.Error()), "\n")
	return "download falhou: " + first
}

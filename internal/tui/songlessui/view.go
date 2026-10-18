package songlessui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"net"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/game"
	"fera-games/internal/games/songless"
	"fera-games/internal/tui/theme"
)

const (
	barCells = 48 // células da barra, sem contar os vãos entre trechos
	boxInner = 60 // largura útil da caixa de tentativas
)

var (
	yellow  = lipgloss.Color("#E0B83A")
	tabOn   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(theme.Accent).Padding(0, 1)
	tabOff  = lipgloss.NewStyle().Foreground(theme.Muted).Padding(0, 1)
	dim     = theme.Subtitle
	bold    = lipgloss.NewStyle().Bold(true)
	current = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	answer  = lipgloss.NewStyle().Bold(true).Foreground(theme.Green)
	match   = lipgloss.NewStyle().Foreground(theme.Accent)
)

// View desenha a tela; cabe em 80x24 com 6 sugestões.
func (m Model) View() string {
	var b strings.Builder
	switch m.phase {
	case phaseLoading:
		b.WriteString(theme.Title.Render("Songless"))
		b.WriteString("\n\n" + m.spinner.View() + " " + loadingLabel + "\n")
	case phaseError:
		b.WriteString(theme.Title.Render("Songless"))
		b.WriteString("\n\n" + theme.Error.Render(friendly(m.err)) + "\n")
		b.WriteString(dim.Render("r tenta de novo · q volta ao menu") + "\n")
	default:
		m.viewGame(&b)
	}
	b.WriteString("\n")
	b.WriteString(m.helpView())
	return theme.Frame.Render(b.String())
}

func (m Model) viewGame(b *strings.Builder) {
	st := m.session.State()

	fmt.Fprintf(b, "%s %s  %s\n", theme.Title.Render("Songless"),
		bold.Render(fmt.Sprintf("#%d", st.Number)), m.categoryTabs())
	b.WriteString("\n" + m.playerView() + "\n")

	outcomes := m.session.Outcomes()
	var rows []string
	for i := 0; i < songless.MaxGuesses; i++ {
		step := i + 1
		var mark, text string
		style := lipgloss.NewStyle()
		switch {
		case i < len(outcomes):
			mark = outcomes[i].Emoji()
			style = outcomeStyle(outcomes[i])
			if outcomes[i] == songless.OutcomeSkipped {
				text = "pulou"
			} else {
				text = st.Guesses[i]
			}
		case step == st.Step && st.Status == game.Playing:
			mark = current.Render("▸ ")
			text, style = "sua vez", current
		default:
			mark, style = "  ", dim
		}
		rows = append(rows, gridRow(step, mark, text, shortLimit(songless.LimitFor(step)), style))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(boxBorder(st.Status)).Padding(0, 1)
	b.WriteString(box.Render(strings.Join(rows, "\n")) + "\n")

	switch {
	case m.phase == phaseFinished:
		m.viewFinished(b, st)
	case m.typing:
		b.WriteString(m.input.View() + "\n")
		if m.searching {
			b.WriteString(dim.Render("  ⟳ buscando…") + "\n")
		}
		for i, e := range m.matches {
			label := e.Display()
			if i == m.sel {
				b.WriteString("  " + match.Render("▸ "+label) + "\n")
			} else {
				b.WriteString("    " + label + "\n")
			}
		}
		if !m.searching && len(m.matches) == 0 && strings.TrimSpace(m.input.Value()) != "" {
			b.WriteString(dim.Render("  nenhuma música parecida") + "\n")
		}
	default:
		hint := "> enter ou comece a digitar para chutar; tab pula; 1/2/3 categoria"
		if m.restored && len(outcomes) > 0 && m.notice == "" {
			hint = "> partida de hoje restaurada; continue de onde parou"
		}
		b.WriteString(dim.Render(hint) + "\n")
	}
	if m.notice != "" {
		b.WriteString(theme.Notice.Render(m.notice) + "\n")
	}
}

func (m Model) categoryTabs() string {
	var parts []string
	for _, c := range m.daily.Categories {
		label := fmt.Sprintf("%d %s", c.ID+1, c.Name)
		if c.ID == m.category {
			parts = append(parts, tabOn.Render(label))
		} else {
			parts = append(parts, tabOff.Render(label))
		}
	}
	return strings.Join(parts, "")
}

// outcomeStyle é a cor de uma tentativa, igual às caixas do site.
func outcomeStyle(o songless.Outcome) lipgloss.Style {
	switch o {
	case songless.OutcomeCorrect:
		return lipgloss.NewStyle().Bold(true).Foreground(theme.Green)
	case songless.OutcomePresent:
		return lipgloss.NewStyle().Foreground(yellow)
	case songless.OutcomeIncorrect:
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
	if ans := m.session.Answer(); ans != nil {
		if st.Status == game.Won {
			fmt.Fprintf(b, "%s %s\n", answer.Render("Acertou!"), answer.Render(ans.Display()))
		} else {
			fmt.Fprintf(b, "%s era %s\n", theme.Error.Render("Não foi hoje."), answer.Render(ans.Display()))
		}
	} else if st.Status == game.Won {
		b.WriteString(answer.Render("Acertou!") + "\n")
	} else {
		b.WriteString(theme.Error.Render("Não foi hoje.") + "\n")
	}
	share := strings.Split(m.shareText(), "\n")
	if len(share) > 0 {
		b.WriteString(dim.Render(share[0]) + "\n")
	}
	if len(share) > 2 {
		b.WriteString(share[2] + "\n")
	}
	if m.copied {
		b.WriteString(theme.Notice.Render("resultado copiado (todas as categorias concluídas hoje)") + "\n")
	}
}

func (m Model) playerView() string {
	pl := m.deps.Player
	pos, dur := pl.Position(), pl.Duration()
	icon := "▶"
	if pl.Playing() {
		icon = "❚❚"
	}
	if !m.audioReady {
		if m.loadingAudio {
			return dim.Render(" ⟳ baixando o áudio…")
		}
		return dim.Render(" (sem áudio)")
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

// friendly traduz erros comuns para uma frase útil.
func friendly(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, songless.ErrNoPuzzle):
		return "A música de hoje ainda não foi publicada. Tente de novo em alguns minutos."
	case errors.Is(err, context.DeadlineExceeded):
		return "O servidor do Songless demorou demais para responder."
	}
	var netErr net.Error
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) || errors.As(err, &netErr) {
		return "Sem conexão com o Songless. Confira a internet."
	}
	return "Não deu para carregar a música: " + err.Error()
}

func clock(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

func (m Model) helpView() string {
	if m.typing {
		return m.help.View(typingHelp{m.keys})
	}
	return m.help.View(playerHelp{k: m.keys, finished: m.phase == phaseFinished})
}

// segmentBar desenha a barra do site: um trecho por round, largura ∝ ao
// limite (width:100*s/15). Rounds jogados ganham a cor do resultado; o
// atual mostra o avanço do player; os futuros ficam apagados.
func (m Model) segmentBar(pos time.Duration) string {
	outcomes := []songless.Outcome(nil)
	step := 0
	if m.session != nil {
		outcomes = m.session.Outcomes()
		step = m.session.State().Step
		if m.phase == phaseFinished {
			step = 0
		}
	}
	var parts []string
	prevEnd, prevT := 0, time.Duration(0)
	for i := 0; i < songless.MaxGuesses; i++ {
		t := songless.LimitFor(i + 1)
		end := int(float64(barCells)*float64(t)/float64(15*time.Second) + 0.5)
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

package songlessui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"fera-games/internal/game"
	"fera-games/internal/games/songless"
	"fera-games/internal/tui/theme"
)

const barWidth = 30

var (
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

	fmt.Fprintf(b, "%s %s   %s\n", theme.Title.Render("Songless"),
		bold.Render(fmt.Sprintf("#%d", st.Number)), dim.Render(m.categoryTabs()))
	b.WriteString("\n")

	b.WriteString(m.playerView())
	b.WriteString("\n\n")

	outcomes := m.session.Outcomes()
	for i := 0; i < songless.MaxGuesses; i++ {
		step := i + 1
		var mark, text string
		switch {
		case i < len(outcomes):
			mark = outcomes[i].Emoji()
			if outcomes[i] == songless.OutcomeSkipped {
				text = dim.Render("pulou")
			} else {
				text = st.Guesses[i]
			}
		case step == st.Step && st.Status == game.Playing:
			mark = current.Render("▸ ")
			text = current.Render("sua vez")
		default:
			mark = "  "
		}
		limit := songless.LimitFor(step)
		fmt.Fprintf(b, " %d  %s %-8s %s\n", step, mark, shortLimit(limit), text)
	}

	b.WriteString("\n")
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
		name := c.Name
		if c.ID == m.category {
			name = current.Render("[" + name + "]")
		} else {
			name = dim.Render(name)
		}
		parts = append(parts, fmt.Sprintf("%d:%s", c.ID+1, name))
	}
	return strings.Join(parts, "  ")
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
		b.WriteString(theme.Notice.Render("resultado copiado para a área de transferência") + "\n")
	}
}

func (m Model) playerView() string {
	pl := m.deps.Player
	pos, dur := pl.Position(), pl.Duration()
	limit := time.Duration(0)
	if m.session != nil {
		limit = m.session.AudioLimit()
	}
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
	// Barra segmentada com largura ∝ s/15 (como o site: width:100*s/15).
	span := 15 * time.Second
	filled := int(float64(barWidth) * float64(min(pos, span)) / float64(span))
	filled = max(0, min(filled, barWidth))
	bar := strings.Repeat("━", filled) + "●" + strings.Repeat("─", barWidth-filled)
	s := fmt.Sprintf(" %-2s %s %s %s", icon, clock(pos), bar, clock(min(dur, span)))
	if limit > 0 {
		s += dim.Render(fmt.Sprintf("  (até %s)", shortLimit(limit)))
	}
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

package bandleui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"fera-games/internal/game"
	"fera-games/internal/games/bandle"
	"fera-games/internal/tui/theme"
)

const barWidth = 30

var (
	dim     = theme.Subtitle
	bold    = lipgloss.NewStyle().Bold(true)
	current = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	locked  = theme.Disabled
	answer  = lipgloss.NewStyle().Bold(true).Foreground(theme.Green)
	match   = lipgloss.NewStyle().Foreground(theme.Accent)
)

// View desenha a tela; cabe em 80x24 com 6 sugestões.
func (m Model) View() string {
	var b strings.Builder
	switch m.phase {
	case phaseLoading:
		b.WriteString(theme.Title.Render("Bandle"))
		b.WriteString("\n\n" + m.spinner.View() + " " + loadingLabel + "\n")
	case phaseError:
		b.WriteString(theme.Title.Render("Bandle"))
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
	p := m.session.Puzzle()
	st := m.session.State()

	// Cabeçalho: número, data e dicas fixas.
	fmt.Fprintf(b, "%s %s   %s\n", theme.Title.Render("Bandle"), bold.Render(fmt.Sprintf("#%d", p.ID)),
		dim.Render(hints(p)))
	b.WriteString("\n")

	// Player.
	b.WriteString(m.playerView())
	b.WriteString("\n\n")

	// Grade de tentativas.
	outcomes := m.session.Outcomes()
	unlocked := len(m.session.UnlockedInstruments())
	for i := 0; i < p.Stages(); i++ {
		step := i + 1
		var mark, inst, text string
		switch {
		case i < len(outcomes):
			mark = outcomes[i].Emoji()
			text = st.Guesses[i]
			if text == "" {
				text = dim.Render("pulou")
			}
		case step == st.Step && st.Status == game.Playing:
			mark = current.Render("▸ ")
			text = current.Render("sua vez")
		default:
			mark = "  "
		}
		if step <= unlocked {
			inst = instrumentLabel(p.Instruments[i])
		} else {
			inst = locked.Render("???")
		}
		if step == st.Step && st.Status == game.Playing {
			inst = current.Render(inst)
		}
		fmt.Fprintf(b, " %d  %s %-22s %s\n", step, mark, inst, text)
	}

	// Dica textual (6º estágio ou fim).
	if clue, ok := m.session.Clue(m.deps.Lang); ok {
		fmt.Fprintf(b, "\n %s %s\n", bold.Render("dica:"), clue)
	}

	b.WriteString("\n")
	switch {
	case m.phase == phaseFinished:
		m.viewFinished(b, p, st)
	case m.typing:
		b.WriteString(m.input.View() + "\n")
		for i, t := range m.matches {
			if i == m.sel {
				b.WriteString("  " + match.Render("▸ "+t) + "\n")
			} else {
				b.WriteString("    " + t + "\n")
			}
		}
		if len(m.matches) == 0 && strings.TrimSpace(m.input.Value()) != "" {
			b.WriteString(dim.Render("  nenhuma música parecida") + "\n")
		}
	default:
		hint := "> enter ou comece a digitar para chutar; tab pula"
		if m.restored && len(outcomes) > 0 && m.notice == "" {
			hint = "> partida de hoje restaurada; continue de onde parou"
		}
		b.WriteString(dim.Render(hint) + "\n")
	}
	if m.notice != "" {
		b.WriteString(theme.Notice.Render(m.notice) + "\n")
	}
}

func (m Model) viewFinished(b *strings.Builder, p bandle.Puzzle, st game.State) {
	if st.Status == game.Won {
		fmt.Fprintf(b, "%s %s\n", answer.Render("Acertou!"), answer.Render(p.Song))
	} else {
		fmt.Fprintf(b, "%s era %s\n", theme.Error.Render("Não foi hoje."), answer.Render(p.Song))
	}
	share := strings.Split(m.shareText(), "\n")
	b.WriteString(dim.Render(share[0]) + "  " + share[1] + "\n")
	if m.stats.Played > 0 {
		fmt.Fprintf(b, "%s\n", dim.Render(fmt.Sprintf("acertos %d/%d · sequência %d (máx %d)",
			m.stats.Won, m.stats.Played, m.stats.CurrentStreak, m.stats.MaxStreak)))
	}
	if m.copied {
		b.WriteString(theme.Notice.Render("resultado copiado para a área de transferência") + "\n")
	}
}

func (m Model) playerView() string {
	pl := m.deps.Player
	pos, dur := pl.Position(), pl.Duration()
	icon := "▶"
	if pl.Playing() {
		icon = "❚❚"
	}
	if m.audioStep == 0 {
		if m.loadingStem > 0 {
			return dim.Render(" ⟳ baixando o áudio…")
		}
		return dim.Render(" (sem áudio)")
	}
	filled := 0
	if dur > 0 {
		filled = int(float64(barWidth) * float64(pos) / float64(dur))
	}
	filled = max(0, min(filled, barWidth))
	bar := strings.Repeat("━", filled) + "●" + strings.Repeat("─", barWidth-filled)
	label := " ouvindo: " + strings.Join(m.audibleInstruments(), " + ")
	if m.loadingStem > 0 {
		label += dim.Render("  ⟳")
	}
	s := fmt.Sprintf(" %-2s %s %s %s%s", icon, clock(pos), bar, clock(dur), dim.Render(label))
	if pl.Silent() {
		s += dim.Render("  (mudo)")
	}
	if m.deps.AudioNotice != "" {
		s += "\n " + theme.Error.Render(m.deps.AudioNotice)
	}
	return s
}

// audibleInstruments são os instrumentos presentes no mp3 carregado.
func (m Model) audibleInstruments() []string {
	p := m.session.Puzzle()
	n := min(m.audioStep, p.AudioStages())
	out := make([]string, 0, n)
	for _, inst := range p.Instruments[:n] {
		out = append(out, instrumentLabel(inst))
	}
	return out
}

// hints são as dicas visíveis desde o início.
func hints(p bandle.Puzzle) string {
	parts := []string{}
	if p.Year > 0 {
		parts = append(parts, fmt.Sprint(p.Year))
	}
	if len(p.Genre) > 0 {
		parts = append(parts, strings.Join(p.Genre, ", "))
	}
	if p.Par > 0 {
		parts = append(parts, fmt.Sprintf("par %d", p.Par))
	}
	return strings.Join(parts, " · ")
}

// instrumentLabel limpa "[bass] + [electric]" para "bass + electric" e
// traduz o marcador da dica.
func instrumentLabel(s string) string {
	if s == "clue" {
		return "dica"
	}
	return strings.NewReplacer("[", "", "]", "").Replace(s)
}

// friendly traduz erros comuns para uma frase útil.
func friendly(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, bandle.ErrNoPuzzle):
		return "O puzzle de hoje ainda não foi publicado. Tente de novo em alguns minutos."
	case errors.Is(err, context.DeadlineExceeded):
		return "O servidor do Bandle demorou demais para responder."
	}
	var netErr net.Error
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) || errors.As(err, &netErr) {
		return "Sem conexão com o Bandle. Confira a internet."
	}
	return "Não deu para carregar o puzzle: " + err.Error()
}

func clock(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

func (m Model) helpView() string {
	if m.typing {
		return m.help.View(typingHelp{m.keys})
	}
	return m.help.View(playerHelp{k: m.keys, finished: m.phase == phaseFinished})
}

package conexoui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"net"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/games/conexo"
	"fera-games/internal/tui/theme"
)

// O tabuleiro imita peças físicas: cada palavra é uma peça creme em relevo
// baixo, com meia célula de lateral desenhada em quadrantes (▖ ▌ ▝▀▘) para
// baixo e para a direita. Selecionar afunda a peça (some a lateral); errar faz
// as 4 tremerem em vermelho; acertar passa um brilho pela faixa do grupo.
// As peças têm cor própria, então o desenho é o mesmo em terminal claro ou
// escuro.
const (
	slotWidth  = 19 // 4 × 19 = as 76 colunas úteis do Frame em 80
	tileFront  = 16 // face da peça; +1 da lateral, +2 de folga para tremer
	tileText   = tileFront - 2
	tileLines  = 3 // altura da face; palavras longas quebram em 2 linhas
	boardWidth = cols * slotWidth
	bandFront  = boardWidth - 1 // a faixa também tem lateral
	shineWidth = 8
	shineStep  = 8 // colunas que o brilho anda por quadro
)

var (
	ink       = lipgloss.Color("#1E2433")
	cream     = lipgloss.Color("#EDE6D6")
	tan       = lipgloss.Color("#B3A68D")
	amber     = lipgloss.Color("#F5B841")
	amberSide = lipgloss.Color("#C9921F")
	missFront = lipgloss.Color("#F2C1C1")
	missText  = lipgloss.Color("#7F1D1D")
	missSide  = lipgloss.Color("#B91C1C")
	white     = lipgloss.Color("#FFFFFF")

	// Por number: a cor do site (tema escuro, também das peças com dica), a
	// lateral, o tom claro (tema claro do site, nas palavras da faixa) e o
	// brilho.
	groupStrong = groupPalette("#C2410C", "#047857", "#0E7490", "#6D28D9")
	groupDeep   = groupPalette("#7C2D12", "#064E3B", "#164E63", "#4C1D95")
	groupLight  = groupPalette("#F6C9AB", "#B8E5C4", "#B4DCEC", "#D6C9F2")
	groupShine  = groupPalette("#EA6A2E", "#10A37A", "#1597B8", "#8B5CF6")

	dim     = theme.Subtitle
	bold    = lipgloss.NewStyle().Bold(true)
	amberFg = lipgloss.NewStyle().Foreground(amber).Bold(true)
	missFg  = lipgloss.NewStyle().Foreground(missSide).Bold(true)
	won     = lipgloss.NewStyle().Foreground(amber).Bold(true)
)

func groupPalette(hex ...string) [conexo.Groups + 1]color.Color {
	var p [conexo.Groups + 1]color.Color
	for i, h := range hex {
		p[i+1] = lipgloss.Color(h)
	}
	return p
}

// View desenha a tela; cabe em 80x24 com o tabuleiro cheio.
func (m Model) View() string {
	var b strings.Builder
	switch m.phase {
	case phaseLoading:
		b.WriteString(wordmark())
		b.WriteString("\n\n" + m.spinner.View() + " " + loadingLabel + "\n")
	case phaseToken:
		m.viewToken(&b)
	case phaseError:
		b.WriteString(wordmark())
		b.WriteString("\n\n" + theme.Error.Render(friendly(m.err)) + "\n")
		b.WriteString(dim.Render("r tenta de novo · q volta ao menu") + "\n")
	default:
		m.viewGame(&b)
	}
	b.WriteString("\n")
	b.WriteString(m.helpView())
	return theme.Frame.Render(b.String())
}

// wordmark é o nome com uma letra de cada grupo.
func wordmark() string {
	var b strings.Builder
	for i, r := range "CONEXO" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(groupStrong[i%conexo.Groups+1]).Render(string(r)))
	}
	return b.String()
}

func (m Model) viewGame(b *strings.Builder) {
	s := m.session
	p := s.Puzzle()
	b.WriteString(m.header() + "\n\n")

	found := s.Found()
	for i, g := range found {
		b.WriteString(band(p, g, m.shine(i, len(found))) + "\n")
	}
	if len(found) > 0 {
		b.WriteString("\n") // respiro entre as faixas e as peças (ou o resultado)
	}
	if grid := m.gridView(); grid != "" {
		b.WriteString(grid + "\n")
	}

	if m.phase == phaseFinished {
		m.viewFinished(b)
		return
	}
	b.WriteString(m.footer() + "\n")
}

// header: nome, número e data à esquerda; grupos achados e contagem à direita.
func (m Model) header() string {
	s := m.session
	left := wordmark() + " " + bold.Render(fmt.Sprintf("#%d", s.Puzzle().ID)) + "  " +
		dim.Render(conexo.DisplayDate(m.deps.Game.Lang(), s.Day()))

	var right strings.Builder
	found := s.Found()
	for i := range conexo.Groups {
		if i < len(found) {
			right.WriteString(lipgloss.NewStyle().Foreground(groupStrong[found[i].Number]).Render("■"))
		} else {
			right.WriteString(dim.Render("□"))
		}
	}
	stats := fmt.Sprintf("tentativas %d", s.Tries())
	if h := s.HintsUsed(); h > 0 {
		stats += fmt.Sprintf(" · dicas %d", h)
	}
	right.WriteString("  " + dim.Render(stats))
	if left := m.tokenExp.Sub(m.deps.Now()); !m.tokenExp.IsZero() && left < tokenWarnAt {
		right.WriteString("  " + theme.Notice.Render(fmt.Sprintf("token expira em %dh", max(int(left.Hours()), 0))))
	}
	gap := max(boardWidth-lipgloss.Width(left)-lipgloss.Width(right.String()), 2)
	return left + strings.Repeat(" ", gap) + right.String()
}

// footer: quantas peças estão selecionadas (a 4ª é o chute) e o aviso.
func (m Model) footer() string {
	n := len(m.session.Selected())
	pips := amberFg.Render(strings.Repeat("◆", n)) + dim.Render(strings.Repeat("◇", cols-n))
	var text string
	switch {
	case m.notice == missNotice:
		text = missFg.Render(m.notice)
	case m.notice != "":
		text = theme.Notice.Render(m.notice)
	case m.restored:
		text = dim.Render("partida de hoje restaurada; continue de onde parou")
	default:
		text = dim.Render("junte 4 palavras do mesmo grupo · a 4ª seleção já é o chute")
	}
	return pips + "  " + ansi.Truncate(text, boardWidth-cols-2, "…")
}

func (m Model) viewFinished(b *strings.Builder) {
	s := m.session
	p := s.Puzzle()
	b.WriteString(won.Render("✦ Parabéns!") + " " + s.ResultLine() + "\n")
	b.WriteString(conexo.Emojis(s.Attempts()) + "\n")
	if p.Credits.Author != "" {
		b.WriteString(dim.Render(fmt.Sprintf("Redação: %s · Revisão: %s", p.Credits.Author, p.Credits.Reviewer)) + "\n")
	}
	if m.copied {
		b.WriteString(theme.Notice.Render("resultado copiado para a área de transferência") + "\n")
	}
}

// shine é a coluna do brilho na faixa i (de n), ou -1 sem brilho. O acerto
// passa o brilho na faixa nova; a vitória passa em cascata por todas.
func (m Model) shine(i, n int) int {
	switch {
	case m.anim.kind == animReveal && i == n-1:
		return m.anim.frame * shineStep
	case m.anim.kind == animWin:
		return (m.anim.frame - i*winStagger) * shineStep
	}
	return -1
}

// band é a faixa de um grupo achado: uma peça larga na cor do grupo, com o
// tema e as palavras.
func band(p conexo.Puzzle, g conexo.Group, shine int) string {
	words := make([]string, len(g.Words))
	for i, w := range g.Words {
		words[i] = p.Display(w)
	}
	n := g.Number
	base := lipgloss.NewStyle().Background(groupStrong[n])
	side := lipgloss.NewStyle().Foreground(groupDeep[n])
	themeLine := shineLine(centered(strings.ToUpper(p.Display(g.Theme)), bandFront), base.Foreground(white).Bold(true), shine, n)
	// O brilho da 2ª linha vem 2 colunas atrás: fica inclinado.
	wordsLine := shineLine(centered(strings.Join(words, ", "), bandFront), base.Foreground(groupLight[n]), shine-2, n)
	return themeLine + side.Render("▖") + "\n" +
		wordsLine + side.Render("▌") + "\n" +
		side.Render(halfEdge(bandFront))
}

// shineLine pinta a linha com o brilho terminando na coluna shine.
func shineLine(plain string, st lipgloss.Style, shine, group int) string {
	a, b := min(max(shine-shineWidth, 0), bandFront), min(max(shine, 0), bandFront)
	if a >= b {
		return st.Render(plain)
	}
	return st.Render(ansi.Cut(plain, 0, a)) +
		st.Background(groupShine[group]).Render(ansi.Cut(plain, a, b)) +
		st.Render(ansi.Cut(plain, b, bandFront))
}

// centered corta e centraliza s em w colunas.
func centered(s string, w int) string {
	s = ansi.Truncate(s, w-2, "…")
	pad := w - ansi.StringWidth(s)
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}

// gridView desenha as palavras soltas, 4 peças por linha.
func (m Model) gridView() string {
	s := m.session
	p := s.Puzzle()
	items := s.Items()
	var rows []string
	for start := 0; start < len(items); start += cols {
		lines := make([]string, tileLines+1)
		for i, w := range items[start:min(start+cols, len(items))] {
			for r, l := range renderTile(cellText(p.Display(w)), m.tileLook(start+i, w)) {
				lines[r] += l
			}
		}
		rows = append(rows, strings.Join(lines, "\n"))
	}
	return strings.Join(rows, "\n")
}

// tileLook é a aparência de uma peça.
type tileLook struct {
	front, side, text color.Color
	pressed           bool // afundada: sem lateral
	offset            int  // coluna da peça dentro do slot (1 = parada)
}

// tileLook decide a peça: erro (vermelha, tremendo) > selecionada (afundada,
// tinta) > cursor (âmbar, mantendo a lateral da dica) > dica (cor cheia do
// grupo, texto branco) > creme.
func (m Model) tileLook(i int, word string) tileLook {
	s := m.session
	lk := tileLook{front: cream, side: tan, text: ink, offset: 1}
	hinted := s.HintGroup() > 0 && slices.Contains(s.Hints(), word)
	if hinted {
		g := s.HintGroup()
		lk.front, lk.side, lk.text = groupStrong[g], groupDeep[g], white
	}
	cursor := i == m.cursor && m.phase == phasePlaying
	if cursor {
		lk.front, lk.text = amber, ink
		if !hinted {
			lk.side = amberSide
		}
	}
	if slices.Contains(s.Selected(), word) {
		lk.pressed, lk.front, lk.text = true, ink, cream
		if cursor {
			lk.text = amber
		}
	}
	if slices.Contains(m.flash, word) {
		lk.front, lk.side, lk.text = missFront, missSide, missText
		if m.anim.kind == animShake {
			lk.offset = shakeOffsets[min(m.anim.frame, len(shakeOffsets)-1)]
		}
	}
	return lk
}

// renderTile devolve as tileLines+1 linhas da peça, cada uma com slotWidth
// colunas: a face, meia coluna de lateral à direita (▖ no topo, ▌ no resto)
// e meia linha embaixo, deslocada meia coluna (▝▀…▀▘).
func renderTile(text string, lk tileLook) []string {
	face := lipgloss.NewStyle().Width(tileFront).Height(tileLines).
		Align(lipgloss.Center).AlignVertical(lipgloss.Center).
		Background(lk.front).Foreground(lk.text).Bold(true).Render(text)
	rows := strings.Split(face, "\n")
	out := make([]string, tileLines+1)
	if lk.pressed {
		for i, r := range rows {
			out[i] = inSlot(lk.offset, r)
		}
		out[tileLines] = inSlot(0, "")
		return out
	}
	side := lipgloss.NewStyle().Foreground(lk.side)
	for i, r := range rows {
		edge := "▌"
		if i == 0 {
			edge = "▖"
		}
		out[i] = inSlot(lk.offset, r+side.Render(edge))
	}
	out[tileLines] = inSlot(lk.offset, side.Render(halfEdge(tileFront)))
	return out
}

// halfEdge é a lateral de baixo de uma face com w colunas: meia linha que
// começa e termina meia coluna à direita da face.
func halfEdge(w int) string { return "▝" + strings.Repeat("▀", w-1) + "▘" }

// inSlot põe s na coluna offset de um slot de slotWidth colunas.
func inSlot(offset int, s string) string {
	return strings.Repeat(" ", offset) + s + strings.Repeat(" ", max(slotWidth-offset-lipgloss.Width(s), 0))
}

// cellText põe a palavra em maiúsculas e quebra em até 2 linhas.
func cellText(word string) string {
	const maxLines = 2
	lines := strings.Split(ansi.Wrap(strings.ToUpper(word), tileText, ""), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = ansi.Truncate(lines[maxLines-1], tileText-1, "") + "…"
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewToken(b *strings.Builder) {
	b.WriteString(wordmark() + "\n\n")
	if errors.Is(m.err, conexo.ErrAppCheck) {
		b.WriteString(theme.Error.Render("O servidor recusou o token do App Check (expirado ou inválido).") + "\n\n")
	} else {
		b.WriteString(bold.Render("O Conexo precisa de um token do App Check, que só o navegador gera.") + "\n\n")
	}
	for i, step := range [][]string{
		{"abra " + bold.Render("https://conexo.ws/pt/") + " no navegador e espere carregar"},
		{"DevTools (F12) → Application → IndexedDB → " + bold.Render("firebase-app-check-database"),
			"→ " + bold.Render("firebase-app-check-store") + dim.Render(" (vazio? recarregue a lista)")},
		{"abra o registro …-[DEFAULT] → value e copie o " + bold.Render("token") + " (começa com eyJ)",
			dim.Render("copiar o registro inteiro também serve")},
		{"cole aqui e aperte " + bold.Render("enter") + "; ele vale até 7 dias"},
	} {
		for j, line := range step {
			num := "   "
			if j == 0 {
				num = amberFg.Render(fmt.Sprintf("%d", i+1)) + "  "
			}
			b.WriteString("  " + num + line + "\n")
		}
	}
	b.WriteString(dim.Render("  ou defina "+conexo.TokenEnv+" antes de abrir o fera") + "\n\n")
	b.WriteString(m.input.View() + "\n")
	if m.notice != "" {
		b.WriteString(theme.Error.Render(ansi.Wrap(m.notice, boardWidth, "")) + "\n")
	}
}

// friendly traduz erros comuns para uma frase útil.
func friendly(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, conexo.ErrNoPuzzle):
		return "O Conexo de hoje ainda não foi publicado. Tente de novo mais tarde."
	case errors.Is(err, conexo.ErrNoToken), errors.Is(err, conexo.ErrAppCheck):
		return "Falta um token válido do App Check: defina " + conexo.TokenEnv + "."
	case errors.Is(err, context.DeadlineExceeded):
		return "O servidor do Conexo demorou demais para responder."
	}
	var netErr net.Error
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) || errors.As(err, &netErr) {
		return "Sem conexão com o Conexo. Confira a internet."
	}
	return "Não deu para carregar o puzzle: " + err.Error()
}

func (m Model) helpView() string {
	if m.phase == phaseToken {
		return m.help.View(tokenHelp{m.keys})
	}
	return m.help.View(boardHelp{k: m.keys, phase: m.phase})
}

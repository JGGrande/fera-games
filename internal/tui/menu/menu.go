// Package menu é a tela inicial: lista os jogos e deixa escolher um.
package menu

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"fera-games/internal/game"
	"fera-games/internal/tui/theme"
)

// SelectedMsg é emitida quando o jogador confirma um jogo disponível.
type SelectedMsg struct{ ID string }

// KeyMap são as teclas do menu; implementa help.KeyMap.
type KeyMap struct {
	Up, Down, Select, Help, Quit key.Binding
}

func (k KeyMap) ShortHelp() []key.Binding { return []key.Binding{k.Up, k.Down, k.Select, k.Quit} }
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Select}, {k.Help, k.Quit}}
}

// DefaultKeyMap segue os atalhos combinados no plano.
var DefaultKeyMap = KeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "subir")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "descer")),
	Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "jogar")),
	Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "ajuda")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "sair")),
}

// Model é o estado do menu.
type Model struct {
	items  []game.Info
	cursor int
	notice string
	keys   KeyMap
	help   help.Model
	width  int
	height int
}

// New cria o menu com os jogos na ordem em que devem aparecer.
func New(items []game.Info) Model {
	return Model{items: items, keys: DefaultKeyMap, help: help.New()}
}

func (m Model) Init() tea.Cmd { return nil }

// Update trata teclas e redimensionamento.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
	case tea.KeyPressMsg:
		m.notice = ""
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Select):
			if len(m.items) == 0 {
				break
			}
			it := m.items[m.cursor]
			if !it.Available {
				m.notice = fmt.Sprintf("%s ainda não está disponível.", it.Name)
				break
			}
			return m, func() tea.Msg { return SelectedMsg{ID: it.ID} }
		}
	}
	return m, nil
}

// Selected devolve o item sob o cursor.
func (m Model) Selected() game.Info {
	if len(m.items) == 0 {
		return game.Info{}
	}
	return m.items[m.cursor]
}

// View desenha o menu. Não usa altura fixa: cabe em 80x24 e cresce com o terminal.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(theme.Title.Render("Fera Games"))
	b.WriteString("  ")
	b.WriteString(theme.Subtitle.Render("jogos diários no terminal"))
	b.WriteString("\n\n")

	for i, it := range m.items {
		cursor := "  "
		style := theme.Normal
		if i == m.cursor {
			cursor = "> "
			style = theme.Selected
		}
		name := style.Render(it.Name)
		desc := theme.Subtitle.Render(it.Description)
		if !it.Available {
			name = theme.Disabled.Render(it.Name)
			desc = theme.Disabled.Render("em breve")
		}
		fmt.Fprintf(&b, "%s%s  %s\n", cursor, name, desc)
	}

	b.WriteString("\n")
	if m.notice != "" {
		b.WriteString(theme.Notice.Render(m.notice))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.help.View(m.keys))
	return theme.Frame.Render(b.String())
}

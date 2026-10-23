package conexoui

import "charm.land/bubbles/v2/key"

// KeyMap são os atalhos da tela. Na fase do token as letras vão para o input.
type KeyMap struct {
	Left      key.Binding
	Right     key.Binding
	Up        key.Binding
	Down      key.Binding
	MoveLeft  key.Binding
	MoveRight key.Binding
	MoveUp    key.Binding
	MoveDown  key.Binding
	Select    key.Binding
	Hint      key.Binding
	Copy      key.Binding
	Submit    key.Binding
	Cancel    key.Binding
	Retry     key.Binding
	Help      key.Binding
	Quit      key.Binding
	Menu      key.Binding
}

// DefaultKeyMap: setas movem o cursor, shift+setas movem o bloco, espaço
// seleciona (o 4º já chuta), d pede dica.
var DefaultKeyMap = KeyMap{
	Left:      key.NewBinding(key.WithKeys("left"), key.WithHelp("setas", "cursor")),
	Right:     key.NewBinding(key.WithKeys("right")),
	Up:        key.NewBinding(key.WithKeys("up")),
	Down:      key.NewBinding(key.WithKeys("down")),
	MoveLeft:  key.NewBinding(key.WithKeys("shift+left"), key.WithHelp("shift+setas", "mover bloco")),
	MoveRight: key.NewBinding(key.WithKeys("shift+right")),
	MoveUp:    key.NewBinding(key.WithKeys("shift+up")),
	MoveDown:  key.NewBinding(key.WithKeys("shift+down")),
	Select:    key.NewBinding(key.WithKeys("space"), key.WithHelp("espaço", "selecionar")),
	Hint:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "dica")),
	Copy:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copiar resultado")),
	Submit:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "usar token")),
	Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "menu")),
	Retry:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "tentar de novo")),
	Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "ajuda")),
	Quit:      key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "sair")),
	Menu:      key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "menu")),
}

// boardHelp é o KeyMap das fases fora do token para o help bubble.
type boardHelp struct {
	k     KeyMap
	phase phase
}

func (h boardHelp) ShortHelp() []key.Binding {
	switch h.phase {
	case phasePlaying:
		return []key.Binding{h.k.Left, h.k.MoveLeft, h.k.Select, h.k.Hint, h.k.Menu}
	case phaseFinished:
		return []key.Binding{h.k.Copy, h.k.Menu}
	case phaseError:
		return []key.Binding{h.k.Retry, h.k.Menu}
	}
	return []key.Binding{h.k.Menu}
}

func (h boardHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{h.k.Left, h.k.MoveLeft},
		{h.k.Select, h.k.Hint, h.k.Copy},
		{h.k.Help, h.k.Menu, h.k.Quit},
	}
}

// tokenHelp é o KeyMap da fase do token.
type tokenHelp struct{ k KeyMap }

func (h tokenHelp) ShortHelp() []key.Binding {
	return []key.Binding{h.k.Submit, h.k.Cancel, h.k.Quit}
}

func (h tokenHelp) FullHelp() [][]key.Binding { return [][]key.Binding{h.ShortHelp()} }

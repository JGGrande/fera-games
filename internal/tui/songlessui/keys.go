package songlessui

import "charm.land/bubbles/v2/key"

// KeyMap são os atalhos da tela. Espelha bandleui, com 1/2/3 para as
// categorias e sem seek fora do limite (o player pausa no timestep).
type KeyMap struct {
	PlayPause key.Binding
	Back5     key.Binding
	Fwd5      key.Binding
	Skip      key.Binding
	Type      key.Binding
	Submit    key.Binding
	Cancel    key.Binding
	Up        key.Binding
	Down      key.Binding
	Complete  key.Binding
	Copy      key.Binding
	Retry     key.Binding
	Help      key.Binding
	Quit      key.Binding
	Menu      key.Binding
	CatAll    key.Binding
	CatRock   key.Binding
	CatHipHop key.Binding
}

// DefaultKeyMap segue os atalhos do Bandle + 1/2/3 para categorias.
var DefaultKeyMap = KeyMap{
	PlayPause: key.NewBinding(key.WithKeys("space"), key.WithHelp("espaço", "tocar/pausar")),
	Back5:     key.NewBinding(key.WithKeys("left"), key.WithHelp("←/→", "±1s")),
	Fwd5:      key.NewBinding(key.WithKeys("right")),
	Skip:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "pular")),
	Type:      key.NewBinding(key.WithKeys("enter", "/"), key.WithHelp("enter", "chutar")),
	Submit:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirmar")),
	Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "voltar ao player")),
	Up:        key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/↓", "escolher")),
	Down:      key.NewBinding(key.WithKeys("down", "ctrl+n")),
	Complete:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "completar")),
	Copy:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copiar resultado")),
	Retry:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "tentar de novo")),
	Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "ajuda")),
	Quit:      key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "sair")),
	Menu:      key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "menu")),
	CatAll:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1/2/3", "categoria")),
	CatRock:   key.NewBinding(key.WithKeys("2")),
	CatHipHop: key.NewBinding(key.WithKeys("3")),
}

// playerHelp é o KeyMap do modo player para o help bubble.
type playerHelp struct {
	k        KeyMap
	finished bool
}

func (h playerHelp) ShortHelp() []key.Binding {
	if h.finished {
		return []key.Binding{h.k.PlayPause, h.k.Copy, h.k.Menu}
	}
	return []key.Binding{h.k.PlayPause, h.k.Skip, h.k.Type, h.k.CatAll, h.k.Menu}
}

func (h playerHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{h.k.PlayPause, h.k.Back5},
		{h.k.Skip, h.k.Type, h.k.Copy},
		{h.k.CatAll, h.k.CatRock, h.k.CatHipHop},
		{h.k.Help, h.k.Menu, h.k.Quit},
	}
}

// typingHelp é o KeyMap do modo digitação.
type typingHelp struct{ k KeyMap }

func (h typingHelp) ShortHelp() []key.Binding {
	return []key.Binding{h.k.Up, h.k.Complete, h.k.Submit, h.k.Cancel}
}

func (h typingHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{{h.k.Up, h.k.Complete}, {h.k.Submit, h.k.Cancel}, {h.k.Quit}}
}

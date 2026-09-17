package bandleui

import "charm.land/bubbles/v2/key"

// KeyMap são os atalhos da tela. O conjunto ativo depende do modo: no modo
// player as letras são comandos; no modo digitação vão para o input.
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
}

// DefaultKeyMap segue os atalhos do plano: espaço, ←/→, tab, enter, c, q.
var DefaultKeyMap = KeyMap{
	PlayPause: key.NewBinding(key.WithKeys("space"), key.WithHelp("espaço", "tocar/pausar")),
	Back5:     key.NewBinding(key.WithKeys("left"), key.WithHelp("←/→", "±5s")),
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
	return []key.Binding{h.k.PlayPause, h.k.Back5, h.k.Skip, h.k.Type, h.k.Menu}
}

func (h playerHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{h.k.PlayPause, h.k.Back5},
		{h.k.Skip, h.k.Type, h.k.Copy},
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

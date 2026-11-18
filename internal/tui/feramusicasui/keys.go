package feramusicasui

import "charm.land/bubbles/v2/key"

// KeyMap são os atalhos da tela de jogo. O cadastro fica no Admin (tecla a no
// menu inicial), fora daqui.
type KeyMap struct {
	// Jogo
	PlayPause key.Binding
	Back      key.Binding // ← (volta 1 s)
	Fwd       key.Binding // → (avança 1 s)
	Skip      key.Binding
	Type      key.Binding
	Category  key.Binding // 1..3 (só ajuda; o tratamento é por dígito)
	Copy      key.Binding
	Retry     key.Binding // só na tela de erro do catálogo
	// Digitação do chute
	Submit   key.Binding
	Cancel   key.Binding
	Up       key.Binding
	Down     key.Binding
	Complete key.Binding
	// Gerais
	Help key.Binding
	Quit key.Binding
	Menu key.Binding
}

// DefaultKeyMap segue os atalhos do Songless.
var DefaultKeyMap = KeyMap{
	PlayPause: key.NewBinding(key.WithKeys("space"), key.WithHelp("espaço", "tocar")),
	Back:      key.NewBinding(key.WithKeys("left"), key.WithHelp("←/→", "±1s")),
	Fwd:       key.NewBinding(key.WithKeys("right")),
	Skip:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "pular")),
	Type:      key.NewBinding(key.WithKeys("enter", "/"), key.WithHelp("enter", "chutar")),
	Category:  key.NewBinding(key.WithKeys("1"), key.WithHelp("1-3", "categoria")),
	Copy:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copiar")),
	Retry:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "tentar de novo")),

	Submit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirmar")),
	Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "voltar ao player")),
	Up:       key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/↓", "escolher")),
	Down:     key.NewBinding(key.WithKeys("down", "ctrl+n")),
	Complete: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "completar")),

	Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "ajuda")),
	Quit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "sair")),
	Menu: key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "menu")),
}

// Cada tela tem seu KeyMap para o help bubble.

type playerHelp struct {
	k        KeyMap
	finished bool
}

func (h playerHelp) ShortHelp() []key.Binding {
	if h.finished {
		return []key.Binding{h.k.PlayPause, h.k.Copy, h.k.Category, h.k.Menu}
	}
	return []key.Binding{h.k.PlayPause, h.k.Skip, h.k.Type, h.k.Category, h.k.Menu}
}

func (h playerHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{h.k.PlayPause, h.k.Back},
		{h.k.Skip, h.k.Type, h.k.Copy},
		{h.k.Category},
		{h.k.Help, h.k.Menu, h.k.Quit},
	}
}

type typingHelp struct{ k KeyMap }

func (h typingHelp) ShortHelp() []key.Binding {
	return []key.Binding{h.k.Up, h.k.Complete, h.k.Submit, h.k.Cancel}
}

func (h typingHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{{h.k.Up, h.k.Complete}, {h.k.Submit, h.k.Cancel}, {h.k.Quit}}
}

type errorHelp struct{ k KeyMap }

func (h errorHelp) ShortHelp() []key.Binding { return []key.Binding{h.k.Retry, h.k.Menu} }
func (h errorHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{{h.k.Retry, h.k.Menu, h.k.Quit}}
}

type menuHelp struct{ k KeyMap }

func (h menuHelp) ShortHelp() []key.Binding  { return []key.Binding{h.k.Menu} }
func (h menuHelp) FullHelp() [][]key.Binding { return [][]key.Binding{{h.k.Menu, h.k.Quit}} }

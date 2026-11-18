package feramusicasadmin

import "charm.land/bubbles/v2/key"

// KeyMap são os atalhos do Admin.
type KeyMap struct {
	// Lista
	Up      key.Binding
	Down    key.Binding
	Add     key.Binding
	Edit    key.Binding
	Delete  key.Binding
	Confirm key.Binding
	Deny    key.Binding
	Retry   key.Binding // tela de erro
	// Cadastro
	Save      key.Binding
	Preview   key.Binding
	Clear     key.Binding
	NextField key.Binding
	PrevField key.Binding
	Close     key.Binding
	// Gerais
	Help key.Binding
	Quit key.Binding
	Menu key.Binding
}

// DefaultKeyMap são os atalhos padrão.
var DefaultKeyMap = KeyMap{
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "escolher")),
	Down:    key.NewBinding(key.WithKeys("down", "j")),
	Add:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "cadastrar")),
	Edit:    key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e", "editar")),
	Delete:  key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "remover")),
	Confirm: key.NewBinding(key.WithKeys("y", "s", "enter"), key.WithHelp("y", "confirmar")),
	Deny:    key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "cancelar")),
	Retry:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "tentar de novo")),

	Save:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "salvar")),
	Preview:   key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "ouvir")),
	Clear:     key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "limpar")),
	NextField: key.NewBinding(key.WithKeys("tab", "down"), key.WithHelp("tab", "campo")),
	PrevField: key.NewBinding(key.WithKeys("shift+tab", "up")),
	Close:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "voltar")),

	Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "ajuda")),
	Quit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "sair")),
	Menu: key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "menu")),
}

type listHelp struct {
	k       KeyMap
	confirm bool
}

func (h listHelp) ShortHelp() []key.Binding {
	if h.confirm {
		return []key.Binding{h.k.Confirm, h.k.Deny}
	}
	return []key.Binding{h.k.Up, h.k.Add, h.k.Edit, h.k.Delete, h.k.Menu}
}

func (h listHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{{h.k.Up, h.k.Add, h.k.Edit, h.k.Delete}, {h.k.Help, h.k.Menu, h.k.Quit}}
}

type formHelp struct {
	k       KeyMap
	confirm bool
}

func (h formHelp) ShortHelp() []key.Binding {
	if h.confirm {
		return []key.Binding{h.k.Confirm, h.k.Deny}
	}
	return []key.Binding{h.k.NextField, h.k.Preview, h.k.Clear, h.k.Save, h.k.Close}
}

func (h formHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{{h.k.NextField, h.k.Preview, h.k.Clear}, {h.k.Save, h.k.Close, h.k.Quit}}
}

type errorHelp struct{ k KeyMap }

func (h errorHelp) ShortHelp() []key.Binding { return []key.Binding{h.k.Retry, h.k.Menu} }
func (h errorHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{{h.k.Retry, h.k.Menu, h.k.Quit}}
}

// Package app é o programa Bubble Tea raiz: roteia entre o menu e as telas de
// jogo e cuida do que é comum a todas (tamanho mínimo, saída com q).
package app

import (
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"fera-games/internal/audio"
	"fera-games/internal/game"
	"fera-games/internal/games/bandle"
	"fera-games/internal/httpx"
	"fera-games/internal/store"
	"fera-games/internal/tui/bandleui"
	"fera-games/internal/tui/menu"
	"fera-games/internal/tui/theme"
)

// Games são os jogos do hub, na ordem do menu. Os "em breve" ainda não têm engine.
var Games = []game.Info{
	{ID: "bandle", Name: "Bandle", Description: "adivinhe a música ouvindo um instrumento por vez", Available: true},
	{ID: "termo", Name: "Termo", Description: "a palavra do dia em 6 tentativas"},
	{ID: "songless", Name: "Songless", Description: "adivinhe a música pelo trecho"},
}

// Deps são as dependências externas, injetáveis nos testes.
type Deps struct {
	Bandle     *bandle.Game
	OpenPlayer func() (audio.Player, error)
	Now        func() time.Time
	Store      *store.Store // nil = não persiste
}

// DefaultDeps monta as dependências reais a partir da configuração.
func DefaultDeps(cfg Config) Deps {
	cache := &httpx.Cache{}
	if cfg.DataDir != "" {
		cache.Dir = filepath.Join(cfg.DataDir, "cache")
	}
	client := bandle.NewClient(httpx.New(30*time.Second), cache)
	open := audio.Open
	if cfg.Mute {
		open = func() (audio.Player, error) { return audio.NewSilent(), nil }
	}
	var st *store.Store
	if cfg.DataDir != "" {
		st = &store.Store{Dir: cfg.DataDir}
	}
	return Deps{Bandle: bandle.NewGame(client), OpenPlayer: open, Now: time.Now, Store: st}
}

type screen int

const (
	screenMenu screen = iota
	screenBandle
)

// Model é o estado raiz.
type Model struct {
	cfg    Config
	deps   Deps
	screen screen
	menu   menu.Model
	bandle *bandleui.Model
	player audio.Player
	width  int
	height int
}

// New monta o app com as dependências reais.
func New(cfg Config) Model { return NewWith(cfg, DefaultDeps(cfg)) }

// NewWith monta o app com dependências explícitas.
func NewWith(cfg Config, deps Deps) Model {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return Model{cfg: cfg, deps: deps, menu: menu.New(Games)}
}

func (m Model) Init() tea.Cmd { return nil }

// Update roteia a mensagem para a tela ativa.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.menu, _ = m.menu.Update(msg)
		if m.bandle != nil {
			b, _ := m.bandle.Update(msg)
			m.bandle = &b
		}
		return m, nil
	case menu.SelectedMsg:
		if msg.ID == "bandle" {
			return m.openBandle()
		}
		return m, nil
	case bandleui.BackMsg:
		m.screen = screenMenu
		return m, nil
	}

	var cmd tea.Cmd
	switch m.screen {
	case screenMenu:
		m.menu, cmd = m.menu.Update(msg)
	case screenBandle:
		b, c := m.bandle.Update(msg)
		m.bandle, cmd = &b, c
	}
	return m, cmd
}

// openBandle abre (ou retoma) a tela do Bandle. O dispositivo de áudio é
// aberto uma vez, na primeira partida; sem ele o jogo segue mudo com aviso.
func (m Model) openBandle() (tea.Model, tea.Cmd) {
	m.screen = screenBandle
	if m.bandle != nil {
		return m, m.bandle.Resume()
	}
	notice := ""
	if m.player == nil {
		p, err := m.deps.OpenPlayer()
		if err != nil {
			p = audio.NewSilent()
			notice = "sem áudio: " + err.Error()
		}
		m.player = p
	}
	b := bandleui.New(bandleui.Deps{
		Game: m.deps.Bandle, Player: m.player, Now: m.deps.Now, AudioNotice: notice, Store: m.deps.Store,
	})
	if m.width > 0 {
		b, _ = b.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}
	m.bandle = &b
	return m, b.Init()
}

// View desenha a tela ativa em tela cheia.
func (m Model) View() tea.View {
	var content string
	switch {
	case m.width > 0 && (m.width < theme.MinWidth || m.height < theme.MinHeight):
		content = theme.TooSmall(m.width, m.height)
	case m.screen == screenBandle && m.bandle != nil:
		content = m.bandle.View()
	default:
		content = m.menu.View()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "Fera Games"
	return v
}

// Package theme concentra as cores e estilos compartilhados pelas telas.
package theme

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// Tamanho mínimo do terminal; abaixo disso as telas mostram TooSmall.
const (
	MinWidth  = 80
	MinHeight = 24
)

var (
	Accent = lipgloss.Color("#F4A11B") // laranja do Bandle
	Green  = lipgloss.Color("#2E8A2B")
	Red    = lipgloss.Color("#D33F3F")
	Muted  = lipgloss.Color("8")

	Title    = lipgloss.NewStyle().Bold(true).Foreground(Accent)
	Subtitle = lipgloss.NewStyle().Foreground(Muted)
	Selected = lipgloss.NewStyle().Bold(true).Foreground(Accent)
	Normal   = lipgloss.NewStyle()
	Disabled = lipgloss.NewStyle().Foreground(Muted)
	Notice   = lipgloss.NewStyle().Foreground(Accent)
	Error    = lipgloss.NewStyle().Foreground(Red)
	Frame    = lipgloss.NewStyle().Padding(1, 2)
)

// TooSmall é o aviso mostrado quando o terminal é menor que o mínimo.
func TooSmall(w, h int) string {
	msg := fmt.Sprintf("Terminal %dx%d; o Fera Games precisa de pelo menos %dx%d.", w, h, MinWidth, MinHeight)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, Error.Render(msg))
}

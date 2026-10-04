package main

import (
	"charm.land/lipgloss/v2"
	"os"
	"strings"
)

// The terminal owns the neutral background. Restrained semantic accents
// have light/dark variants; generated prose uses color without decoration.
func (m *model) accent(light, dark string) lipgloss.Style {
	style := lipgloss.NewStyle()
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return style
	}
	color := light
	if m.dark {
		color = dark
	}
	return style.Foreground(lipgloss.Color(color))
}
func (m *model) judgeStyle() lipgloss.Style { return m.accent("#735597", "#C1A7DD") }
func (m *model) aiStyle() lipgloss.Style    { return m.accent("#526F91", "#A4BBD3") }
func (m *model) humanStyle() lipgloss.Style { return m.accent("#756047", "#C6B494").Bold(true) }
func (m *model) statusView(status string) string {
	style := dim
	if strings.HasPrefix(status, "Error:") || strings.HasPrefix(status, "Disconnected:") || strings.HasPrefix(status, "No new text") {
		style = m.accent("#A84F39", "#DB937C")
	}
	return style.Render(line(safe(status), m.width-2))
}

// Reuse the prose palette for help: blue for explanation, warm for key affordances.
func (m *model) helpStyle() lipgloss.Style { return m.accent("#526F91", "#A4BBD3") }
func (m *model) keysStyle() lipgloss.Style { return m.accent("#756047", "#C6B494") }

// The save indicator is compact and right-aligned; the workspace is in the header.
// Reserve its width before clipping the keyboard legend at narrow terminal sizes.
func (m *model) footerView(legend, saved string) string {
	width := max(0, m.width-2)
	right := lipgloss.Width(saved)
	if saved == "" {
		return " " + m.keysStyle().Render(line(legend, width))
	}
	left := max(0, width-right-2)
	return " " + m.keysStyle().Render(line(legend, left)) +
		strings.Repeat(" ", width-left-right) + dim.Render(saved)
}

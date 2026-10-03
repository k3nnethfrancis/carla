package main

import (
	"github.com/charmbracelet/x/ansi"
)

// Share argument guidance between the highlighted completion and /help. This is
// documentation, not a second parser; regression tests exercise the shown syntax.

func (m *model) commandHints() []string {
	choices := m.commandChoices()
	if len(choices) == 0 {
		return nil
	}
	selected := choices[min(m.commandIndex, len(choices)-1)]
	args := m.commandArguments(selected.id)
	if len(args) == 0 {
		return nil
	}
	// The selected row gives the purpose above these arguments. Wrap instead of
	// truncating so later flags (especially --eval and --loops) stay discoverable.
	lines := []string{}
	line := ""
	for _, arg := range args {
		if line != "" && ansi.StringWidth(line+" · "+arg) > max(1, m.width-4) {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " · "
		}
		line += arg
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Share argument guidance between the highlighted completion and /help. This is
// documentation, not a second parser; regression tests exercise the shown syntax.
func (m *model) commandArguments(id string) []string {
	switch id {
	case "loom", "continue":
		args := []string{}
		if id == "loom" {
			noun := "[alternatives]"
			if m.section == 3 && m.simSelection == nil {
				noun = "[conversations]"
			}
			if m.section == 3 && len(m.selectedConversations()) > 1 {
				noun = "[alternative sets]"
			}
			args = append(args, noun)
		}
		args = append(args, "--tokens N|Max", "--model alias")
		if m.section == 3 {
			args = append(args, "--turns N", "--visitor \"text\"", "--visitor-model alias")
		}
		args = append(args, "--eval \"name\"")
		if id == "loom" {
			args = append(args, "--loops N")
		}
		return args
	case "eval":
		return []string{"[\"evaluation name\"]", "--train-on-pass true|false"}
	case "models":
		if m.section == 3 {
			return []string{"[character|visitor]"}
		}
	case "configure":
		if m.section == 3 {
			return []string{"[setting]", "e.g. /config documents"}
		}
	}
	return nil
}

func (m *model) commandHelp(id string) string {
	text := commandDescriptions[id]
	if args := m.commandArguments(id); len(args) > 0 {
		text += "\n\n/" + commandName(action{id: id}) + " " + strings.Join(args, " · ")
	}
	if id == "loom" {
		text += "\n\nCount can also be written --count N or -n N. Flags accept --flag=value and any order; examples put --loops last. --eval runs after generation, without changing generation prompts. Multiple loops require a configured selection policy."
	}
	if id == "configure" && m.section == 3 {
		text += "\n\nSettings: documents, character_alias, visitor_alias, openings, turns, visitor_brief, character_settings, visitor_settings, character_template, visitor_template."
	}
	return text
}

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

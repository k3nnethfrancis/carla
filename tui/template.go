package main

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

var templateToken = regexp.MustCompile(`\{\{[^{}\r\n]*\}\}`)
var templateName = regexp.MustCompile(`^\{\{\s*[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)*\s*\}\}$`)
var terminalEscape = regexp.MustCompile(`^\x1b\[[0-?]*[ -/]*[@-~]`)

func explicitTemplate(kind, text string) string {
	if strings.Contains(text, "{{") {
		return text
	}
	switch kind {
	case "policy-judge-prompt", "selection_assessment_prompt":
		return text + "\n\nBehaviors:\n{{behaviors}}\n\nText:\n{{text}}"
	case "policy_prompt":
		return text + "\n\nBehaviors:\n{{behaviors}}\n\nCandidates:\n{{candidates}}\n\nAssessments:\n{{assessments}}"
	}
	return text
}
func templateHint(kind string) string {
	switch kind {
	case "policy-judge-prompt", "selection_assessment_prompt":
		return "Variables: {{behaviors}} · {{behaviors.name}} · {{behaviors.spec}} · {{text}} / {{history}}"
	case "policy_prompt":
		return "Variables: {{behaviors}} · {{candidates}} · {{assessments}} · {{spec}}"
	case "character_template", "visitor_template":
		return "Variables: {{history}} · {{anthology}} · {{visitor_brief}}"
	}
	return ""
}

// Add foreground syntax accents to the rendered viewport only. Keep its ANSI
// cursor/selection controls and exact cell layout; never modify editor contents.
func (m *model) templateEditorView() string {
	view := m.editor.View()
	if templateHint(m.editing) == "" {
		return view
	}
	spans := templateToken.FindAllStringIndex(ansi.Strip(view), -1)
	if len(spans) == 0 {
		return view
	}
	good := strings.Split(m.helpStyle().Render("X"), "X")[0]
	bad := strings.Split(m.accent("#A84F39", "#DB937C").Render("X"), "X")[0]
	var out strings.Builder
	pos, index := 0, 0
	plain := ansi.Strip(view)
	for len(view) > 0 {
		if esc := terminalEscape.FindString(view); esc != "" {
			out.WriteString(esc)
			view = view[len(esc):]
			continue
		}
		for index < len(spans) && pos >= spans[index][1] {
			index++
		}
		_, size := utf8.DecodeRuneInString(view)
		if index < len(spans) && pos >= spans[index][0] {
			style := good
			token := plain[spans[index][0]:spans[index][1]]
			if !templateName.MatchString(token) || !templateVariableKnown(m.editing, token) {
				style = bad
			}
			out.WriteString(style)
			out.WriteString(view[:size])
			if style != "" {
				out.WriteString("\x1b[39m")
			}
		} else {
			out.WriteString(view[:size])
		}
		pos += size
		view = view[size:]
	}
	return out.String()
}

// Keep the visible vocabulary aligned with the backend data-only schemas.
func templateVariableKnown(kind, token string) bool {
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(token, "{{"), "}}"))
	switch kind {
	case "character_template", "visitor_template":
		return name == "history" || name == "anthology" || name == "visitor_brief"
	case "policy_prompt":
		switch name {
		case "candidates", "candidates.node", "candidates.parent", "candidates.continuation", "assessments", "spec":
			return true
		}
	default:
		if name == "text" || name == "history" {
			return true
		}
	}
	return name == "behaviors" || name == "behaviors.name" || name == "behaviors.spec" || name == "behaviors.id"
}

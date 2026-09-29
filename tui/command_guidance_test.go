package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCommandPrefixesExposeContextualFlags(t *testing.T) {
	for _, section := range []int{1, 3} {
		m := evalFixture()
		m.section, m.focus, m.width, m.height = section, 3, 80, 24
		for _, input := range []string{"/lo", "/loom", "/generate", `/simulate 2 --eval "Voice"`} {
			m.command.SetValue(input)
			hints := strings.Join(m.commandHints(), " ")
			for _, flag := range []string{"--tokens", "--eval", "--model"} {
				if !strings.Contains(hints, flag) {
					t.Fatalf("%s: %s", input, hints)
				}
			}
			if strings.Contains(hints, "--turns") != (section == 3) {
				t.Fatal(hints)
			}
		}
	}
}

func TestCommandGuideFollowsHighlightedChoice(t *testing.T) {
	m := evalFixture()
	m.section, m.focus, m.width, m.height = 1, 3, 100, 30
	m.command.SetValue("/")
	for i, a := range m.commandChoices() {
		m.commandIndex = i
		hints := strings.Join(m.commandHints(), " ")
		if a.id == "loom" && !strings.Contains(hints, "--eval") {
			t.Fatal(hints)
		}
		if a.id == "branch" && hints != "" {
			t.Fatal("fork advertised generation flags", hints)
		}
		if m.commandHelp(a.id) == "" {
			t.Fatal("missing description", a.id)
		}
	}
}

func TestLoomFlagsAndFooterFitSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}, {120, 36}} {
		m := evalFixture()
		m.section, m.focus, m.width, m.height = 3, 3, size[0], size[1]
		m.command.SetValue("/lo")
		m.reflow()
		frame := ansi.Strip(m.View().Content)
		for _, s := range []string{"/loom", "--eval", "--loops", "--turns", "ENTER run"} {
			if !strings.Contains(frame, s) {
				t.Fatalf("%v missing %s:\n%s", size, s, frame)
			}
		}
		m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	}
}

func TestFullCommandHelpScrollsAndReturns(t *testing.T) {
	m := evalFixture()
	m.section, m.width, m.height = 3, 60, 18
	m.openHelp()
	parent := m.dialog
	for i, r := range parent.rows {
		if r.preview == "" {
			t.Fatal("missing help", r.id)
		}
		if r.id == "loom" {
			parent.index = i
		}
	}
	m.submitDialog()
	if m.dialog.kind != "help-detail" {
		t.Fatal(m.dialog.kind)
	}
	all := strings.Join(m.helpLines(), " ")
	for _, flag := range []string{"--eval", "--loops", "--count"} {
		if !strings.Contains(all, flag) {
			t.Fatal(flag, all)
		}
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "selection policy") || !strings.Contains(frame, "ESC back") {
		t.Fatal(frame)
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog != parent {
		t.Fatal("lost command list")
	}
}

func TestHelpWheelDoesNotMoveBackgroundTarget(t *testing.T) {
	m := evalFixture()
	m.width, m.height = 60, 18
	m.section = 4
	m.evalCollection = "set"
	m.focus = 0
	m.selected = 0
	m.openHelp()
	m.dialog.index = 1
	m.submitDialog()
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.dialog.index != 3 || m.selected != 0 {
		t.Fatalf("help=%d background=%d", m.dialog.index, m.selected)
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.dialog.index != 0 {
		t.Fatal(m.dialog.index)
	}
	m.closeDialog()
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.dialog.index != 4 || m.selected != 0 {
		t.Fatal("help list wheel leaked")
	}
}

func TestHelpHintsUseConfiguredBindings(t *testing.T) {
	m := evalFixture()
	m.height = 18
	m.width = 60
	m.data.Bindings = map[string]string{"nav.up": "k", "nav.down": "j", "nav.enter": "o", "nav.back": "q"}
	m.openHelp()
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "O details") || !strings.Contains(frame, "Q return") {
		t.Fatal(frame)
	}
	m.dialog.index = 1
	m.dialogKey(tea.KeyPressMsg{Code: 'o', Text: "o"})
	frame = ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "K/J scroll") || !strings.Contains(frame, "Q back") {
		t.Fatal(frame)
	}
	m.dialogKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.dialog.index != 1 {
		t.Fatal("configured scroll key ignored")
	}
	m.dialogKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.dialog.kind != "help" {
		t.Fatal("configured back key ignored")
	}
}

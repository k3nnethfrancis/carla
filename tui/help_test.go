package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestHelpSearchesFlagsAndAliasesAcrossStages(t *testing.T) {
	for _, section := range []int{0, 1, 2, 3, 4} {
		for _, query := range []string{"--turns", "--visitor-model", "--selection", "generate", "fork", "--train-on-pass"} {
			m := evalFixture()
			m.width, m.height, m.section = 60, 18, section
			m.openHelp()
			home := append([]row{}, m.dialog.rows...)
			m.dialogKey(tea.KeyPressMsg{Text: query})
			if len(m.dialog.rows) == 0 {
				t.Fatalf("section %d missing %s", section, query)
			}
			parent := m.dialog
			if cmd := m.submitDialog(); cmd != nil || m.pending || m.data.Busy {
				t.Fatal("help executed action")
			}
			if m.dialog.kind != "help-detail" {
				t.Fatal(m.dialog.kind)
			}
			if strings.HasPrefix(query, "--") && !strings.Contains(ansi.Strip(m.View().Content), query) {
				t.Fatal("flag search did not reveal explanation", query)
			}
			if !strings.Contains(strings.Join(m.helpLines(), " "), query) {
				t.Fatal("search result omitted matched flag/alias", query)
			}
			m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.dialog != parent || m.dialog.query != query {
				t.Fatal("lost search on return")
			}
			for range query {
				m.dialogKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
			}
			if len(m.dialog.rows) != len(home) || m.dialog.rows[0].id != "loom" {
				t.Fatal("search clear lost home")
			}
		}
	}
}

func TestHelpTopicsRestoreParentAndDoNotExecute(t *testing.T) {
	m := evalFixture()
	m.width, m.height = 60, 18
	m.openHelp()
	home := m.dialog
	for i, r := range home.rows {
		if !strings.HasPrefix(r.id, "group:") {
			continue
		}
		home.index = i
		m.submitDialog()
		topic := m.dialog
		if topic.parent != home || len(topic.rows) == 0 {
			t.Fatal("missing topic", r.id)
		}
		m.submitDialog()
		if m.dialog.kind != "help-detail" || m.pending {
			t.Fatal("expected read-only command guide")
		}
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.dialog.kind != "help-detail" || m.pending {
			t.Fatal("Enter executed example")
		}
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.dialog != topic {
			t.Fatal("lost topic")
		}
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.dialog != home || m.dialog.index != i {
			t.Fatal("lost home position")
		}
	}
}

func TestHelpLoomGuideFitsAndReachesEveryFlag(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {120, 36}} {
		m := evalFixture()
		m.width, m.height = size[0], size[1]
		m.openHelp()
		m.submitDialog()
		for i := 0; i < len(m.helpLines()); i++ {
			frame := ansi.Strip(m.View().Content)
			if len(strings.Split(frame, "\n")) > m.height {
				t.Fatal("frame too tall")
			}
			for _, line := range strings.Split(frame, "\n") {
				if ansi.StringWidth(line) > m.width {
					t.Fatal("frame too wide", line)
				}
			}
			m.dialogKey(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		all := ansi.Strip(strings.Join(m.helpLines(), "\n"))
		for _, s := range []string{"GENERATION FLAGS", "SIMULATOR FLAGS", "POLICY FLAGS", "--tokens", "--turns", "--loops", "--model", "--visitor-model", "--visitor", "--eval", "--selection", "--monitoring", "EXAMPLES", "No Simulator target"} {
			if !strings.Contains(all, s) {
				t.Fatal("missing", s)
			}
		}
	}
}

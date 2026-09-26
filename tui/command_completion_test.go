package main

import (
	tea "charm.land/bubbletea/v2"
	"testing"
)

func TestPartialCommandArrowSelection(t *testing.T) {
	m := fixture()
	m.focusCommand(true)
	for _, ch := range "loo" {
		m.commandKey(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.commandChoices()[m.commandIndex].id != "loom-policy" || m.command.Value() != "/loo" {
		t.Fatal("partial loom did not select", m.command.Value())
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyUp})
	// A partial command executes the selected action, not the literal prefix.
	m.section = 0
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.status != "Open the Branches or Simulator tab to run /loom" {
		t.Fatal("partial command not dispatched", m.status)
	}
	m.focusCommand(true)
	for _, ch := range "re" {
		m.commandKey(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	choices := m.commandChoices()
	if len(choices) < 2 {
		t.Fatal("expected multiple re matches", choices)
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.commandIndex != 1 {
		t.Fatal("first down must select second suggestion")
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.commandIndex != 2%len(choices) {
		t.Fatal("second down did not advance")
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.commandIndex != 1 {
		t.Fatal("up did not return")
	}
	m.commandKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.commandIndex != 0 || m.commandChoices()[0].id != "restart" {
		t.Fatal("typing should reset suggestion navigation")
	}
}

func TestExactCommandCanStillNavigateSuggestions(t *testing.T) {
	m := fixture()
	m.focusCommand(true)
	m.command.SetValue("/loom")
	m.commandHistory = []string{"/restart"}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.command.Value() != "/loom" || m.historyPosition != 0 {
		t.Fatal("complete command incorrectly routed to history")
	}
}

func TestPrefixEnterUsesAlreadySelectedTopMatch(t *testing.T) {
	for _, prefix := range []string{"l", "lo", "loo"} {
		m := fixture()
		m.section = 1
		m.focusCommand(true)
		for _, ch := range prefix {
			m.commandKey(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		}
		if m.commandIndex != 0 || m.commandChoices()[0].id != "loom" {
			t.Fatal(prefix, m.commandChoices())
		}
		// Switch to an unsupported tab to verify dispatch without starting inference.
		m.section = 2
		m.command.SetValue("/lo")
		m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.status != "Open the Branches or Simulator tab to run /loom" {
			t.Fatal(m.status)
		}
	}
}

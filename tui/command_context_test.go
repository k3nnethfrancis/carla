package main

import (
	tea "charm.land/bubbletea/v2"
	"testing"
)

func TestPageCommandsComeFirst(t *testing.T) {
	for _, tc := range []struct {
		section int
		first   string
	}{{0, "add"}, {1, "continue"}, {2, "snapshot"}, {3, "continue"}} {
		m := fixture()
		m.section = tc.section
		m.focus = 3
		m.command.SetValue("/")
		m.data.Selected = nil
		choices := m.commandChoices()
		if len(choices) == 0 || choices[0].id != tc.first {
			t.Fatalf("section %d: %v", tc.section, choices)
		}
		m.command.SetValue("/restart")
		if choices = m.commandChoices(); len(choices) != 1 || choices[0].id != "restart" {
			t.Fatal("global command lost")
		}
	}
}

func TestSimulatorCommandAliasesAndGuards(t *testing.T) {
	m := fixture()
	m.section = 3
	m.focus = 3
	m.width, m.height = 120, 36
	m.command.SetValue("/")
	choices := m.commandChoices()
	if choices[0].id != "continue" || choices[1].id != "loom" {
		t.Fatal(choices)
	}
	m.command.SetValue("/configure")
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog == nil || m.dialog.kind != "sim-config" {
		t.Fatal("configure did not open Simulator")
	}
	m.dialog = nil
	for _, busy := range []bool{false, true} {
		m.data.Busy = busy
		if !busy {
			m.editing = "document"
		}
		m.command.SetValue("/run")
		if len(m.commandChoices()) != 0 {
			t.Fatal("run available during busy/edit")
		}
		m.editing = ""
	}
	m.data.Busy = false
	m.section = 1
	m.command.SetValue("/configure")
	if choices := m.commandChoices(); len(choices) != 1 || choices[0].id != "configure" {
		t.Fatal("page configuration missing")
	}
	m.command.SetValue("/simulate")
	if len(m.commandChoices()) != 1 {
		t.Fatal("explicit cross-page command missing")
	}
}

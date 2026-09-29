package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestLoomRejectsConversationOptionsInDocumentViews(t *testing.T) {
	for _, section := range []int{1} {
		m := fixture()
		m.section = section
		if m.loom(generationOptions{Turns: 2}) != nil || !strings.Contains(m.status, "--turns") {
			t.Fatal("incompatible option accepted")
		}
	}
}

func TestSimulatorLoomRoutesCountAndTokenOverride(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m := fixture()
	m.client = &client{conn: left}
	m.width, m.height = 120, 36
	m.section = 3
	m.focus = 3
	// A previously viewed document must never become the Simulator command target.
	m.commandDocument = m.currentID()
	m.command.SetValue(`/loom 5 --turns 4 --tokens 64 --message "Where do paths meet?"`)
	cmd := m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no simulator command")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var request struct {
		Command string
		Args    struct {
			Count   int
			Tokens  int `json:"n_predict"`
			Turns   int
			Node    string
			Message string `json:"visitor"`
		}
	}
	if err := json.NewDecoder(right).Decode(&request); err != nil {
		t.Fatal(err)
	}
	<-done
	if request.Command != "simulator.run" || request.Args.Count != 5 || request.Args.Tokens != 64 || request.Args.Turns != 4 || request.Args.Node != "" || request.Args.Message != "Where do paths meet?" {
		t.Fatalf("wrong destination: %+v", request)
	}
}

func TestLoomRejectsTurnsOnBranches(t *testing.T) {
	m := fixture()
	m.section = 1
	if m.loom(generationOptions{Turns: 3}) != nil || !strings.Contains(m.status, "Simulator") {
		t.Fatal("branch turns silently ignored", m.status)
	}
}

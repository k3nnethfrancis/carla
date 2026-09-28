package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"net"
	"testing"
)

func TestUnifiedAliases(t *testing.T) {
	for _, section := range []int{0, 1, 2, 3} {
		m := fixture()
		m.section = section
		m.focus = 3
		for alias, want := range map[string]string{"config": "configure", "settings": "configure", "generate": "loom", "continue": "loom", "run": "loom", "branch": "branch", "fork": "branch", "remove": "remove", "delete": "remove"} {
			// Document-only actions require a valid document target; test their identity separately.
			id := m.canonicalCommand(alias)
			if id != want {
				t.Fatalf("%s → %s", alias, id)
			}
		}
		m.command.SetValue("/config")
		if choices := m.commandChoices(); len(choices) != 1 || choices[0].id != "configure" {
			t.Fatal(choices)
		}
		m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		want := "loom-config"
		if section == 3 {
			want = "sim-config"
		}
		if m.dialog == nil || m.dialog.kind != want {
			t.Fatal(section, m.dialog)
		}
	}
}

func TestLoopsParser(t *testing.T) {
	for _, input := range []string{"/loom 3 --tokens 512 --turns 2 --loops 4", "/loom --loops=4 --turns 2 3 --tokens 512"} {
		opts, err := parseGenerationOptions(input, "loom")
		if err != nil || opts.Count != 3 || opts.Tokens != 512 || opts.Turns != 2 || opts.Loops != 4 {
			t.Fatal(opts, err)
		}
	}
	for _, input := range []string{"/loom --loops 0", "/loom --loops -2", "/loom --loops 2 --loops 3", "/loom --loops"} {
		if _, err := parseGenerationOptions(input, "loom"); err == nil {
			t.Fatal(input)
		}
	}
}

func TestDocumentEntryPointsShareOneOperation(t *testing.T) {
	for _, section := range []int{0, 1, 2} {
		m := fixture()
		m.section = section
		m.width, m.height = 120, 36
		if section == 2 {
			m.data.Nodes[0].Kept = true
		}
		m.selected = 0
		left, right := net.Pipe()
		m.client = &client{conn: left}
		m.focus = 1
		m.reflow()
		cmd := m.loom(generationOptions{})
		if cmd == nil {
			t.Fatal(section, m.status)
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		var request struct {
			Command string
			Args    map[string]any
		}
		if err := json.NewDecoder(right).Decode(&request); err != nil {
			t.Fatal(err)
		}
		<-done
		left.Close()
		right.Close()
		if request.Command != "continue" || request.Args["count"] != float64(1) || request.Args["loops"] != float64(1) || m.section != 1 {
			t.Fatal(section, request)
		}
	}
}

func TestBareSimulatorLoomUsesOneReplyAndBatchRequiresSelection(t *testing.T) {
	m := fixture()
	m.section = 3
	m.focus = 0
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m.client = &client{conn: left}
	cmd := m.loom(generationOptions{})
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var request struct {
		Command string
		Args    map[string]any
	}
	json.NewDecoder(right).Decode(&request)
	<-done
	if request.Args["count"] != float64(1) || request.Args["turns"] != float64(1) || request.Args["loops"] != float64(1) {
		t.Fatal(request)
	}
	m.pending = false
	m.data.SimulationRuns = []runSummary{{ID: "batch", Count: 4, Status: "complete"}}
	for i, r := range m.rows() {
		if r.kind == "simulation" {
			m.selected = i
		}
	}
	cmd = m.loom(generationOptions{Count: 4})
	go func() { done <- cmd() }()
	json.NewDecoder(right).Decode(&request)
	<-done
	if request.Command != "simulator.open" || request.Args["run"] != "batch" {
		t.Fatal(request)
	}
}

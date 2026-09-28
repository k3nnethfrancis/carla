package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestSimulatorStreamAndConfiguration(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.section = 3
	data, _ := json.Marshal(simulationRun{ID: "run", Status: "running", Conversations: []simulationConversation{{Index: 0, Turns: []simulationTurn{{Role: "character", Model: localModel{Name: "Base"}, Status: "generating"}}}}})
	m.apply(event{Type: "simulation", Data: data})
	m.apply(event{Type: "simulation.token", Data: json.RawMessage(`{"run":"run","conversation":0,"turn":0,"text":"A path."}`)})
	if !strings.Contains(m.simulationText(), "A path.") || m.section != 3 {
		t.Fatal("missing streamed turn")
	}
	m.numberConfig("sim", "n_predict", 512, "character_settings")
	m.numberKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if m.dialog.fields[0].input.Value() != "Max" {
		t.Fatal("Max exposed as sentinel")
	}
	m.openSimulatorConfig()
	if len(m.dialog.rows) != 11 {
		t.Fatal("missing simulator controls")
	}
	if strings.Join(sectionNames, " ") != "Library Branches Anthology Simulator" {
		t.Fatal(sectionNames)
	}
}

func TestConfigEscapeReturnsOneLevel(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	m.openGrowConfig()
	grow := m.dialog
	grow.index = 0
	m.submitDialog()
	if m.dialog.kind != "models" {
		t.Fatal("model picker not opened")
	}
	m.dialogKey(esc)
	if m.dialog != grow || m.dialog.index != 0 {
		t.Fatal("lost Grow parent")
	}
	grow.index = 6 // output budget
	m.submitDialog()
	m.dialogKey(esc)
	if m.dialog != grow || m.dialog.index != 6 {
		t.Fatal("lost number picker parent")
	}
	grow.index = 2 // criteria editor
	m.submitDialog()
	if m.editing != "policy_spec" {
		t.Fatal("criteria editor not opened")
	}
	m.cancelEdit()
	if m.dialog != grow {
		t.Fatal("editor lost parent")
	}
	m.dialogKey(esc)
	if m.dialog != nil {
		t.Fatal("root must close")
	}

	m.data.SimulatorConfig = map[string]any{"character_settings": map[string]any{"n_predict": float64(512)}}
	m.openSimulatorConfig()
	root := m.dialog
	for i, r := range root.rows {
		if r.id == "character_settings" {
			root.index = i
		}
	}
	m.submitDialog()
	sampling := m.dialog
	if sampling.kind != "sim-sampling" {
		t.Fatal("missing sampling page")
	}
	m.submitDialog()
	m.dialogKey(esc)
	if m.dialog != sampling {
		t.Fatal("skipped sampling page")
	}
	m.dialogKey(esc)
	if m.dialog != root {
		t.Fatal("skipped Simulator config")
	}
	m.dialogKey(esc)
	if m.dialog != nil {
		t.Fatal("root must close")
	}

	m.openDialog("models")
	m.dialogKey(esc)
	if m.dialog != nil {
		t.Fatal("standalone picker must close")
	}
}

func TestOpeningConfigurationAndPromptEditor(t *testing.T) {
	m := fixture()
	m.data.SimulatorConfig = map[string]any{"opening_mode": "generated", "opening_prompt": "A test prompt", "visitor_alias": "base"}
	m.openSimulatorConfig()
	parent := m.dialog
	for i, r := range parent.rows {
		if r.id == "openings" {
			parent.index = i
		}
	}
	m.submitDialog()
	if m.dialog.kind != "sim-openings" || len(m.dialog.rows) != 5 {
		t.Fatal("missing opening controls")
	}
	openings := m.dialog
	m.submitDialog()
	if m.dialog.kind != "sim-opening-mode" {
		t.Fatal("missing mode picker")
	}
	m.closeDialog()
	if m.dialog != openings {
		t.Fatal("escape lost parent")
	}
	openings.index = 2
	m.submitDialog()
	if m.editing != "opening_prompt" || m.editor.Value() != "A test prompt" {
		t.Fatal("prompt not editable")
	}
	m.cancelEdit()
	if m.dialog != openings {
		t.Fatal("editor lost parent")
	}
}

func TestConcurrentSimulationTokensAndCapacity(t *testing.T) {
	m := fixture()
	m.data.Busy = true
	data, _ := json.Marshal(simulationRun{ID: "batch", Conversations: []simulationConversation{
		{Index: 0, Turns: []simulationTurn{{Role: "character"}}},
		{Index: 1, Turns: []simulationTurn{{Role: "character"}}},
	}})
	m.apply(event{Type: "simulation", Data: data})
	for _, raw := range []string{
		`{"run":"batch","conversation":1,"turn":0,"text":"B"}`,
		`{"run":"batch","conversation":0,"turn":0,"text":"A"}`,
		`{"run":"batch","conversation":1,"turn":0,"text":"2"}`,
	} {
		m.apply(event{Type: "simulation.token", Data: json.RawMessage(raw)})
	}
	if m.simulation.Conversations[0].Turns[0].Text != "A" || m.simulation.Conversations[1].Turns[0].Text != "B2" {
		t.Fatal("crossed streams")
	}
	m.apply(event{Type: "capacity", Data: json.RawMessage(`{"active":2,"waiting":1,"slots":4}`)})
	if m.capacityStatus != "2 active · 1 waiting · limit 4" {
		t.Fatal(m.capacityStatus)
	}
}

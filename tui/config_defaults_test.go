package main

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTopLevelDefaultsSaveToExistingSettings(t *testing.T) {
	for _, item := range []struct {
		section, index      int
		command, group, key string
	}{
		{1, 0, "configure", "settings", "n_predict"},
		{3, 0, "simulator.configure", "", "turns"},
		{3, 1, "simulator.configure", "character_settings", "n_predict"},
		{3, 2, "simulator.configure", "visitor_settings", "n_predict"},
	} {
		m := fixture()
		m.section = item.section
		m.data.SimulatorConfig = map[string]any{"turns": float64(2), "character_settings": map[string]any{"n_predict": float64(512), "temperature": .7}, "visitor_settings": map[string]any{"n_predict": float64(256), "temperature": .9}}
		m.openConfig()
		if item.section == 3 {
			m.submitDialog()
		}
		parent := m.dialog
		parent.index = item.index
		m.submitDialog()
		m.dialog.fields[0].input.SetValue("1024")
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.dialog != parent {
			t.Fatal("cancel lost config parent")
		}
		m.submitDialog()
		if m.dialog.fields[0].input.Value() == "1024" {
			t.Fatal("cancel persisted draft")
		}
		m.dialog.fields[0].input.SetValue("3")
		left, right := net.Pipe()
		m.client = &client{conn: left}
		cmd := m.submitDialog()
		done := make(chan tea.Msg, 1)
		go func() { done <- runPrimaryCommand(cmd) }()
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
		if request.Command != item.command || m.dialog != parent {
			t.Fatal(request, "lost parent or wrong command")
		}
		values := request.Args
		if item.group != "" {
			values = values[item.group].(map[string]any)
		}
		if values[item.key] != float64(3) {
			t.Fatal(request)
		}
		if strings.HasSuffix(item.group, "_settings") && values["temperature"] == nil {
			t.Fatal("token change erased sampling")
		}
		if item.section == 1 {
			m.data.Settings.Tokens = 3
		} else if item.group == "" {
			m.data.SimulatorConfig[item.key] = float64(3)
		} else {
			m.data.SimulatorConfig[item.group] = values
		}
		m.refreshConfig()
		if !strings.HasSuffix(m.dialog.rows[item.index].label, " · 3") {
			t.Fatal("stale displayed default")
		}
	}
}

func TestSimulatorPromptSubmenuAndShortcuts(t *testing.T) {
	m := fixture()
	m.section = 3
	m.openConfig()
	parent := m.dialog
	for i, r := range parent.rows {
		if r.id == "prompts" {
			parent.index = i
		}
	}
	m.submitDialog()
	if m.dialog.title != "Prompts" || len(m.dialog.rows) != 3 {
		t.Fatal("missing prompt submenu")
	}
	m.refreshConfig()
	if m.dialog.title != "Prompts" || m.dialog.parent != parent {
		t.Fatal("refresh lost submenu")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog != parent {
		t.Fatal("Escape skipped config")
	}
	m.dialog = nil
	_, handled := m.directConfig("configure", "config character_template")
	if !handled || m.editing != "character_template" {
		t.Fatal("legacy prompt shortcut lost")
	}
}

func TestSimulatorContextPickerTargetsSpeakerModel(t *testing.T) {
	m := fixture()
	m.section = 3
	m.data.SimulatorConfig = map[string]any{"character_alias": "second"}
	m.data.ModelContexts = map[string]modelContext{"second": {Configured: 8192, Native: 32768}}
	m.openSimulatorContexts()
	parent := m.dialog
	for i, r := range parent.rows {
		if r.id == "character_context" {
			parent.index = i
			if !strings.Contains(r.label, "8192") || !strings.Contains(r.preview, "32,768") {
				t.Fatal(r)
			}
		}
	}
	m.submitDialog()
	m.numberKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if m.dialog.fields[0].input.Value() != "Max" {
		t.Fatal("native context option missing")
	}
	left, right := net.Pipe()
	m.client = &client{conn: left}
	cmd := m.submitDialog()
	done := make(chan tea.Msg, 1)
	go func() { done <- runPrimaryCommand(cmd) }()
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
	if request.Command != "simulator.configure" || request.Args["character_context"] != float64(0) || len(request.Args) != 1 || m.dialog != parent {
		t.Fatal(request)
	}
}

func TestNumericConfigTypingCursorAndUnitSteps(t *testing.T) {
	m := fixture()
	m.numberConfig("sim", "turns", 2, "")
	m.dialogKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	m.dialogKey(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m.dialogKey(tea.KeyPressMsg{Code: '1', Text: "13"})
	if m.dialog.fields[0].input.Value() != "13" {
		t.Fatal(m.dialog.fields[0].input.Value())
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.dialogKey(tea.KeyPressMsg{Code: '2', Text: "2"})
	if m.dialog.fields[0].input.Value() != "123" {
		t.Fatal("cursor editing", m.dialog.fields[0].input.Value())
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.dialog.fields[0].input.Value() != "124" {
		t.Fatal("increment")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.dialog.fields[0].input.Value() != "123" {
		t.Fatal("decrement")
	}
	m.dialog.fields[0].input.SetValue("1.5")
	if cmd := m.submitDialog(); cmd != nil || m.status != "Enter a whole number" {
		t.Fatal("fraction silently truncated")
	}
	for _, key := range []string{"n_predict", "context", "monitor_interval_tokens"} {
		m.numberConfig("sim", key, 512, "")
		m.numberKey(tea.KeyPressMsg{Code: tea.KeyUp})
		if m.dialog.fields[0].input.Value() != "513" {
			t.Fatal(key, m.dialog.fields[0].input.Value())
		}
	}
}

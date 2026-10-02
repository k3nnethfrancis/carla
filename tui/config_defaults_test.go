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

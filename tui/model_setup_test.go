package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSetupUsesDialogsAndPreservesInputOnBack(t *testing.T) {
	m := fixture()
	m.width = 100
	m.height = 30
	m.setupEvent(json.RawMessage(`{"stage":"home","presets":["8B","14B","30B"]}`))
	home := m.dialog
	if home.kind != "setup-home" || len(home.rows) != 6 {
		t.Fatal("missing setup choices")
	}
	home.index = 4
	m.submitDialog()
	if m.dialog.kind != "setup-input" {
		t.Fatal("local path field missing")
	}
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.dialog == nil || m.dialog.fields[0].input.Value() != "/" {
		t.Fatal("slash stole path input")
	}
	m.dialog.fields[0].input.SetValue("/tmp/my model.gguf")
	input := m.dialog
	m.submitDialog()
	if m.dialog.kind != "setup-role" {
		t.Fatal("missing model type")
	}
	m.closeDialog()
	if m.dialog != input || m.dialog.fields[0].input.Value() != "/tmp/my model.gguf" {
		t.Fatal("back lost path")
	}
	m.closeDialog()
	if m.dialog != home {
		t.Fatal("back skipped setup choices")
	}
	home.index = 5
	m.submitDialog()
	if m.dialog != nil {
		t.Fatal("skip did not open app")
	}
}

func TestSetupConfirmationAndNarrowLayout(t *testing.T) {
	m := fixture()
	m.width = 80
	m.height = 24
	parent := &dialog{kind: "setup-home", title: "Set up a model"}
	m.setupBusy(parent, map[string]any{"kind": "base"}, "Checking")
	m.setupEvent(json.RawMessage(`{"stage":"plans","plans":[{"name":"model.gguf","repo":"owner/repo","revision":"abc","files":["model.gguf"],"size":1073741824,"license":"apache-2.0"}]}`))
	if m.dialog.kind != "setup-confirm" {
		t.Fatal("missing explicit download confirmation")
	}
	view := m.renderDialog()
	if !strings.Contains(view, "1.00 GiB") || !strings.Contains(view, "apache-2.0") {
		t.Fatal(view)
	}
	m.closeDialog()
	if m.dialog.kind != "setup-files" {
		t.Fatal("Escape must return one level")
	}
	m.closeDialog()
	if m.dialog != parent {
		t.Fatal("Escape should return source picker")
	}
}

func TestImportDialogKeepsSlashesAndRefreshesLibrarySelection(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section = 110, 32, 0
	m.perform("import")
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.dialog == nil || m.dialog.fields[0].input.Value() != "/" {
		t.Fatal("slash must be literal inside file path")
	}
	m.apply(event{Type: "error", Data: json.RawMessage(`{"message":"Choose an existing text file"}`)})
	if m.dialog == nil || m.dialog.fields[0].input.Value() != "/" {
		t.Fatal("failed import should retain fields for correction")
	}
	m.sources = append(m.sources, source{Key: "new-doc", Title: "New document"})
	m.apply(event{Type: "library.imported", Data: json.RawMessage(`{"key":"new-doc","title":"New document"}`)})
	if m.dialog != nil || m.section != 0 || m.targetRow().id != "new-doc" {
		t.Fatal("import should reveal its new library entry")
	}
}

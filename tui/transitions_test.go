package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"
)

type capturedRequest struct {
	ID, Command string
	Args        map[string]json.RawMessage
}

func captureCommand(t *testing.T, m *model, action func() tea.Cmd) capturedRequest {
	t.Helper()
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m.client = &client{conn: left}
	cmd := action()
	if cmd == nil {
		t.Fatalf("no command: %s", m.status)
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	right.SetReadDeadline(time.Now().Add(time.Second))
	var req capturedRequest
	if err := json.NewDecoder(right).Decode(&req); err != nil {
		t.Fatal(err)
	}
	if _, ok := (<-done).(sent); !ok {
		t.Fatal("request failed")
	}
	return req
}
func stateEvent(t *testing.T, m *model, id string) event {
	t.Helper()
	data, err := json.Marshal(m.data)
	if err != nil {
		t.Fatal(err)
	}
	return event{Type: "state", ID: id, Data: data}
}

func TestImportedLibraryDispatchesResetAndAllowsNextAction(t *testing.T) {
	m := fixture()
	m.section = 0
	m.pending = true
	req := captureCommand(t, m, func() tea.Cmd {
		return m.apply(event{Type: "library.imported", Data: json.RawMessage(`{"key":"gunkel","title":"Paths"}`)})
	})
	if req.Command != "seed.clear" {
		t.Fatalf("expected reset, got %s", req.Command)
	}
	m.apply(stateEvent(t, m, req.ID))
	m.perform("find")
	if m.pending || !m.searching {
		t.Fatal("next action still blocked")
	}
}
func TestSaveRetainsDraftUntilMatchingReply(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 1, 1
	m.reflow()
	m.beginEdit("document")
	m.editor.SetValue("A new 🙂 route")
	m.editor.CursorEnd()
	offset := m.cursorOffset()
	req := captureCommand(t, m, m.saveEditor)
	if req.Command != "node.edit" || m.editing != "document" {
		t.Fatal("save prematurely closed editor")
	}
	m.apply(stateEvent(t, m, "background"))
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !m.pending || m.editing != "document" || m.editor.Value() != "A new 🙂 route" || m.cursorOffset() != offset {
		t.Fatal("unrelated state or input changed pending draft")
	}
	m.apply(event{Type: "error", ID: req.ID, Data: json.RawMessage(`{"message":"disk full"}`)})
	if m.pending || m.editing != "document" || m.editor.Value() != "A new 🙂 route" || m.cursorOffset() != offset {
		t.Fatal("error lost draft or cursor")
	}
	req = captureCommand(t, m, m.saveEditor)
	m.apply(stateEvent(t, m, req.ID))
	if m.editing != "" || m.editRequest != "" || m.pending {
		t.Fatal("successful retry did not close edit")
	}
}
func TestSaveDisconnectKeepsDraftRecoverable(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section = 120, 36, 1
	m.reflow()
	m.beginEdit("document")
	m.editor.SetValue("draft")
	captureCommand(t, m, m.saveEditor)
	m.Update(failure{errors.New("connection closed")})
	if m.editing != "document" || m.editor.Value() != "draft" || m.editRequest != "" {
		t.Fatal("disconnect discarded draft")
	}
	if m.saveEditor() != nil || m.editor.Value() != "draft" {
		t.Fatal("disconnected save changed draft")
	}
}
func TestNotesActionsIgnoreHiddenSelection(t *testing.T) {
	for _, action := range []string{"remove", "delete"} {
		t.Run(action, func(t *testing.T) {
			m := fixture()
			m.width, m.height, m.section = 120, 36, 2
			a := *m.data.Current
			a.Kept = true
			b := a
			b.ID = "second"
			m.data.Nodes = []node{a, b}
			m.data.Current = &b
			m.branchSelection = map[string]bool{a.ID: true}
			m.openNotes(true)
			if action == "delete" {
				m.selectionAction(action)
				ids := m.dialog.args["nodes"].([]string)
				if len(ids) != 1 || ids[0] != b.ID {
					t.Fatalf("wrong deletion targets: %v", ids)
				}
				return
			}
			req := captureCommand(t, m, func() tea.Cmd { return m.contextualAction(action) })
			var ids []string
			json.Unmarshal(req.Args["nodes"], &ids)
			if len(ids) != 1 || ids[0] != b.ID {
				t.Fatalf("displayed %s but acted on %v", b.ID, ids)
			}
		})
	}
}
func TestGridOpeningKeepsSidebarAndPreviewAligned(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 1, 1
	second := *m.data.Current
	second.ID = "second"
	second.Parent = m.currentID()
	second.Text = "SECOND"
	m.data.Nodes = append(m.data.Nodes, second)
	m.collapsed[m.currentID()] = true
	m.loomTiles = []loomTile{{ID: m.currentID(), Status: "complete"}, {ID: second.ID, Status: "complete"}}
	m.loomGrid = true
	m.reflow()
	captureCommand(t, m, func() tea.Cmd { return m.openGridTile(1) })
	m.data.Current = &second
	m.apply(stateEvent(t, m, ""))
	if m.targetRow().id != second.ID || m.previewTarget() != nil {
		t.Fatal("state reopened stale sidebar document")
	}
}
func TestConversationBranchShortcutUsesSameRequestAsPalette(t *testing.T) {
	for _, shortcut := range []bool{false, true} {
		m := simulatorFixture()
		m.openGridTile(1)
		req := captureCommand(t, m, func() tea.Cmd {
			if shortcut {
				_, cmd := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
				return cmd
			}
			m.focusCommand(true)
			m.command.SetValue("/branch")
			return m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		})
		if req.Command != "simulator.fork" {
			t.Fatalf("shortcut=%v: %s", shortcut, req.Command)
		}
		var index int
		json.Unmarshal(req.Args["conversation"], &index)
		if index != 1 {
			t.Fatalf("forked wrong conversation: %d", index)
		}
	}
}

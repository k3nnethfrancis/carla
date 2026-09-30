package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func fixture() *model {
	m := newModel(&client{})
	m.data = state{Workspace: workspace{"Gunkel exploration", "/workspace/gunkel"}, Settings: settings{3, 256, 1, .98, 1}, Models: []localModel{{"Qwen3-14B Base", "base"}}, ModelAlias: "base"}
	m.sources = []source{{Key: "gunkel", Title: "Gunkel · Paths", Passages: []passage{{ID: "paths", Text: "Paths that lead toward a beginning."}}}, {Key: "meditations", Title: "Meditations"}, {Key: "tractatus", Title: "Tractatus"}}
	m.expanded["gunkel"] = true
	text := "Paths that lead toward a beginning.\n\nThe path looked back at the lines it had traced. A line is a memory of movement; a path is an invitation to move again.\n\nWhere the line ended, the path began."
	m.data.Current = &node{ID: "abcdef123456", Kind: "generated", Text: text, Model: "Qwen3-14B Base", Origins: []origin{{0, 34, "source"}, {34, len([]rune(text)), "ai"}}}
	m.data.Selected = []string{"gunkel:paths"}
	m.data.Nodes = []node{*m.data.Current}
	m.status = "Saved locally · /workspace/gunkel"
	m.focus = 1
	return m
}

func TestLayoutAndUnicode(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {60, 18}, {80, 24}, {100, 32}, {144, 42}, {220, 60}} {
		for range []bool{false} {
			for _, inspector := range []bool{false, true} {
				for focus := 0; focus < 4; focus++ {
					m := fixture()
					m.width, m.height = size[0], size[1]
					m.focus = focus
					m.showInspector = inspector
					m.data.Current.Text += "\n日本語 🙂 👩‍💻 é مرحبا بالعالم mixed text"
					m.inspection = strings.Repeat("Long inspection with 日本語 and 👩‍💻. ", 30)
					m.reflow()
					frame := ansi.Strip(m.View().Content)
					lines := strings.Split(frame, "\n")
					if len(lines) > m.height {
						t.Fatalf("%v: %d rows exceed height", size, len(lines))
					}
					for _, line := range lines {
						if ansi.StringWidth(line) > m.width {
							t.Fatalf("%v: %d cells exceed width: %q", size, ansi.StringWidth(line), line)
						}
					}

				}
			}
		}
	}
}

func TestGoldenFrames(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 36}, {144, 42}} {
		m := fixture()
		m.width, m.height = size[0], size[1]
		m.showInspector = size[0] >= 132
		m.inspection = "Generation\nQwen3-14B Base\n\nRaw completion\nNo system prompt\n\nSource: Gunkel / Paths"
		m.reflow()
		got := ansi.Strip(m.View().Content) + "\n"
		path := filepath.Join("testdata", fmt.Sprintf("loom-%dx%d.txt", size[0], size[1]))
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			os.MkdirAll("testdata", 0755)
			os.WriteFile(path, []byte(got), 0644)
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Fatalf("frame differs: %s", path)
		}
	}
}
func TestFocusAndKeys(t *testing.T) {
	m := fixture()
	m.width, m.height = 80, 24
	m.focus = 0
	m.reflow()
	key := func(code rune) { m.Update(tea.KeyPressMsg{Code: code}) }
	key(tea.KeyTab)
	if m.focus != 1 {
		t.Fatal(m.focus)
	}
	key(tea.KeyTab)
	if m.focus != 3 {
		t.Fatal(m.focus)
	}
	m.Update(tea.PasteMsg{Content: "/mod"})
	if len(m.commandChoices()) != 1 {
		t.Fatal("command filter failed")
	}
	key(tea.KeyEnter)
	if m.dialog == nil || m.dialog.kind != "models" {
		t.Fatal("model picker missing")
	}
	key(tea.KeyEscape)
	m.beginEdit("document")
	m.Update(tea.PasteMsg{Content: "日本語\nمرحبا"})
	if !strings.Contains(m.editor.Value(), "日本語") {
		t.Fatal("paste lost")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.editing != "" {
		t.Fatal("edit not closed")
	}
	if textOffset("🙂a\né文", 1, 2) != 5 {
		t.Fatal("offset is not codepoint based")
	}
}
func TestFormAndDialogBounds(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}, {144, 42}} {
		m := fixture()
		m.width, m.height = size[0], size[1]
		m.openDialog("settings")
		m.reflow()
		for i := range m.dialog.fields {
			m.dialog.field = i
			frame := ansi.Strip(m.View().Content)
			if len(strings.Split(frame, "\n")) > m.height {
				t.Fatal("dialog too tall")
			}
			if !strings.Contains(frame, m.dialog.fields[i].label) {
				t.Fatalf("focused field not visible at %v", size)
			}
		}
	}
}

func TestCommandInputAndDraftSafety(t *testing.T) {
	m := fixture()
	m.width, m.height = 60, 18
	m.focusCommand(true)
	m.Update(tea.PasteMsg{Content: "continue"})
	if m.pending || m.command.Value() != "/continue" {
		t.Fatal("paste executed a command")
	}
	m.command.SetValue("/")
	m.commandIndex = 0
	for i, a := range m.commandChoices() {
		if a.id == "branches" {
			m.commandIndex = i
		}
	}
	m.reflow()
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "/branches") || !strings.Contains(frame, "TAB") {
		t.Fatal("suggestions/footer clipped", frame)
	}
	m.command.SetValue("")
	m.beginEdit("document")
	draft := m.editor.Value()
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.PasteMsg{Content: "/model"})
	if len(m.commandChoices()) != 0 {
		t.Fatal("unsafe workspace/model change available during draft")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editor.Value() != draft || m.editing != "document" {
		t.Fatal("draft lost")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != 1 || m.editor.Value() != draft {
		t.Fatal("could not return to draft")
	}
}

func TestSectionsHelpAndBranchTree(t *testing.T) {
	m := fixture()
	m.width, m.height = 80, 24
	m.focusCommand(false)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.sectionFocus || m.focus != 1 {
		t.Fatal("palette did not restore originating pane")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.section != 1 || m.focus != 0 || m.sectionFocus {
		t.Fatal("section navigation failed")
	}
	m.data.Nodes = []node{{ID: "root", Status: "complete"}, {ID: "a", Parent: "root", Status: "complete"}, {ID: "b", Parent: "root", Status: "complete"}, {ID: "aa", Parent: "a", Status: "complete", Kept: true}}
	rows := m.rows()
	if rows[2].id != "aa" || rows[3].id != "b" {
		t.Fatal("siblings not grouped", rows)
	}
	m.branchArrow("left")
	if len(m.rows()) != 1 || !strings.Contains(m.rows()[0].label, "▸") {
		t.Fatal("collapse failed")
	}
	m.branchArrow("right")
	m.branchArrow("right")
	if m.selected != 1 || len(m.rows()) != 4 {
		t.Fatal("expand/descend failed")
	}
	m.branchArrow("left")
	m.branchArrow("left")
	if m.selected != 0 {
		t.Fatal("parent navigation failed")
	}
	m.section = 2
	if len(m.rows()) != 1 || m.rows()[0].id != "aa" {
		t.Fatal("collapse hid kept documents")
	}
	for _, size := range [][2]int{{60, 18}, {80, 24}, {144, 42}} {
		m.width, m.height = size[0], size[1]
		m.openHelp()
		if len(m.dialog.rows) < 25 {
			t.Fatal("help missing commands")
		}
		for i, r := range m.dialog.rows {
			if r.preview == "" {
				t.Fatal("missing description", r)
			}
			m.dialog.index = i
			frame := ansi.Strip(m.View().Content)
			if !strings.Contains(frame, r.label) || len(strings.Split(frame, "\n")) > m.height {
				t.Fatal("help clipped", frame)
			}
		}
	}
}

func TestThemeNoColor(t *testing.T) {
	m := fixture()
	for _, dark := range []bool{true, false} {
		m.dark = dark
		if ansi.Strip(m.statusView("Error: failure")) != line("Error: failure", m.width-2) {
			t.Fatal("error text changed")
		}
	}
	t.Setenv("NO_COLOR", "1")
	if m.aiStyle().GetForeground() != lipgloss.NewStyle().GetForeground() || m.humanStyle().GetForeground() != lipgloss.NewStyle().GetForeground() {
		t.Fatal("NO_COLOR ignored")
	}
}

func TestContextualCollectionActions(t *testing.T) {
	m := fixture()
	m.width, m.height = 80, 24
	has := func(id string) bool {
		for _, a := range m.contextualActions() {
			if a.id == id {
				return true
			}
		}
		return false
	}
	m.section = 0
	m.selected = 1
	if has("add") || !has("remove") || has("keep") {
		t.Fatal("selected source actions incorrect")
	}
	m.data.Selected = nil
	if !has("add") || has("remove") {
		t.Fatal("unselected source actions incorrect")
	}
	m.section = 1
	m.selected = 0
	if !has("keep") || has("remove") {
		t.Fatal("branch actions incorrect")
	}
	m.data.Nodes[0].Kept = true
	m.section = 2
	if has("keep") || !has("remove") || !has("continue") {
		t.Fatal("kept actions incorrect")
	}
	m.data.Nodes = nil
	if has("continue") || has("remove") {
		t.Fatal("empty collection has target actions")
	}
}

func TestRestartArguments(t *testing.T) {
	got := restartArgs([]string{"--workspace", "old", "--models", "custom.json", "--project=old"}, "/current/workspace")
	if strings.Join(got, "|") != "--models|custom.json|--project|/current/workspace" {
		t.Fatal(got)
	}
	m := fixture()
	m.width, m.height = 80, 24
	m.focusCommand(false)
	m.command.SetValue("/restart")
	if len(m.commandChoices()) != 1 {
		t.Fatal("restart missing")
	}
	cmd := m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.restarting || cmd == nil {
		t.Fatal("restart not requested")
	}
	m.editing = "document"
	m.command.SetValue("/restart")
	if len(m.commandChoices()) != 1 {
		t.Fatal("restart missing during draft")
	}
}

func TestFocusRingAndOuterNavigation(t *testing.T) {
	for _, inspector := range []bool{false, true} {
		m := fixture()
		m.width, m.height = 100, 32
		m.showInspector = inspector
		ring := []int{0, 1, 3}
		if inspector {
			ring = []int{0, 1, 2, 3}
		}
		for i, focus := range ring {
			m.focus = focus
			m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			if m.focus != ring[(i+1)%len(ring)] || m.sectionFocus {
				t.Fatal("forward ring", m.focus)
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
			if m.focus != focus || m.sectionFocus {
				t.Fatal("reverse ring", m.focus)
			}
		}
		m.focusCommand(true)
		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.sectionFocus || m.command.Value() != "" {
			t.Fatal("first escape must dismiss input")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if !m.sectionFocus {
			t.Fatal("second escape must move outward")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if m.sectionFocus || m.focus != 0 {
			t.Fatal("down must enter list")
		}
		m.data.Busy = true
		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if !m.sectionFocus || m.pending {
			t.Fatal("navigation must not cancel inference")
		}
	}
}

func TestBindingEditorAndRouting(t *testing.T) {
	m := fixture()
	m.width, m.height = 80, 24
	m.openKeys()
	m.keysKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.keysKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !strings.Contains(m.keyNotice, "Already assigned") {
		t.Fatal("conflict accepted")
	}
	m.keysKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.keysKey(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if m.keyDraft["continue"] != "ctrl+y" || m.bindingKey("continue") != "ctrl+r" {
		t.Fatal("draft applied prematurely")
	}
	m.keysKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.bindingKey("continue") != "ctrl+r" {
		t.Fatal("cancel changed bindings")
	}
	m.data.Bindings = map[string]string{"nav.next": "ctrl+j", "models": "ctrl+y"}
	m.focus = 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != 0 {
		t.Fatal("old binding still active")
	}
	m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if m.focus != 1 {
		t.Fatal("new navigation binding inactive")
	}
	m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if m.dialog == nil || m.dialog.kind != "models" {
		t.Fatal("new action binding inactive")
	}
	m.dialog = nil
	m.focusCommand(false)
	m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.command.Value() != "c" || m.pending {
		t.Fatal("typing invoked action")
	}
	for _, size := range [][2]int{{60, 18}, {80, 24}, {144, 42}} {
		m.width, m.height = size[0], size[1]
		m.openKeys()
		for i := range m.dialog.rows {
			m.dialog.index = i
			if !strings.Contains(ansi.Strip(m.View().Content), strings.ToUpper(bindingValue(m.keyDraft, bindingCatalog()[i]))) {
				t.Fatal("binding clipped")
			}
		}
	}
}

func TestInlineDocumentCursor(t *testing.T) {
	m := fixture()
	m.width, m.height = 80, 24
	m.section = 1
	m.focus = 1
	m.reflow()
	end := len([]rune(m.currentText()))
	if m.cursorOffset() != end {
		t.Fatal("cursor not at end", m.cursorOffset(), end)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.cursorOffset() != end-1 {
		t.Fatal("left did not move cursor", m.cursorOffset())
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursorOffset() >= end-1 {
		t.Fatal("up did not move cursor")
	}
	text := m.currentText()
	pos := m.cursorOffset()
	m.beginEdit("document")
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	expected := string([]rune(text)[:pos]) + "x" + string([]rune(text)[pos:])
	if m.editor.Value() != expected || m.currentText() != text || m.editing != "document" {
		t.Fatal("inline edit lost cursor or mutated source")
	}
	m.Update(tea.PasteMsg{Content: "日本語"})
	if !strings.Contains(m.editor.Value(), "x日本語") {
		t.Fatal("paste lost")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !m.cursorActive() {
		t.Fatal("cannot return to document cursor")
	}
}

func TestContinueShortcutUsesDocumentCursor(t *testing.T) {
	for _, siblings := range []bool{false, true} {
		left, right := net.Pipe()
		m := fixture()
		m.client = &client{conn: left}
		m.width, m.height = 80, 24
		m.section = 1
		m.focus = 1
		m.reflow()
		m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		expected := m.cursorOffset()
		key := 'r'
		if siblings {
			key = 'b'
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: key, Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("shortcut did not send generation")
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		var request struct {
			Command string
			Args    struct {
				Node   string
				Offset int
				Branch bool
			}
		}
		if err := json.NewDecoder(right).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if (!siblings && (request.Command != "continue" || request.Args.Offset != expected || !request.Args.Branch)) || (siblings && request.Command != "node.fork") || request.Args.Node != m.currentID() {
			t.Fatalf("wrong cursor request: %+v", request)
		}
		<-done
		left.Close()
		right.Close()
	}
}

func TestBranchSelectionActions(t *testing.T) {
	m := fixture()
	m.width, m.height = 80, 24
	m.section = 1
	m.focus = 0
	m.data.Nodes = []node{{ID: "root", Kind: "source", Status: "complete"}, {ID: "child", Parent: "root", Status: "complete"}, {ID: "leaf", Parent: "child", Status: "complete"}, {ID: "sibling", Parent: "root", Status: "complete"}}
	m.selected = 1
	m.toggleTarget()
	if len(m.selectedBranches()) != 1 || !strings.Contains(m.rows()[1].label, "✓") {
		t.Fatal("selection checkbox missing")
	}
	m.collapsed["child"] = true
	m.selectionAction("delete")
	if m.dialog.kind != "delete" || m.dialog.rows[0].id != "cancel" || len(m.dialog.args["expected"].([]string)) != 2 {
		t.Fatal("delete scope/confirmation incorrect")
	}
	m.submitDialog()
	if m.dialog != nil || len(m.selectedBranches()) != 1 {
		t.Fatal("cancel changed selection")
	}
	m.activate()
	if m.dialog.kind != "selection" {
		t.Fatal("enter should open actions")
	}
	m.dialog = nil
	m.selectionAction("clear")
	if len(m.selectedBranches()) != 0 || len(m.data.Nodes) != 4 {
		t.Fatal("clear must only uncheck")
	}
	m.selected = 1
	m.toggleTarget()
	for _, size := range [][2]int{{60, 18}, {80, 24}} {
		m.width, m.height = size[0], size[1]
		m.reflow()
		if strings.Contains(ansi.Strip(m.View().Content), "[ Delete… ]") {
			t.Fatal("selection strip should not appear")
		}

	}
}

func TestDocumentTailAfterResize(t *testing.T) {
	m := fixture()
	m.section = 1
	m.focus = 1
	m.width, m.height = 120, 36
	text := strings.Repeat("A long line of text wraps across the pane. ", 40) + " FINAL-TEXT"
	m.data.Current.Text = text
	m.data.Current.Origins = []origin{{0, len([]rune(text)), "source"}}
	m.reflow()
	m.revealCursor()
	m.width, m.height = 60, 18
	m.reflow()
	if !strings.Contains(ansi.Strip(m.View().Content), "FINAL-TEXT") {
		t.Fatal("last document text is cut off after resize")
	}
}

func TestDocumentSelectionDoesNotIncludeAncestryDescendants(t *testing.T) {
	m := fixture()
	m.section, m.focus = 1, 0
	m.data.Nodes = []node{{ID: "root"}, {ID: "child", Parent: "root"}, {ID: "leaf", Parent: "child"}}
	m.collapsed["root"] = true
	m.toggleTarget()
	if len(m.selectedBranches()) != 1 || !m.branchSelection["root"] {
		t.Fatal(m.branchSelection)
	}
	m.toggleTarget()
	if len(m.selectedBranches()) != 0 {
		t.Fatal(m.branchSelection)
	}
}

func TestLongDialogDescriptionFits(t *testing.T) {
	m := fixture()
	m.width, m.height = 60, 18
	m.dialog = &dialog{kind: "help", title: "Details", rows: []row{{label: "Selected item", preview: strings.Repeat("description ", 14) + "LAST DETAIL"}}}
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "LAST DETAIL") || !strings.Contains(frame, "ESC return") {
		t.Fatal("description or footer cut off", frame)
	}
}

func TestSlashLeavesEditorWithoutChangingDraft(t *testing.T) {
	m := fixture()
	m.width, m.height = 100, 32
	m.section = 1
	m.focus = 1
	m.reflow()
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.beginEdit("document")
	m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	draft := m.editor.Value()
	pos := textOffset(draft, m.editor.Line(), m.editor.Column())
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.focus != 3 || m.command.Value() != "/" || m.editor.Value() != draft || m.editing != "document" {
		t.Fatal("slash lost draft or entered text")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != 1 || textOffset(m.editor.Value(), m.editor.Line(), m.editor.Column()) != pos {
		t.Fatal("draft cursor not restored")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.dialog = &dialog{kind: "help"}
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.dialog != nil || m.focus != 3 {
		t.Fatal("slash did not leave window")
	}
}

func TestSelectionLabelsStayAligned(t *testing.T) {
	m := fixture()
	m.section = 1
	before := m.rows()[0].label
	m.branchSelection[m.data.Nodes[0].ID] = true
	after := m.rows()[0].label
	if strings.Contains(before, "[") || strings.Contains(after, "]") {
		t.Fatal("checkbox brackets remain")
	}
	if ansi.StringWidth(before) != ansi.StringWidth(after) {
		t.Fatal("label shifted")
	}
	m.section = 0
	m.data.Selected = nil
	before = m.rows()[1].label
	m.data.Selected = []string{"gunkel:paths"}
	after = m.rows()[1].label
	if ansi.StringWidth(before) != ansi.StringWidth(after) || strings.Contains(before, "[") {
		t.Fatal("library alignment changed")
	}
}

func TestCollapsedSourceSelectionIsVisible(t *testing.T) {
	m := fixture()
	m.section = 0
	m.expanded["gunkel"] = false
	m.sources = append(m.sources, source{Key: "tract", Title: "Tractatus", Passages: []passage{{ID: "1", Text: "The world is everything that is the case."}}})
	m.data.Selected = []string{"gunkel:paths", "tract:1"}
	rows := m.rows()
	if !strings.Contains(rows[0].label, "✓") || !strings.Contains(rows[len(rows)-2].label, "1 selected") {
		t.Fatal("collapsed selections hidden")
	}
	if !strings.Contains(m.seedSummary(), "Gunkel") || !strings.Contains(m.seedSummary(), "Tractatus") {
		t.Fatal("combined source summary incomplete")
	}
}

func TestLibraryReturnResetsSelectionsAcrossRoutes(t *testing.T) {
	for _, route := range []string{"command", "arrows", "click"} {
		m := fixture()
		m.width, m.height = 100, 32
		m.section = 1
		m.focus = 0
		m.reflow()
		switch route {
		case "command":
			m.perform("library")
		case "find":
			m.perform("find")
		case "arrows":
			m.sectionFocus = true
			m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		case "click":
			m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: 2})
		}
		if m.section != 0 || len(m.data.Selected) != 0 || !m.pending {
			t.Fatal("selection reset missing", route)
		}
		if len(m.data.Nodes) != 1 {
			t.Fatal("reset removed branches")
		}
	}
	m := fixture()
	m.section = 1
	m.pending = true
	m.switchSection(0)
	if !m.resetSeeds {
		t.Fatal("reset lost during pending request")
	}
	m.pending = false
	if m.flushSeedReset() == nil || m.resetSeeds {
		t.Fatal("deferred reset did not flush")
	}
}

func TestEmptyGenerationFeedback(t *testing.T) {
	m := fixture()
	m.data.Nodes[0].Status = "empty"
	m.section = 1
	if !strings.Contains(m.rows()[0].label, "no new text") {
		t.Fatal("empty branch not labeled")
	}
	m.apply(event{Type: "operation", Data: json.RawMessage(`{"stage":"complete","message":"No new text — model ended immediately (EOS)"}`)})
	if !strings.Contains(m.status, "EOS") {
		t.Fatal("completion message lost")
	}
}

func TestSlashGenerationPreservesCursorAndDraft(t *testing.T) {
	for _, draft := range []bool{false} {
		for _, name := range []string{"continue", "generate", "loom"} {
			left, right := net.Pipe()
			m := fixture()
			m.client = &client{conn: left}
			m.width, m.height = 80, 24
			m.section = 1
			m.focus = 1
			m.reflow()
			m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
			expected := m.cursorOffset()
			text := ""
			if draft {
				m.Update(tea.KeyPressMsg{Code: 'Z', Text: "Z"})
				expected = textOffset(m.editor.Value(), m.editor.Line(), m.editor.Column())
				text = m.editor.Value()
			}
			m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			m.command.SetValue("/" + name)
			choices := m.commandChoices()
			canonical := "continue"
			if name == "loom" {
				canonical = "loom"
			}
			if len(choices) == 0 || choices[0].id != canonical {
				t.Fatal("missing command", name, draft)
			}
			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("command did not execute")
			}
			done := make(chan tea.Msg, 1)
			go func() { done <- cmd() }()
			var request struct {
				Command string
				Args    struct {
					Node, Text string
					Offset     int
					Branch     bool
				}
			}
			if err := json.NewDecoder(right).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Command != "continue" || request.Args.Offset != expected || request.Args.Text != text || !request.Args.Branch {
				t.Fatalf("wrong command prefix: %+v", request)
			}
			<-done
			left.Close()
			right.Close()
		}
	}
}

func TestGenerateSearchIncludesContinue(t *testing.T) {
	m := fixture()
	m.section = 1
	m.focusCommand(true)
	for _, query := range []string{"/g", "/gen", "/generate"} {
		m.command.SetValue(query)
		found := map[string]int{}
		for _, a := range m.commandChoices() {
			found[a.id]++
		}
		if found["generate"] != 0 || found["continue"] != 1 {
			t.Fatal("alias discovery missing or duplicated", query, found)
		}
	}
	m.command.SetValue("/")
	count := 0
	for _, a := range m.commandChoices() {
		if a.id == "loom" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("unfiltered list duplicates continue")
	}
}

func TestLoomCountParsingAndCursorRequest(t *testing.T) {
	for _, input := range []string{"/loom --count 5", "/loom -n 5", "/loom --count=5"} {
		options, err := parseGenerationOptions(input, "loom")
		if err != nil || options.Count != 5 {
			t.Fatal(input, options, err)
		}
	}
	for _, input := range []string{"/loom --count 0", "/loom --count -2", "/loom --count x", "/loom --count", "/loom --count 2 extra", "/loom --unknown 2"} {
		if _, err := parseGenerationOptions(input, "loom"); err == nil {
			t.Fatal("accepted invalid count", input)
		}
	}
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m := fixture()
	m.client = &client{conn: left}
	m.width, m.height = 80, 24
	m.section = 1
	m.focus = 1
	m.reflow()
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})

	text := ""
	offset := m.cursorOffset()
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.command.SetValue("/loom --count 0")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editing != "" || m.command.Value() != "/loom --count 0" {
		t.Fatal("invalid input lost draft")
	}
	m.command.SetValue("/loom 5 --tokens 1024")
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("loom did not run")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var request struct {
		Args struct {
			Count, Offset int
			Tokens        int `json:"n_predict"`
			Text          string
			Branch        bool
		}
	}
	if err := json.NewDecoder(right).Decode(&request); err != nil {
		t.Fatal(err)
	}
	if request.Args.Tokens != 1024 {
		t.Fatal("token range missing", request)
	}
	if request.Args.Count != 5 || request.Args.Offset != offset || request.Args.Text != text || !request.Args.Branch || m.data.Settings.Count != 3 {
		t.Fatal("wrong override or cursor", request)
	}
	<-done
}

func TestDraftRequiresExplicitSaveOrCancel(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 1, 1
	m.reflow()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog == nil || m.dialog.rows[0].label != "Edit here" {
		t.Fatal("missing edit action")
	}
	m.submitDialog()
	m.Update(tea.KeyPressMsg{Code: 'Z', Text: "Z"})
	draft := m.editor.Value()
	m.focusCommand(true)
	for _, name := range []string{"continue", "loom", "branch"} {
		m.command.SetValue("/" + name)
		if len(m.commandChoices()) != 0 {
			t.Fatal("draft offers implicit save", name)
		}
	}
	m.command.SetValue("/cancel")
	if len(m.commandChoices()) != 1 {
		t.Fatal("cancel missing")
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editing != "" || m.focus != 1 || !m.notesOpen || m.currentText() == draft {
		t.Fatal("cancel left document or saved draft")
	}
}

func TestWorkspaceChangeClearsDocumentState(t *testing.T) {
	m := fixture()
	m.editing, m.editNode, m.commandDocument = "document", "old", "old"
	m.editor.SetValue("old draft")
	m.inspection, m.showInspector = "old prompt", true
	m.collapsed["old"] = true
	next := m.data
	next.Workspace.Path = "/different-workspace"
	payload, _ := json.Marshal(next)
	m.apply(event{Type: "state", Data: payload})
	if m.editing != "" || m.editNode != "" || m.commandDocument != "" || m.editor.Value() != "" || m.inspection != "" || m.showInspector || len(m.collapsed) != 0 {
		t.Fatal("document state leaked across workspaces")
	}
}

func TestRecoveryControlsIgnoreBackendState(t *testing.T) {
	for _, command := range []string{"restart", "exit", "quit", "ctrl+q"} {
		for _, editing := range []string{"", "document", "policy_spec"} {
			m := fixture()
			m.data.Workspace.Path = t.TempDir()
			m.width, m.height = 80, 24
			m.pending, m.disconnected, m.data.Busy = true, true, true
			m.editing, m.editNode = editing, "draft-parent"
			m.editor.SetValue("Unsaved 🙂")
			var cmd tea.Cmd
			if command == "ctrl+q" {
				m.dialog = &dialog{kind: "keys"}
				_, cmd = m.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
			} else {
				m.focusCommand(true)
				m.command.SetValue("/" + command)
				cmd = m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			}
			if cmd == nil {
				t.Fatalf("%s blocked in %s", command, editing)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("exit queued through backend")
			}
			if m.restarting != (command == "restart") {
				t.Fatal("wrong exit mode")
			}
			files, _ := filepath.Glob(filepath.Join(m.data.Workspace.Path, "recovered-drafts", "*.json"))
			if editing != "" {
				if len(files) != 1 {
					t.Fatal("draft not preserved")
				}
				raw, err := os.ReadFile(files[0])
				if err != nil {
					t.Fatal(err)
				}
				var draft struct{ Kind, Node, Text string }
				if err = json.Unmarshal(raw, &draft); err != nil {
					t.Fatal(err)
				}
				if draft.Text != "Unsaved 🙂" || draft.Kind != editing || draft.Node != "draft-parent" {
					t.Fatal("wrong recovery data")
				}
			} else if len(files) != 0 {
				t.Fatal("unexpected recovery file")
			}
		}
	}
}

func TestSettingsShowResolvedDefault(t *testing.T) {
	m := fixture()
	m.data.NativeContext = 32768
	m.data.ModelContext = 0
	m.data.Settings.Tokens = -1
	m.openDialog("settings")
	if m.dialog.fields[3].input.Value() != "Default" || !strings.Contains(m.dialog.fields[3].label, "32,768 tokens") {
		t.Fatal("missing actual model default")
	}
	if m.dialog.fields[0].input.Value() != "Max" {
		t.Fatal("raw sentinel exposed")
	}
}

func TestSettingsPickersAndSteppers(t *testing.T) {
	m := fixture()
	m.data.NativeContext = 32768
	m.width, m.height = 80, 24
	m.openDialog("settings")
	d := m.dialog
	before := d.fields[0].input.Value()
	m.dialogKey(tea.KeyPressMsg{Code: '9', Text: "9"})
	if d.fields[0].input.Value() != before {
		t.Fatal("settings accept free text")
	}
	d.field = 0
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(d.rows) == 0 || d.rows[0].id != "Max" {
		t.Fatal("output picker absent")
	}
	d.index = 0
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if d.fields[0].input.Value() != "Max" || d.adjusting {
		t.Fatal("selection not applied")
	}
	d.field = 2
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	for i := 0; i < 20; i++ {
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	if d.fields[2].input.Value() != "1.00" {
		t.Fatal("top-p upper bound")
	}
	for i := 0; i < 20; i++ {
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	if d.fields[2].input.Value() != "0.00" {
		t.Fatal("top-p lower bound")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if d.fields[2].input.Value() != "0.98" {
		t.Fatal("cancel changed setting")
	}
	d.field = 3
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(d.rows[0].label, "32,768") {
		t.Fatal("default count missing")
	}
}

func TestGenerationTokenArguments(t *testing.T) {
	for input, want := range map[string]generationOptions{
		"/continue --tokens 512":            {Tokens: 512},
		"/generate --tokens=90":             {Tokens: 90},
		"/loom 5 --tokens 1024":             {Count: 5, Tokens: 1024},
		"/loom --tokens Max -n 3 --turns 4": {Count: 3, Tokens: -1, Turns: 4},
	} {
		id := strings.TrimPrefix(strings.Fields(input)[0], "/")
		got, err := parseGenerationOptions(input, id)
		if err != nil || got != want {
			t.Fatal(input, got, err)
		}
		args := map[string]any{}
		got.apply(args)
		if args["n_predict"] != want.Tokens {
			t.Fatal(args)
		}

	}
	for _, input := range []string{"/continue 0", "/continue 10-20", "/loom 3 --tokens 256-512", "/continue 1-2-3", "/continue --count 2", "/loom 3 --tokens 2 --tokens 4"} {
		if _, err := parseGenerationOptions(input, strings.TrimPrefix(strings.Fields(input)[0], "/")); err == nil {
			t.Fatal("accepted", input)
		}
	}
}

func TestCommandHistoryAndGuides(t *testing.T) {
	m := fixture()
	m.data.Workspace.Path = t.TempDir()
	m.width, m.height = 80, 24
	m.focusCommand(false)
	for _, entry := range []struct{ text, id string }{{"/continue 512", "continue"}, {"/loom 5 --tokens 1024", "loom"}} {
		m.command.SetValue(entry.text)
		m.recordCommand(action{id: entry.id})
	}
	m.command.SetValue("")
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.command.Value() != "/loom 5 --tokens 1024" {
		t.Fatal("missing newest")
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.command.Value() != "/continue 512" {
		t.Fatal("missing older")
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.command.Value() != "" {
		t.Fatal("draft not restored")
	}
	m.loadCommandHistory()
	if len(m.commandHistory) != 2 {
		t.Fatal("history did not persist")
	}
	m.command.SetValue("/continue 100")
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyUp})
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.command.Value() != "/continue 100" {
		t.Fatal("unfinished command lost")
	}
	m.section = 1
	for _, size := range [][2]int{{60, 18}, {80, 24}} {
		m.width, m.height = size[0], size[1]
		m.command.SetValue("/loom")
		m.reflow()
		frame := ansi.Strip(m.View().Content)
		if !strings.Contains(frame, "--tokens") || len(strings.Split(frame, "\n")) > m.height {
			t.Fatal("syntax guide clipped", frame)
		}
	}
	// Partial-name arrows still select autocomplete, not old commands.
	m.command.SetValue("/co")
	m.historyPosition = 0
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.command.Value() != "/co" {
		t.Fatal("autocomplete replaced by history")
	}
}

func TestDocumentNotesNavigation(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 2, 0
	m.data.Current.Kept = true
	m.data.Nodes[0].Kept = true
	m.data.Annotations = []annotation{
		{ID: "mine", Node: m.currentID(), Note: "Local note", Start: 50, End: 50},
		{ID: "other", Node: "other-version", Note: "Other version"},
	}
	m.reflow()
	m.activate()
	if !m.notesOpen || m.focus != 1 || m.editing != "" {
		t.Fatal("opening kept document did not open notes and editor")
	}
	if m.selected != 0 {
		t.Fatal("opening a document should highlight Back to Branches")
	}
	rows := m.rows()
	if len(rows) != 4 || rows[0].label != "← Branches" || rows[1].label != "+ New note" {
		t.Fatal(rows)
	}
	m.selected = 3
	m.activateNote()
	if m.cursorOffset() != 50 {
		t.Fatal("note anchor not revealed", m.cursorOffset())
	}
	if m.dialog == nil || m.dialog.kind != "note-edit" || m.dialog.fields[0].input.Value() != "Local note" {
		t.Fatal("note editor missing")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.backFromNotes()
	if m.notesOpen || m.section != 1 || m.targetRow().id != m.currentID() {
		t.Fatal("back lost branch target")
	}
	m.focus = 1
	m.beginEdit("document")
	m.Update(tea.KeyPressMsg{Code: 'Z', Text: "Z"})
	text := m.editor.Value()
	m.focusCommand(true)
	m.command.SetValue("/notes")
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.notesOpen || m.focus != 0 || m.editor.Value() != text || m.editing != "document" {
		t.Fatal("notes lost draft")
	}
	m.selected = 1
	m.activateNote()
	if m.dialog != nil || m.editor.Value() != text {
		t.Fatal("dirty draft note must wait for explicit save")
	}
	m.cancelEdit()
	if m.editing != "" || !m.notesOpen {
		t.Fatal("cancel left document")
	}

}

func TestNotesAnchorInLongUnicodeDocument(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 1, 1
	text := strings.Repeat("A long line 🙂 and more text.\n", 250) + "Target"
	m.data.Current.Text = text
	m.reflow()
	offset := len([]rune(text)) - 6
	moveTextCursor(&m.navigator, offset)
	m.reflow()
	m.revealCursor()
	if m.cursorOffset() != offset || !strings.Contains(ansi.Strip(m.document.View()), "Target") {
		t.Fatal("anchor off screen")
	}
}

func TestExistingNoteSaveUpdatesInPlace(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m := fixture()
	m.client = &client{conn: left}
	m.width, m.height, m.section = 120, 36, 1
	m.data.Annotations = []annotation{{ID: "note-1", Node: m.currentID(), Note: "Before", Start: 3, End: 3}}
	m.openNotes(true)
	m.selected = 3
	m.activateNote()
	m.dialog.fields[0].input.SetValue("After")
	cmd := m.submitDialog()
	if cmd == nil {
		t.Fatal("missing save")
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
	if request.Command != "note.update" || request.Args["id"] != "note-1" || request.Args["note"] != "After" {
		t.Fatal(request)
	}
	if m.notePending {
		t.Fatal("edit treated as new note")
	}
}

func TestVersionPreviewScrollsToChangeOnSelection(t *testing.T) {
	for _, width := range []int{80, 140} {
		m := fixture()
		m.width, m.height, m.section, m.focus = width, 32, 1, 0
		prefix := strings.Repeat("Long source 日本語 text that wraps across the preview pane. ", 180) + "\n"
		n := node{ID: "new-version", Kind: "generated", Text: prefix + "NEW CONTINUATION\n" + strings.Repeat("more text\n", 50), ChangeOffset: len([]rune(prefix))}
		next := m.data
		next.Current = &n
		next.Nodes = []node{n}
		raw, _ := json.Marshal(next)
		m.apply(event{Type: "state", Data: raw})
		if width < 90 {
			m.focus = 1
			m.reflow()
		}
		if m.document.YOffset() == 0 || !strings.Contains(ansi.Strip(m.document.View()), "NEW CONTINUATION") {
			t.Fatalf("width %d: selection did not reveal continuation: %s", width, m.document.View())
		}
		if m.cursorOffset() != n.ChangeOffset {
			t.Fatal("cursor should enter at previewed change")
		}
		m.focus = 0
		m.document.SetYOffset(5)
		m.apply(event{Type: "state", Data: raw})
		if m.document.YOffset() != 5 {
			t.Fatal("refreshing the same selection must preserve manual scrolling")
		}
	}
}

func TestBranchIdentifiersCompactOnlyNestedAutomaticNames(t *testing.T) {
	m := fixture()
	m.section = 1
	m.data.Nodes = []node{
		{ID: "root", Title: "paths-branch-0001", Label: "paths-branch-0001", Status: "complete"},
		{ID: "gen", Parent: "root", Title: "paths-gen-0001", Label: "paths-gen-0001", Status: "complete", Kept: true},
		{ID: "edit", Parent: "gen", Title: "My name", Label: "paths-edit-0001", Status: "complete"},
	}
	rows := m.branchRows()
	if !strings.Contains(rows[0].label, "paths-branch-0001") || strings.Contains(rows[1].label, "paths-") || !strings.Contains(rows[1].label, "gen-0001") || !strings.Contains(rows[2].label, "My name") {
		t.Fatalf("unexpected labels: %#v", rows)
	}
	m.section = 2
	if !strings.Contains(m.branchRows()[0].label, "paths-gen-0001") {
		t.Fatal("anthology needs source context")
	}
}

func TestEmptyDocumentPreview(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {120, 36}} {
		m := fixture()
		m.section = 1
		m.width, m.height = size[0], size[1]
		m.data.Current.Text = ""
		m.data.Current.Origins = nil
		m.reflow()
		body := ansi.Strip(m.renderDocument(m.document.Width()))
		if strings.TrimSpace(body) != "Empty document" {
			t.Fatalf("empty document rendered %q", body)
		}
		rows := strings.Split(body, "\n")
		middle := (m.document.Height() - 1) / 2
		if !strings.Contains(rows[middle], "Empty document") && !strings.Contains(rows[min(middle+1, len(rows)-1)], "Empty document") {
			t.Fatal("empty state not centered")
		}
		m.editing = "document"
		if strings.Contains(m.renderDocument(m.document.Width()), "Empty document") {
			t.Fatal("placeholder appeared in editor")
		}
		m.editing = ""
		m.data.Current = nil
		if !strings.Contains(m.renderDocument(m.document.Width()), "Start with a seed") {
			t.Fatal("missing document lost onboarding")
		}
	}
}

func TestNavigationFooterFitsNarrowPane(t *testing.T) {
	m := fixture()
	for _, section := range []int{0, 1, 2, 3, 4} {
		m.section = section
		body := ansi.Strip(m.navigation(rect{0, 0, 26, 18}))
		for _, row := range strings.Split(body, "\n") {
			if ansi.StringWidth(row) > 22 {
				t.Fatalf("section %d: clipped footer %q", section, row)
			}
		}
		if (section == 1 || section == 2) && !strings.Contains(body, "ENTER actions") {
			t.Fatalf("missing action hint: %q", body)
		}
		if len(strings.Split(body, "\n")) > 14 {
			t.Fatal("footer exceeds panel body")
		}
	}
}

func TestNotesDocumentActions(t *testing.T) {
	m := fixture()
	m.section = 1
	m.width, m.height = 120, 36
	m.openNotes(true)
	m.selected = 2
	m.activateNote()
	if m.editing != "document" || m.focus != 1 {
		t.Fatal("missing document editor")
	}
	for _, r := range m.noteRows() {
		if r.kind == "document-delete" {
			t.Fatal("delete belongs in selection actions")
		}
	}
}

func TestMouseFocusesNotesActionsWithoutActivating(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section = 120, 36, 1
	m.openNotes(true)
	r := m.layout().panels[0].box
	for i := 1; i < 3; i++ {
		m.notesClick(i, r)
		if m.selected != i || m.focus != 0 || m.dialog != nil || m.editing != "" {
			t.Fatalf("click activated row %d", i)
		}
	}
	m.activateNote()
	if m.editing != "document" {
		t.Fatal("Enter should activate focused edit")
	}
}
func TestMouseFocusesBranchWithoutEntering(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section = 120, 36, 1
	m.reflow()
	r := m.layout().panels[0].box
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: r.x + 12, Y: r.y + 3})
	if m.notesOpen || m.focus != 0 || m.editing != "" || m.dialog != nil {
		t.Fatal("row click entered document")
	}
	m.activate()
	if !m.notesOpen {
		t.Fatal("Enter should still open document")
	}
}

func TestVersionPreviewWhileGenerationBusy(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 1, 0
	prefix := strings.Repeat("Source line\n", 100)
	n := node{ID: "busy-preview", Text: prefix + "LATEST CHANGE", Kind: "generated", ChangeOffset: len([]rune(prefix))}
	next := m.data
	next.Busy = true
	next.Current = &n
	next.Nodes = []node{n}
	raw, _ := json.Marshal(next)
	m.apply(event{Type: "state", Data: raw})
	if !strings.Contains(ansi.Strip(m.document.View()), "LATEST CHANGE") {
		t.Fatal("busy state suppressed preview focus")
	}
}
func TestRapidDocumentPreviewCatchesLatestSelection(t *testing.T) {
	m := fixture()
	m.section, m.focus, m.width, m.height = 1, 0, 120, 36
	first := *m.data.Current
	second := node{ID: "second", Kind: "source", Text: "Second document"}
	m.data.Nodes = []node{first, second}
	m.pending = true
	m.selected = 1
	if m.previewTarget() != nil || !m.previewSelectionPending {
		t.Fatal("navigation not queued")
	}
	next := m.data
	raw, _ := json.Marshal(next)
	req := captureCommand(t, m, func() tea.Cmd { return m.apply(event{Type: "state", Data: raw}) })
	if req.Command != "node.open" || string(req.Args["node"]) != `"second"` {
		t.Fatalf("wrong preview: %#v", req)
	}
}

func TestEditorCompletionAndBackspace(t *testing.T) {
	m := fixture()
	m.section = 1
	m.width, m.height = 120, 36
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEnter, Mod: tea.ModSuper}, {Code: tea.KeyEnter, Mod: tea.ModCtrl}, {Code: 's', Mod: tea.ModCtrl}} {
		if !m.saveKey(key) {
			t.Fatalf("save key not recognized: %s", key.String())
		}
	}
	if m.saveKey(tea.KeyPressMsg{Code: tea.KeyEnter}) {
		t.Fatal("plain Enter must remain newline")
	}
	m.editDocumentWithNotes()
	m.editor.SetValue("AB")
	m.editor.CursorEnd()
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.editor.Value() != "A" || m.dialog != nil {
		t.Fatal("Backspace did not delete text", m.editor.Value())
	}
}

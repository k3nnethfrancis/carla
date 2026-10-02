package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestBranchesFitExpandedRowsWithoutHoverResize(t *testing.T) {
	for _, width := range []int{60, 120, 180} {
		m := fixture()
		m.width, m.height, m.section, m.focus = width, 36, 1, 0
		for _, nameWidth := range []int{27, 48, 200} {
			m.data.Nodes = []node{{ID: "root", Title: "doc-1"}, {ID: "child", Parent: "root", Title: strings.Repeat("x", nameWidth)}}
			got := m.layout().panels[0].box.w
			if got != 28 || len(m.layout().panels) != 2 {
				t.Fatalf("long shallow names must leave room for document: %+v", m.layout())
			}
			m.selected = 1
			if m.layout().panels[0].box.w != got {
				t.Fatal("hover resized pane")
			}
		}
		m.collapsed["root"] = true
		m.selected = 0
		m.reflow()
		if m.layout().panels[0].box.w != 24 {
			t.Fatal("collapse did not restore preview")
		}
	}
}

func TestFullBranchPaneSkipsHiddenDocumentAndAllowsNotes(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 1, 0
	m.data.Nodes = []node{{ID: "root", Title: "doc-1"}}
	parent := "root"
	for i := 0; i < 120; i++ {
		id := strings.Repeat("n", i+1)
		m.data.Nodes = append(m.data.Nodes, node{ID: id, Parent: parent, Title: "continue-1-doc-1"})
		parent = id
	}
	m.reflow()
	m.cycleFocus(1)
	if m.focus != 3 {
		t.Fatalf("tab landed in hidden pane %d", m.focus)
	}
	m.cycleFocus(-1)
	if m.focus != 0 {
		t.Fatalf("back tab landed in hidden pane %d", m.focus)
	}
	m.notesOpen = true
	m.reflow()
	if len(m.layout().panels) != 2 {
		t.Fatal("notes must retain document pane")
	}
	m.notesOpen = false
	m.editing = "document"
	m.focus = 1
	m.reflow()
	if len(m.layout().panels) != 2 {
		t.Fatal("editing must retain document pane")
	}
}

func TestCompactBranchNamesRetainFullMarquee(t *testing.T) {
	t.Setenv("REDUCE_MOTION", "")
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 1, 0
	name := "continue-1-edit-3-doc-1-日本語"
	m.data.Nodes = []node{{ID: "root", Title: "doc-1"}, {ID: "child", Parent: "root", Title: name}}
	m.selected = 1
	m.reflow()
	panel := m.layout().panels[0].box
	r := m.rows()[1]
	if !strings.HasSuffix(m.branchRowText(r, panel), "…") {
		t.Fatal("unfocused overflow must show an ellipsis")
	}
	if m.syncLabelScroll() == nil || !strings.Contains(m.labelScroll.target.text, name) {
		t.Fatal("focused marquee must retain the full name")
	}
	for m.labelScroll.offset < m.labelScroll.limit {
		m.labelScroll.advance(labelScrollTick(m.labelScroll.generation))
	}
	if !strings.HasSuffix(strings.TrimSpace(m.scrollingLabel(m.navigationLabel(r, panel), panel.w-4)), "日本語") {
		t.Fatal("marquee must reach the name's end")
	}
	if m.layout().panels[0].box.w != panel.w {
		t.Fatal("revealing the title must not resize the tree")
	}
}

func TestBranchHorizontalViewportMatchesMouseTargets(t *testing.T) {
	m := fixture()
	m.width, m.height, m.section, m.focus = 60, 36, 1, 0
	m.data.Nodes = []node{{ID: "root", Title: "doc-1"}}
	parent := "root"
	for i := 0; i < 12; i++ {
		id := strings.Repeat("n", i+1)
		m.data.Nodes = append(m.data.Nodes, node{ID: id, Parent: parent, Title: strings.Repeat("continue-1-", 12) + "doc-1"})
		parent = id
	}
	m.selected = 10
	m.reflow()
	before := m.branchHorizontalOffset(m.layout().panels[0].box)
	m.scrollBranches(8)
	panel := m.layout().panels[0].box
	rows := m.rows()
	r := rows[m.selected]
	offset := m.branchHorizontalOffset(panel)
	if offset != before+8 {
		t.Fatal(offset)
	}
	if !strings.HasPrefix(strings.TrimLeft(ansi.Strip(m.branchRowText(r, panel)), " "), "▾") {
		t.Fatal(m.branchRowText(r, panel))
	}
	arrowX := m.treeRowX(r, panel)
	y := panel.y + 3 + m.selected - m.navStart(m.navigationRows(panel))
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: arrowX, Y: y})
	if !m.collapsed[r.id] {
		t.Fatal("visible arrow hit target did not follow horizontal viewport")
	}
	// Collapsing the root shrinks and clamps the old horizontal scroll.
	m.collapsed["root"] = true
	m.selected = 0
	m.reflow()
	if m.branchScroll != 0 {
		t.Fatal("stale horizontal scroll after collapse")
	}
}

func TestRenameDocumentAction(t *testing.T) {
	m := fixture()
	m.section = 1
	m.width = 120
	m.height = 36
	m.openNotes(true)
	for i, r := range m.noteRows() {
		if r.kind == "document-rename" {
			m.selected = i
			m.activateNote()
		}
	}
	if m.dialog == nil || m.dialog.kind != "rename" || m.dialog.args["node"] != m.currentID() {
		t.Fatal("rename must target opened document")
	}
}

func TestRenameDescendantsToggle(t *testing.T) {
	m := fixture()
	m.renameDocument()
	d := m.dialog
	if d.fields[1].input.Value() != "Off" {
		t.Fatal("rename should default to this document")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if d.fields[1].input.Value() != "On" {
		t.Fatal("space should toggle child names")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if d.fields[1].input.Value() != "Off" {
		t.Fatal("left should toggle child names")
	}
}

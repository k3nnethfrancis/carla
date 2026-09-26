package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

// Tree operations include hidden descendants, independent of rendered rows.
func (m *model) subtree(ids []string) map[string]bool {
	result := map[string]bool{}
	for _, id := range ids {
		result[id] = true
	}
	for changed := true; changed; {
		changed = false
		for _, n := range m.data.Nodes {
			if result[n.Parent] && !result[n.ID] {
				result[n.ID] = true
				changed = true
			}
		}
	}
	return result
}
func (m *model) selectionMark(id string) string {
	ids := m.subtree([]string{id})
	count := 0
	for key := range ids {
		if m.branchSelection[key] {
			count++
		}
	}
	if count == len(ids) {
		return "✓ "
	}
	if count > 0 {
		return "− "
	}
	return "  "
}

func (m *model) selectedBranches() []string {
	var ids []string
	for _, n := range m.data.Nodes {
		if m.branchSelection[n.ID] && (m.section != 2 || n.Kept) {
			ids = append(ids, n.ID)
		}
	}
	return ids
}
func (m *model) selectionVisible() bool {
	return !m.notesOpen && (m.section == 1 || m.section == 2) && m.editing == "" && len(m.selectedBranches()) > 0
}
func (m *model) openSelectionActions() tea.Cmd {
	actionID, label := "keep", "Keep selected"
	if m.section == 2 {
		actionID, label = "remove", "Remove from anthology"
	}
	m.dialog = &dialog{kind: "selection", title: fmt.Sprintf("%d selected branches", len(m.selectedBranches())), rows: []row{
		{id: "clear", label: "Clear selection", preview: "Uncheck all branches. Saved branches stay unchanged."},
		{id: actionID, label: label, preview: "Applies to the checked document versions."},
		{id: "delete", label: "Delete selected…", preview: "Review selected branches and all their descendants before deleting."},
	}}
	return nil
}
func (m *model) selectionAction(id string) tea.Cmd {
	if m.pending || m.data.Busy {
		m.status = "Wait for the current operation"
		return nil
	}
	switch id {
	case "clear":
		m.branchSelection = map[string]bool{}
		m.reflow()
	case "keep", "remove":
		ids := m.selectedBranches()
		if len(ids) == 0 && m.targetRow().kind == "node" {
			ids = []string{m.targetRow().id}
		}
		return m.send("node.keep", map[string]any{"nodes": ids, "kept": id == "keep"})
	case "delete":
		ids := m.selectedBranches()
		if len(ids) == 0 && m.targetRow().kind == "node" {
			ids = []string{m.targetRow().id}
		}
		if len(ids) == 0 {
			return nil
		}
		affected := m.subtree(ids)
		expected := []string{}
		rows := []row{{id: "cancel", label: "Cancel", preview: "Return without deleting anything."}}
		for _, n := range m.data.Nodes {
			if affected[n.ID] {
				expected = append(expected, n.ID)
				label := n.Title
				if label == "" {
					label = n.Kind + " · " + n.ID
				}
				if n.Kept {
					label = "★ " + label
				}
				reason := "Selected branch"
				if !m.branchSelection[n.ID] {
					reason = "Descendant — included to preserve a valid tree"
				}
				rows = append(rows, row{id: "info", label: label, preview: reason})
			}
		}
		rows = append(rows, row{id: "confirm", label: fmt.Sprintf("Delete all %d branches", len(expected)), preview: "Removes affected notes and policy decisions too. A recovery snapshot is saved in the workspace’s deleted folder; library sources stay intact."})
		m.dialog = &dialog{kind: "delete", title: fmt.Sprintf("Delete %d branches? (%d additional descendants)", len(expected), len(expected)-len(ids)), rows: rows, args: map[string]any{"nodes": ids, "expected": expected}}
	}
	return nil
}
func (m *model) selectionLabels() []string {
	label := "Keep selected"
	if m.section == 2 {
		label = "Remove from anthology"
	}
	return []string{"[ Clear selection ]", "[ " + label + " ]", "[ Delete… ]"}
}
func (m *model) selectionRects() []rect {
	if !m.selectionVisible() {
		return nil
	}
	x := 1
	var rects []rect
	for _, label := range m.selectionLabels() {
		w := ansi.StringWidth(label)
		rects = append(rects, rect{x, m.layout().actionY - 1, w, 1})
		x += w + 1
	}
	return rects
}
func (m *model) selectionBar() string {
	return " " + strings.Join(m.selectionLabels(), " ")
}

// Selection is a new working set on each Library visit, not saved provenance.
// Defer its reset if another request is in flight; saved branch roots are intact.
func (m *model) switchSection(section int) tea.Cmd {
	if section == m.section && m.notesOpen {
		return m.backFromNotes()
	}
	m.rememberPage()
	m.notesOpen = false
	m.commandDocument = ""
	m.section = section
	m.selected = 0
	m.focus = 0
	m.filter = ""
	p := m.pages[section]
	if p != nil {
		copy := *p
		p = &copy
		known := false
		for _, n := range m.data.Nodes {
			if n.ID == p.nodeID {
				known = true
				break
			}
		}
		if !known {
			p.notes = false
		}
		m.selected, m.focus, m.filter, m.notesOpen, m.noteScroll = p.index, p.focus, p.filter, p.notes, p.noteScroll
		rows := m.rows()
		for i, r := range rows {
			if r.id == p.rowID {
				m.selected = i
				break
			}
		}
		m.selected = max(0, min(m.selected, len(rows)-1))
		if !p.notes && len(rows) > 0 && rows[m.selected].kind == "node" && rows[m.selected].id != p.nodeID {
			p.nodeID = rows[m.selected].id
			p.rowID = p.nodeID
			p.cursor = 0
			p.scroll = 0
		}
		m.restorePage = p
	}
	m.search.SetValue(m.filter)
	if section == 0 {
		m.resetSeeds = true
		m.data.Selected = nil
	}
	m.reflow()
	if reset := m.flushSeedReset(); reset != nil {
		return reset
	}
	if p != nil && p.notes && p.nodeID != "" && p.nodeID != m.currentID() {
		return m.send("node.open", map[string]any{"node": p.nodeID})
	}
	cmd := m.previewTarget()
	if cmd == nil {
		m.applyPagePosition()
	}
	return cmd
}

func (m *model) flushSeedReset() tea.Cmd {
	if !m.resetSeeds || m.pending || m.disconnected {
		return nil
	}
	m.resetSeeds = false
	return m.send("seed.clear", nil)
}

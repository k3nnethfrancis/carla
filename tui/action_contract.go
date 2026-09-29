package main

import (
	tea "charm.land/bubbletea/v2"
	"strings"
)

// Public command vocabulary is intentionally small; controls retain searchable
// legacy names and keyboard bindings without filling the default palette.
func (m *model) paletteAvailable(id, input string) bool {
	if m.inNotesContext() {
		switch id {
		case "remove", "branch", "eval", "snapshot", "keep", "delete", "continue", "loom":
			return false
		}
	}
	primary := map[string]bool{"add": true, "remove": true, "branch": true, "continue": true, "loom": true, "eval": true, "snapshot": true, "configure": true, "policy": true, "cancel": true, "help": true, "library": true, "branches": true, "kept": true, "simulator": true, "evaluations": true}
	if !primary[id] {
		return strings.TrimSpace(input) != "/"
	}
	switch id {
	case "add":
		return m.section != 3
	case "continue", "loom":
		return m.section != 4 && !m.inNotesContext()
	case "branch":
		return (m.section == 1 || m.section == 2) && len(m.actionNodeIDs()) > 0 || m.section == 3 && len(m.selectedConversations()) > 0
	case "remove":
		return m.section != 3
	}
	return true
}
func (m *model) addDescription() string {
	if m.inNotesContext() {
		return "Add a note to this document"
	}
	switch m.section {
	case 0:
		return "Import a source document"
	case 1:
		return "Add selected versions to Anthology"
	case 2:
		return "Choose versions to add to Anthology"
	case 4:
		return "Add items to this evaluation"
	}
	return "Add item"
}
func (m *model) addItem() tea.Cmd {
	if m.inNotesContext() {
		m.selected = 1
		return m.activateNote()
	}
	switch m.section {
	case 0:
		return m.openDialog("import")
	case 1:
		return m.selectionAction("keep")
	case 2:
		d := &dialog{kind: "anthology-add", title: "Add to Anthology"}
		for _, n := range m.data.Nodes {
			if !n.Kept {
				d.rows = append(d.rows, row{id: n.ID, label: documentLabel(n), preview: n.Preview})
			}
		}
		m.dialog = d
		return nil
	case 4:
		return m.openCollectionItems()
	}
	return nil
}
func (m *model) exportItems() tea.Cmd {
	scopes := []string{"library", "branches", "anthology", "simulator", "evaluate"}
	args := map[string]any{"scope": scopes[m.section]}
	switch m.section {
	case 0:
		if len(m.data.Selected) > 0 {
			args["refs"] = m.data.Selected
		}
	case 1, 2:
		if m.selectionVisible() {
			args["nodes"] = m.selectedBranches()
		} else if m.targetRow().kind == "document-set" {
			args["nodes"] = m.documentSetMembers(m.targetRow().id)
		}
	case 3:
		if targets := m.selectedConversations(); len(targets) > 0 {
			args["targets"] = targets
		}
	case 4:
		args["collection"] = m.evalCollection
		ids := []string{}
		for _, item := range m.collectionItems() {
			if m.evalSelection[item.ID] {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) > 0 {
			args["items"] = ids
		}
	}
	return m.send("export", args)
}
func (m *model) inNotesContext() bool {
	focus := m.focus
	if focus == 3 && m.commandOrigin != nil {
		focus = m.commandOrigin.focus
	}
	return m.notesOpen && focus == 0
}

// Checked versions win over preview. Preserve group identity only when every
// member is targeted; subsets are explicit leaf selections.
func (m *model) documentGroupArgs() map[string]any {
	ids := m.actionNodeIDs()
	set := m.targetRow()
	if set.kind == "document-set" {
		members := m.documentSetMembers(set.id)
		same := len(ids) == len(members)
		for i := range ids {
			if i >= len(members) || ids[i] != members[i] {
				same = false
			}
		}
		if same {
			return map[string]any{"set": set.id}
		}
	}
	return map[string]any{"nodes": ids}
}
func (m *model) sendDocumentGeneration(args map[string]any) tea.Cmd {
	if args["action"] == "continue" {
		m.pendingDocumentNodes = map[string]bool{}
		for _, n := range m.data.Nodes {
			m.pendingDocumentNodes[n.ID] = true
		}
		m.pendingDocumentSelection = map[string]bool{}
		for id, yes := range m.branchSelection {
			m.pendingDocumentSelection[id] = yes
		}
		m.pendingDocumentSet, _ = args["set"].(string)
	}
	return m.send("continue", args)
}
func (m *model) finishDocumentSelection() {
	if m.pendingDocumentSelection == nil {
		return
	}
	unchanged := len(m.pendingDocumentSelection) == len(m.branchSelection)
	for id, yes := range m.pendingDocumentSelection {
		unchanged = unchanged && m.branchSelection[id] == yes
	}
	if unchanged {
		for old, selected := range m.pendingDocumentSelection {
			if !selected {
				continue
			}
			result := ""
			for _, n := range m.data.Nodes {
				if !m.pendingDocumentNodes[n.ID] && (n.RevisionOf == old || n.Parent == old) {
					result = n.ID
				}
			}
			if result == "" {
				for _, n := range m.data.Nodes {
					if n.ID == old {
						result = m.data.DocumentHeads[n.DocumentID]
					}
				}
			}
			if result != "" {
				delete(m.branchSelection, old)
				m.branchSelection[result] = true
			}
		}
	}
	if m.pendingDocumentSet != "" && m.section == 1 {
		for _, s := range m.data.DocumentSets {
			if s.ID == m.pendingDocumentSet {
				head := m.data.DocumentSetHeads[s.SetID]
				for i, r := range m.rows() {
					if r.id == head {
						m.selected = i
					}
				}
				break
			}
		}
	}
	m.pendingDocumentSelection = nil
	m.pendingDocumentNodes = nil
	m.pendingDocumentSet = ""
}

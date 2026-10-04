package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"slices"
	"sort"
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
	primary := map[string]bool{"add": true, "remove": true, "branch": true, "continue": true, "loom": true, "eval": true, "snapshot": true, "configure": true, "policy": true, "behaviors": true, "cancel": true, "help": true, "library": true, "branches": true, "kept": true, "simulator": true, "evaluations": true}
	if !primary[id] {
		return strings.TrimSpace(input) != "/"
	}
	switch id {
	case "snapshot":
		return m.section != 4 || m.currentEvaluation() != nil
	case "add":
		return m.section != 3 && !(m.section == 4 && m.evalArea == "runs")
	case "continue", "loom":
		return (m.section == 1 || m.section == 2 || m.section == 3) && !m.inNotesContext()
	case "branch":
		return (m.section == 1 || m.section == 2) && len(m.actionNodeIDs()) > 0 || m.section == 3 && len(m.selectedConversations()) > 0
	case "remove":
		return m.section != 3 && !(m.section == 4 && m.currentEvaluation() == nil && m.targetRow().kind != "eval-collection")
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
		if m.evalArea == "policies" {
			return "Create a policy"
		}
		if m.currentEvaluation() == nil {
			return "Create a data collection"
		}
		return "Add documents and traces to this collection"
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
		if m.evalArea == "policies" {
			return m.newEvaluationPolicy(nil)
		}
		if m.currentEvaluation() == nil {
			m.evalArea = "data"
			return m.evaluationCollectionAction("eval-create", "")
		}
		return m.openCollectionItems()
	}
	return nil
}
func (m *model) exportItems() tea.Cmd {
	if m.section == 4 && m.currentEvaluation() == nil {
		m.status = "Open a data collection to export its items and results"
		return nil
	}
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
	if !m.selectionVisible() && m.targetRow().kind == "document-set" {
		return map[string]any{"scope": m.documentScope(m.targetRow().id)}
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	used := map[string]bool{}
	children := []actionScope{}
	// Explicit set markers survive moving the preview to another row.
	groupIDs := []string{}
	for key, yes := range m.branchSelection {
		if yes && strings.HasPrefix(key, "set:") {
			groupIDs = append(groupIDs, strings.TrimPrefix(key, "set:"))
		}
	}
	sort.Strings(groupIDs)
	for _, groupID := range groupIDs {
		members := m.documentSetMembers(groupID)
		complete := len(members) > 0
		for _, id := range members {
			complete = complete && selected[id] && !used[id]
		}
		if !complete {
			continue
		}
		for _, id := range members {
			used[id] = true
		}
		children = append(children, m.documentScope(groupID))
	}
	for _, id := range ids {
		if !used[id] {
			children = append(children, actionScope{Kind: "document", Node: id})
		}
	}
	if len(children) == 1 {
		return map[string]any{"scope": children[0]}
	}
	return map[string]any{"scope": actionScope{Kind: "set", Children: children}}

}
func (m *model) sendDocumentGeneration(args map[string]any) tea.Cmd {
	if m.section == 2 {
		args["from_anthology"] = true
	}
	if args["action"] == "continue" || args["action"] == "loom" && (args["count"] == nil || args["count"] == 1) {
		m.pendingDocumentNodes = map[string]bool{}
		for _, n := range m.data.Nodes {
			m.pendingDocumentNodes[n.ID] = true
		}
		m.pendingDocumentSelection = map[string]bool{}
		for id, yes := range m.branchSelection {
			m.pendingDocumentSelection[id] = yes
		}
		m.pendingDocumentGroups = map[string][]string{}
		for id, yes := range m.branchSelection {
			if yes && strings.HasPrefix(id, "set:") {
				key := strings.TrimPrefix(id, "set:")
				m.pendingDocumentGroups[key] = append([]string(nil), m.documentSetMembers(key)...)
			}
		}
		m.pendingDocumentSet = ""
		if row := m.targetRow(); row.kind == "document-set" {
			m.pendingDocumentSet = row.id
			m.pendingDocumentGroups[row.id] = append([]string(nil), m.documentSetMembers(row.id)...)
		}
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
			if strings.HasPrefix(old, "set:") {
				id := strings.TrimPrefix(old, "set:")
				if next := m.continuedDocumentGroup(id); next != "" && next != id {
					delete(m.branchSelection, old)
					m.branchSelection["set:"+next] = true
				}
				continue
			}
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
				for _, n := range m.data.Nodes {
					if n.ID == result {
						if head := m.data.DocumentHeads[n.DocumentID]; head != "" {
							result = head
						}
						break
					}
				}
				delete(m.branchSelection, old)
				m.branchSelection[result] = true
			}
		}
	}
	if m.pendingDocumentSet != "" && m.section == 1 {
		if next := m.continuedDocumentGroup(m.pendingDocumentSet); next != "" {
			for i, r := range m.rows() {
				if r.id == next {
					m.selected = i
					break
				}
			}
		}
	}
	m.pendingDocumentGroups = nil
	m.pendingDocumentSelection = nil
	m.pendingDocumentNodes = nil
	m.pendingDocumentSet = ""
}

// Find the result subtree corresponding to a frozen selection, then follow the
// result set's latest loop revision. Hover and historical labels are irrelevant.
func (m *model) continuedDocumentGroup(id string) string {
	members := m.pendingDocumentGroups[id]
	if len(members) == 0 {
		return ""
	}
	var leafIDs func(actionScope) []string
	leafIDs = func(scope actionScope) []string {
		if scope.Kind == "document" {
			return []string{scope.Node}
		}
		var ids []string
		for _, child := range scope.Children {
			ids = append(ids, leafIDs(child)...)
		}
		return ids
	}
	var match func(actionScope, string) (string, bool)
	match = func(scope actionScope, path string) (string, bool) {
		if scope.Kind == "set" && slices.Equal(leafIDs(scope), members) {
			return path, true
		}
		for i, child := range scope.Children {
			suffix := fmt.Sprintf("/%d", i)
			if path == "" {
				suffix = fmt.Sprintf("/scope/%d", i)
			}
			if found, ok := match(child, path+suffix); ok {
				return found, true
			}
		}
		return "", false
	}
	result := ""
	for _, group := range m.data.DocumentSets {
		if group.SourceScope == nil || len(group.Members) == 0 {
			continue
		}
		fresh := true
		for _, member := range group.Members {
			fresh = fresh && !m.pendingDocumentNodes[member]
		}
		if !fresh {
			continue
		}
		if path, ok := match(*group.SourceScope, ""); ok {
			head := m.data.DocumentSetHeads[group.SetID]
			if head == "" {
				head = group.ID
			}
			result = head + path
		}
	}
	return result
}

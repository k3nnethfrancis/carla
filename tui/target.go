package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"
)

func (m *model) targetRow() row {
	if m.notesOpen {
		return row{id: m.currentID(), kind: "node"}
	}
	rows := m.rows()
	if len(rows) == 0 {
		return row{}
	}
	return rows[min(m.selected, len(rows)-1)]
}
func (m *model) targetRefs() []string {
	r := m.targetRow()
	if r.kind == "passage" {
		return []string{r.id}
	}
	var refs []string
	if r.kind == "source" {
		for _, s := range m.sources {
			if s.Key == r.id {
				for _, p := range s.Passages {
					refs = append(refs, s.Key+":"+p.ID)
				}
			}
		}
	}
	return refs
}
func (m *model) targetKept() bool {
	id := m.targetRow().id
	for _, n := range m.data.Nodes {
		if n.ID == id {
			return n.Kept
		}
	}
	return false
}
func (m *model) contextualActions() []action {
	r := m.targetRow()
	var actions []action
	if m.section == 0 && len(m.targetRefs()) > 0 {
		refs := m.targetRefs()
		count := 0
		for _, ref := range refs {
			for _, s := range m.data.Selected {
				if s == ref {
					count++
				}
			}
		}
		if count < len(refs) {
			actions = append(actions, action{id: "add", label: "Import a source document"})
		}
		if count > 0 {
			actions = append(actions, action{id: "remove", label: "Remove from seeds"})
		}

	} else if (m.section == 1 || m.section == 2) && (r.kind == "node" || m.selectionVisible()) {
		if m.section == 1 {
			label := fmt.Sprintf("Keep %d in anthology", m.collectionCount())
			if m.targetKept() && !m.selectionVisible() {
				label = "Already kept"
			}
			actions = append(actions, action{id: "keep", label: label})
		}
		if m.section == 2 {
			actions = append(actions, action{id: "remove", label: fmt.Sprintf("Remove %d from anthology", m.collectionCount())})
		}
		if r.kind == "node" {
			actions = append(actions, action{id: "continue", label: "Continue from this branch"})
		}
	}
	return actions
}
func (m *model) targetLabel() string {
	if m.section == 3 && m.editing == "" {
		if m.simSelection != nil {
			return m.simulationActionLabel() + " · /clear for fresh"
		}
		return "Loom · new conversation · SPACE selects a continuation target"
	}
	if m.notesOpen && m.editing == "" {
		return "Document · " + m.nodeTitle()
	}
	if m.editing != "" {
		return "Editing · " + m.nodeTitle()
	}
	r := m.targetRow()
	if r.id == "" {
		return "No item selected"
	}
	return "Focused · " + strings.TrimSpace(strings.TrimLeft(r.label, "▾▸★ "))
}

// Opening the highlighted branch keeps the preview and command target aligned.
// If a request is pending, the next state event catches up to the latest highlight.
func (m *model) previewTarget() tea.Cmd {
	// Coalesce fast navigation: after the pending reply, preview the latest row.
	if m.pending && m.focus == 0 && !m.notesOpen && (m.section == 1 || m.section == 2) {
		m.previewSelectionPending = true
	}
	if m.pending || m.editing != "" || m.dialog != nil {
		return nil
	}
	r := m.targetRow()
	if m.section == 3 && m.focus == 0 {
		args := map[string]any{"preview": true}
		switch r.kind {
		case "conversation":
			target, ok := gridConversation(r.id)
			if !ok {
				return nil
			}
			if m.simulation != nil && m.simulation.ID == target.Run && m.conversationOpen && m.gridSelection == target.Conversation {
				return nil
			}
			args["run"], args["conversation"] = target.Run, target.Conversation
		case "simulation":
			if m.simulation != nil && m.simulation.ID == r.id && !m.conversationOpen && m.gridGroup == "" {
				return nil
			}
			args["run"] = r.id
		case "simulation-group":
			if m.gridGroup == r.id && m.simulation != nil {
				return nil
			}
			scope := m.simulationGroupScope(r.id)
			if scope == nil {
				return nil
			}
			args["scope"] = *scope
		default:
			m.simulation = nil
			m.loomGrid = false
			m.conversationOpen = false
			m.gridGroup = ""
			m.reflow()
			return nil
		}
		return m.send("simulator.open", args)
	}
	if m.section == 4 && r.kind == "eval-run" && m.evaluationRunStale(r.id) {
		return m.send("evaluation.run.open", map[string]any{"id": r.id})
	}
	if m.section == 4 && r.kind == "evaluation" && (m.evaluation == nil || m.evaluation.ID != r.id) {
		return m.send("evaluation.item.open", map[string]any{"collection": m.evalCollection, "id": r.id})
	}
	if (m.section == 1 || m.section == 2) && r.kind == "node" && r.id != m.currentID() {
		m.loomGrid = false
		return m.send("node.open", map[string]any{"node": r.id})
	}
	return nil
}
func (m *model) contextualAction(id string) tea.Cmd {
	allowed := false
	for _, a := range m.contextualActions() {
		if a.id == id {
			allowed = true
		}
	}
	if !allowed {
		m.status = "Choose an item where /" + id + " is available"
		return nil
	}
	switch id {
	case "add":
		return m.addItem()
	case "remove":
		if m.section == 0 {
			return m.send("seed.remove", map[string]any{"refs": m.targetRefs()})
		}
		return m.selectionAction("remove")
	case "keep":
		if m.selectionVisible() {
			return m.selectionAction("keep")
		}
		return m.send("node.keep", map[string]any{"node": m.targetRow().id, "kept": true})
	case "continue":
		return m.loom(generationOptions{Action: "continue"})
	}
	return nil
}

func documentAction(id string) bool {
	switch id {
	case "branch", "grow", "edit", "inspect", "review", "rename":
		return true
	}
	return false
}

func (m *model) sourceSelectedCount(s source) int {
	selected := map[string]bool{}
	for _, ref := range m.data.Selected {
		selected[ref] = true
	}
	count := 0
	for _, p := range s.Passages {
		if selected[s.Key+":"+p.ID] {
			count++
		}
	}
	return count
}
func (m *model) seedSummary() string {
	var names []string
	for _, s := range m.sources {
		if count := m.sourceSelectedCount(s); count > 0 {
			names = append(names, fmt.Sprintf("%s (%d)", s.Title, count))
		}
	}
	if len(names) == 0 {
		return "No seed passages selected"
	}
	return m.keyLabel("nav.enter") + " opens together: " + strings.Join(names, " + ")
}

func (m *model) collectionCount() int {
	if m.selectionVisible() {
		return len(m.selectedBranches())
	}
	return 1
}

// The displayed action and the mutation must use the same visible selection.
// Notes hides batch selection, so only its open document is an action target.
func (m *model) actionNodeIDs() []string {
	if m.selectionVisible() {
		return m.selectedBranches()
	}
	if r := m.targetRow(); r.kind == "document-set" {
		return m.documentSetMembers(r.id)
	} else if r.kind == "node" {
		return []string{r.id}
	}
	return nil
}

func (m *model) selectDocument(id string) {
	// Expand the displayed ancestry, including operation/set wrappers that are
	// not document parents in storage. Leave unrelated collapsed groups alone.
	collapsed := m.collapsed
	m.collapsed = map[string]bool{}
	all := m.branchRows()
	m.collapsed = collapsed
	for i, r := range all {
		if r.id != id {
			continue
		}
		depth := r.depth
		for j := i - 1; j >= 0; j-- {
			if all[j].depth < depth {
				delete(m.collapsed, all[j].id)
				depth = all[j].depth
			}
		}
		break
	}
	parents := map[string]string{}
	for _, n := range m.data.Nodes {
		parents[n.ID] = n.Parent
	}
	for parent := parents[id]; parent != ""; parent = parents[parent] {
		delete(m.collapsed, parent)
	}
	for i, r := range m.rows() {
		if r.kind == "node" && r.id == id {
			m.selected = i
			break
		}
	}
}

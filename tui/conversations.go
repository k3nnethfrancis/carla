package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"strings"
)

type conversationParent struct {
	Run          string `json:"run"`
	Conversation int    `json:"conversation"`
}

// A run groups sibling alternatives; a one-conversation run is a leaf. Forks
// remain nested beneath their originating conversation, including on reload.
func (m *model) simulationRows() []row {
	rows := []row{{id: "config", kind: "sim-config", label: "Configure"}, {id: "run", kind: "sim-run", label: "▶ New conversation · /loom"}}
	runs := m.simulationSummaries()
	// Number persisted Loom identities, not individual members or current rows.
	// Collapsing a subtree therefore never renumbers the rest of the workspace.
	loomNumbers := map[string]int{}
	for _, run := range runs {
		key := run.ID
		if run.AlternativeGroup != "" {
			key = run.AlternativeGroup
		}
		if loomNumbers[key] == 0 {
			loomNumbers[key] = len(loomNumbers) + 1
		}
	}
	loomNumber := func(run runSummary) int {
		key := run.ID
		if run.AlternativeGroup != "" {
			key = run.AlternativeGroup
		}
		return loomNumbers[key]
	}
	seen := map[string]bool{}
	groups := map[string]bool{}
	var addRun func(runSummary, int)
	var addLeafRun func(runSummary, int, bool)
	var addScope func(actionScope, int, bool)
	var addChildren func(string, int, int)
	var addScopeChildren func(string, int)
	addScopeChildren = func(id string, depth int) {
		for _, r := range runs {
			if r.SourceScope != nil && r.SourceScope.ID == id {
				addRun(r, depth)
			}
		}
	}
	addChildren = func(run string, index, depth int) {
		for _, r := range runs {
			if r.SourceScope != nil && r.SourceScope.Kind == "set" && r.SourceScope.ID != "" && r.SourceScope.ID != run {
				continue
			}
			if r.Parent != nil && r.Parent.Run == run && r.Parent.Conversation == index {
				addRun(r, depth)
			}
		}
	}
	addScope = func(scope actionScope, depth int, suppressWrapper bool) {
		leaves := simulationScopeLeaves(scope)
		if len(leaves) == 0 {
			return
		}
		if scope.Kind == "conversation" {
			for _, run := range runs {
				if run.ID == scope.Run {
					seen[run.ID] = true
					for _, c := range run.Conversations {
						if c.Index == scope.Conversation {
							mark := "  "
							if m.conversationSelected(run.ID, c.Index) {
								mark = "✓ "
							}
							rows = append(rows, row{id: conversationKey(run.ID, c.Index), kind: "conversation", depth: depth, preview: run.ID, label: mark + conversationRowLabel(c, depth > 0)})
							addChildren(run.ID, c.Index, depth+1)
							return
						}
					}
				}
			}
			return
		}
		sameRun := true
		for _, child := range scope.Children {
			if child.Kind != "conversation" {
				sameRun = false
			}
		}
		for _, leaf := range leaves {
			if leaf.Run != leaves[0].Run {
				sameRun = false
			}
		}
		if sameRun {
			for _, run := range runs {
				if run.ID == leaves[0].Run && len(leaves) == len(run.Conversations) {
					if !suppressWrapper && scope.Label != "" {
						run.Label, run.ShortLabel, run.Title = scope.Label, scope.ShortLabel, scope.Title
					}
					addLeafRun(run, depth, !suppressWrapper)
					addScopeChildren(scope.ID, depth+1)
					return
				}
			}
		}
		if suppressWrapper {
			for _, child := range scope.Children {
				addScope(child, depth, false)
			}
			addScopeChildren(scope.ID, depth)
			return
		}
		mark := "  "
		if m.simSelection != nil && m.simSelection.Group == scope.ID {
			mark = "✓ "
		}
		arrow := "▾ "
		if m.collapsed[scope.ID] {
			arrow = "▸ "
		}
		rows = append(rows, row{id: scope.ID, kind: "simulation-group", depth: depth, label: mark + arrow + fmt.Sprintf("%s · %d conversations", simulationName(scope.Title, scope.ShortLabel, scope.Label, "set"), len(leaves))})
		if m.collapsed[scope.ID] {
			for _, leaf := range leaves {
				seen[leaf.Run] = true
			}
			return
		}
		for _, child := range scope.Children {
			addScope(child, depth+1, false)
		}
		addScopeChildren(scope.ID, depth+1)
	}
	addRun = func(r runSummary, depth int) {
		if r.AlternativeGroup == "" {
			addLeafRun(r, depth, true)
			return
		}
		group := r.AlternativeGroup
		if groups[group] {
			return
		}
		groups[group] = true
		mark := "  "
		if m.simSelection != nil && m.simSelection.Group == group {
			mark = "✓ "
		}
		arrow := "▾ "
		if m.collapsed[group] {
			arrow = "▸ "
		}
		rows = append(rows, row{id: group, kind: "simulation-group", depth: depth, label: mark + arrow + fmt.Sprintf("%s · %d branches", simulationName(r.OperationTitle, r.OperationShortLabel, r.OperationLabel, fmt.Sprintf("loom-%d", loomNumber(r))), r.AlternativeCount)})
		if m.collapsed[group] {
			for _, peer := range runs {
				if peer.AlternativeGroup == group {
					seen[peer.ID] = true
				}
			}
			return
		}
		for alternative := 0; alternative < r.AlternativeCount; alternative++ {
			key := fmt.Sprintf("%s/%d", group, alternative)
			mark = "  "
			if m.simSelection != nil && m.simSelection.Group == key {
				mark = "✓ "
			}
			arrow = "▾ "
			if m.collapsed[key] {
				arrow = "▸ "
			}
			branchName := fmt.Sprintf("branch-%d", alternative+1)
			for _, peer := range runs {
				if peer.AlternativeGroup == group && peer.AlternativeIndex == alternative {
					branchName = simulationName(peer.AlternativeTitle, peer.AlternativeShortLabel, peer.AlternativeLabel, branchName)
					break
				}
			}
			rows = append(rows, row{id: key, kind: "simulation-group", depth: depth + 1, label: mark + arrow + branchName})
			renderedScope := false
			for _, peer := range runs {
				if peer.AlternativeGroup == group && peer.AlternativeIndex == alternative {
					if m.collapsed[key] {
						seen[peer.ID] = true
					} else if peer.AlternativeScope != nil {
						if !renderedScope {
							addScope(*peer.AlternativeScope, depth+2, true)
							renderedScope = true
						}
					} else {
						addLeafRun(peer, depth+2, false)
					}
				}
			}
		}
		addScopeChildren(group, depth+1)
	}
	addLeafRun = func(r runSummary, depth int, showGroup bool) {
		if seen[r.ID] {
			return
		}
		seen[r.ID] = true
		cs := r.Conversations
		if len(cs) == 0 {
			for i := 0; i < r.Count; i++ {
				cs = append(cs, simulationConversation{Index: i, Status: r.Status})
			}
		}
		if len(cs) > 1 && showGroup {
			arrow := "▾ "
			if m.collapsed[r.ID] {
				arrow = "▸ "
			}
			mark := "  "
			if m.simSelection != nil && m.simSelection.Run == r.ID && m.simSelection.All {
				mark = "✓ "
			}
			rows = append(rows, row{id: r.ID, kind: "simulation", depth: depth, label: mark + arrow + fmt.Sprintf("%s · %d conversations · %s", simulationName(r.Title, r.ShortLabel, r.Label, fmt.Sprintf("loom-%d", loomNumber(r))), len(cs), r.Status), preview: r.ID})
			if m.collapsed[r.ID] {
				return
			}
			depth++
		}
		addChildren(r.ID, -1, depth)
		for _, c := range cs {
			key := conversationKey(r.ID, c.Index)
			label := conversationRowLabel(c, depth > 0)
			hasChildren := false
			for _, child := range runs {
				if child.Parent != nil && child.Parent.Run == r.ID && child.Parent.Conversation == c.Index {
					hasChildren = true
				}
			}
			if hasChildren {
				if m.collapsed[key] {
					label = "▸ " + label
				} else {
					label = "▾ " + label
				}
			}
			for _, t := range c.Turns {
				if c.Status == "running" && turnFlagged(t) {
					label = "! " + label
					break
				}
			}
			mark := "  "
			if m.conversationSelected(r.ID, c.Index) {
				mark = "✓ "
			}
			label = mark + label
			rows = append(rows, row{id: key, kind: "conversation", depth: depth, label: label, preview: r.ID})
			if !m.collapsed[key] {
				addChildren(r.ID, c.Index, depth+1)
			}
		}
	}
	for _, r := range runs {
		if r.Parent == nil {
			addRun(r, 0)
		}
	}
	for _, r := range runs {
		if !seen[r.ID] { // Missing/deleted ancestor still leaves its descendants reachable.
			parentExists := false
			for _, p := range runs {
				if r.Parent != nil && p.ID == r.Parent.Run {
					parentExists = true
				}
			}
			if !parentExists {
				addRun(r, 0)
			}
		}
	}
	return rows
}
func (m *model) conversationTarget() (map[string]any, bool) {
	if m.section != 3 {
		return nil, false
	}
	focus := m.focus
	if focus == 3 && m.commandOrigin != nil {
		focus = m.commandOrigin.focus
	}
	if focus == 1 && m.simulation != nil {
		if m.conversationOpen {
			return map[string]any{"run": m.simulation.ID, "conversation": m.gridSelection}, true
		}
		if m.gridVisible() {
			items := m.gridItems()
			if m.gridSelection < len(items) {
				if target, ok := gridConversation(items[m.gridSelection].ID); ok {
					return map[string]any{"run": target.Run, "conversation": target.Conversation}, true
				}
			}
		}
	}
	r := m.targetRow()
	if r.kind == "conversation" {
		parts := strings.SplitN(r.id, ":", 2)
		if len(parts) == 2 {
			index, err := strconv.Atoi(parts[1])
			if err == nil {
				return map[string]any{"run": parts[0], "conversation": index}, true
			}
		}
	}
	return nil, false
}
func (m *model) simulatorBack() bool {
	if m.section != 3 || m.editing != "" {
		return false
	}
	if m.focus == 1 {
		if m.conversationOpen {
			m.conversationOpen = false
			m.loomGrid = m.simulation != nil && (len(m.simulation.Conversations) > 1 || m.gridGroup != "")
			if m.gridGroup != "" {
				for i, item := range m.gridItems() {
					if item.ID == conversationKey(m.simulation.ID, m.gridSelection) {
						m.gridSelection = i
						break
					}
				}
			}
			if !m.loomGrid {
				m.focus = 0
			}
			m.gridPinned = true
		} else {
			m.focus = 0
		}
		m.reflow()
		return true
	}
	return false
}
func (m *model) conversationArrow(direction string) {
	r := m.targetRow()
	if r.kind != "simulation" && r.kind != "conversation" && r.kind != "simulation-group" {
		return
	}
	if direction == "right" {
		m.collapsed[r.id] = false
		return
	}
	if !m.collapsed[r.id] {
		m.collapsed[r.id] = true
		return
	}
	rows := m.rows()
	for i := m.selected - 1; i >= 0; i-- {
		if rows[i].depth < r.depth {
			m.selected = i
			return
		}
	}
}
func (m *model) editConversation(visitor bool) tea.Cmd {
	if !m.conversationOpen || m.simulation == nil {
		m.status = "Open a conversation first"
		return nil
	}
	targets := m.selectedConversations()
	if len(targets) != 1 || targets[0].Run != m.simulation.ID || targets[0].Conversation != m.gridSelection {
		m.status = "Open and select one conversation to edit"
		return nil
	}
	if m.data.Busy || m.pending {
		return nil
	}
	if visitor {
		turns := m.simulation.Conversations[m.gridSelection].Turns
		if len(turns) > 0 && turns[len(turns)-1].Role != "character" {
			m.status = "Edit the existing visitor message, or /loom to get a character reply"
			return nil
		}
		m.conversationEdit = -1
		return m.startConversationEdit("")
	}
	d := &dialog{kind: "conversation-edit", title: "Edit a turn · creates a fork through this message"}
	for i, t := range m.simulation.Conversations[m.gridSelection].Turns {
		d.rows = append(d.rows, row{id: strconv.Itoa(i), label: fmt.Sprintf("%d · %s", i+1, speakerName(t.Role)), preview: t.Text})
	}
	m.dialog = d
	return nil
}
func (m *model) startConversationEdit(text string) tea.Cmd {
	m.dialog = nil
	m.editing = "conversation"
	m.editor.SetValue(text)
	m.editor.CursorEnd()
	m.focus = 1
	m.reflow()
	return m.editor.Focus()
}
func (m *model) saveConversationEdit(text string) tea.Cmd {
	args := map[string]any{"run": m.simulation.ID, "conversation": m.gridSelection, "text": text}
	if m.conversationEdit < 0 {
		args["visitor"] = true
	} else {
		args["turn"] = m.conversationEdit
	}
	return m.submitEditor("simulator.fork", args)
}
func speakerName(role string) string {
	if role == "character" {
		return "Character"
	}
	return "Visitor"
}
func (m *model) conversationDocument(width int) string {
	if m.simulation == nil || !m.conversationOpen {
		if m.simulation == nil {
			return ansi.Wrap(safe(m.simulationText()), width, "")
		}
		return "Select a conversation to open it, or select a Loom to view its grid."
	}
	c := m.simulation.Conversations[m.gridSelection]
	var blocks []string
	for i, t := range c.Turns {
		style := m.humanStyle().Bold(false)
		if t.Role == "character" {
			style = m.aiStyle()
		}
		name := speakerName(t.Role)
		detail := t.Model.Name
		if t.Origin == "human" || t.Origin == "human_edit" {
			detail = "human written"
		}
		if detail != "" {
			name += " · " + detail
		}
		heading := style.Bold(true).Render(ansi.Wrap(safe(fmt.Sprintf("%02d  %s", i+1, name)), width, ""))
		body := style.Render(ansi.Wrap(safe(t.Text), width, ""))
		block := heading + "\n\n" + body
		if t.Monitor.Status != "" {
			info := strings.TrimPrefix(monitorSummary(t.Monitor), "Policy: ")
			block += "\n\n" + m.accent("#79628C", "#B8A0CB").Render(ansi.Wrap("// policy · "+safe(info), width, ""))
		}
		if len(t.Flags) > 0 {
			block += "\n" + dim.Render(ansi.Wrap("// generation · "+strings.Join(t.Flags, ", "), width, ""))
		}
		blocks = append(blocks, block)
	}
	if error := conversationFailure(m.simulation, c); error != "" {
		blocks = append(blocks, m.accent("#A84F39", "#DB937C").Render(ansi.Wrap(safe(error), width, "")))
	}
	return strings.Join(blocks, "\n\n\n")
}

func (m *model) selectConversationRow() {
	if m.simulation == nil {
		return
	}
	key := conversationKey(m.simulation.ID, m.gridSelection)
	for i, r := range m.rows() {
		if r.id == key {
			m.selected = i
			return
		}
	}
}

func (m *model) selectLoomRow() {
	if m.simulation == nil {
		return
	}
	// Reveal ancestors of a resumed Loom before selecting its new group.
	id := m.simulation.ID
	for steps := 0; steps <= len(m.data.SimulationRuns); steps++ {
		m.collapsed[id] = false
		var parent *conversationParent
		if id == m.simulation.ID {
			parent = m.simulation.Parent
		} else {
			for _, r := range m.data.SimulationRuns {
				if r.ID == id {
					parent = r.Parent
					break
				}
			}
		}
		if parent == nil {
			break
		}
		m.collapsed[conversationKey(parent.Run, parent.Conversation)] = false
		id = parent.Run
	}
	for i, r := range m.rows() {
		if r.id == m.simulation.ID {
			m.selected = i
			return
		}
	}
}

func turnFlagged(t simulationTurn) bool {
	if len(t.Monitor.Detections) > 0 {
		return true
	}
	for _, check := range t.MonitorChecks {
		if len(check.Detections) > 0 {
			return true
		}
	}
	return false
}

func conversationFlags(c simulationConversation) string {
	seen := map[string]bool{}
	var flags []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			flags = append(flags, name)
		}
	}
	for _, turn := range c.Turns {
		for _, check := range append([]monitorResult{turn.Monitor}, turn.MonitorChecks...) {
			for _, detection := range check.Detections {
				add(detection.Name)
			}
		}
	}
	if len(flags) == 0 {
		return ""
	}
	return "! " + strings.Join(flags, " · ")
}

// Keep policy evidence visible even while the transcript is scrolled. The
// sidebar's transient warning is distinct from the persistent header summary.
func (m *model) conversationHeading(width int) string {
	title := fmt.Sprintf("convo-%d", m.gridSelection+1)
	if m.simulation == nil || m.gridSelection >= len(m.simulation.Conversations) {
		return title
	}
	conversation := m.simulation.Conversations[m.gridSelection]
	title = conversationName(conversation, false)
	if status := safe(conversationStatus(conversation)); status != "" {
		title += " · " + status
	}
	title = ansi.Truncate(title, width, "…")
	flags := safe(conversationFlags(conversation))
	if flags == "" || width-ansi.StringWidth(title) < 3 {
		return title
	}
	flags = ansi.Truncate(flags, max(1, width-ansi.StringWidth(title)-2), "…")
	return title + strings.Repeat(" ", max(2, width-ansi.StringWidth(title)-ansi.StringWidth(flags))) + m.accent("#A84F39", "#DB937C").Render(flags)
}

// Merge the live run into persisted summaries without losing alternative-set metadata.
func (m *model) simulationSummaries() []runSummary {
	runs := append([]runSummary{}, m.data.SimulationRuns...)
	if m.simulation != nil {
		s := m.simulation
		found := false
		for i := range runs {
			if runs[i].ID == s.ID {
				runs[i].Conversations = s.Conversations
				runs[i].Status = s.Status
				copySimulationNames(&runs[i], s)
				found = true
			}
		}
		if !found {
			runs = append(runs, runSummary{Label: s.Label, Title: s.Title, ShortLabel: s.ShortLabel, OperationTitle: s.OperationTitle, AlternativeTitle: s.AlternativeTitle, OperationLabel: s.OperationLabel, OperationShortLabel: s.OperationShortLabel, AlternativeLabel: s.AlternativeLabel, AlternativeShortLabel: s.AlternativeShortLabel, ID: s.ID, Status: s.Status, Count: len(s.Conversations), Conversations: s.Conversations, Parent: s.Parent, AlternativeScope: s.AlternativeScope, SourceScope: s.SourceScope, AlternativeGroup: s.AlternativeGroup, AlternativeIndex: s.AlternativeIndex, AlternativeCount: s.AlternativeCount})
		}
	}
	return runs
}
func alternativeKey(r runSummary) string {
	return fmt.Sprintf("%s/%d", r.AlternativeGroup, r.AlternativeIndex)
}

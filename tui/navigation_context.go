package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"
	"time"
)

// The palette is a temporary layer, not another document editing mode.
type commandOrigin struct {
	focus            int
	outer, searching bool
	dialog           *dialog
}
type pagePosition struct {
	rowID, nodeID, filter                    string
	index, focus, scroll, cursor, noteScroll int
	notes                                    bool
}
type runSummary struct {
	Parent                     *conversationParent
	Conversations              []simulationConversation
	ID, Status, Created, Label string
	Count                      int
	PolicyStops                int `json:"policy_stops"`
}

func (m *model) dismissCommands() tea.Cmd {
	m.command.SetValue("")
	m.command.Blur()
	m.commandIndex = 0
	m.historyPosition = 0
	if origin := m.commandOrigin; origin != nil {
		m.focus, m.sectionFocus, m.searching, m.dialog = origin.focus, origin.outer, origin.searching, origin.dialog
		m.commandOrigin = nil
	} else if m.editing != "" {
		m.focus = 1
	} else {
		m.focus = 0
	}
	m.reflow()
	if m.editing != "" && m.focus == 1 {
		return m.editor.Focus()
	}
	if m.searching {
		return m.search.Focus()
	}
	return nil
}

func (m *model) rememberPage() {
	p := &pagePosition{index: m.selected, focus: m.focus, scroll: m.document.YOffset(), filter: m.filter, notes: m.notesOpen, nodeID: m.currentID(), noteScroll: m.noteScroll}
	if m.focus == 3 && m.commandOrigin != nil {
		p.focus = m.commandOrigin.focus
	}
	rows := m.rows()
	if len(rows) > 0 {
		p.rowID = rows[min(m.selected, len(rows)-1)].id
	}
	if m.cursorNode == m.currentID() {
		p.cursor = m.cursorOffset()
	}
	m.pages[m.section] = p
}
func (m *model) applyPagePosition() {
	p := m.restorePage
	if p == nil {
		return
	}
	if (m.section == 1 || m.section == 2) && p.nodeID != "" && p.nodeID != m.currentID() {
		return
	}
	m.restorePage = nil
	for i, r := range m.rows() {
		if r.id == p.rowID {
			m.selected = i
			break
		}
	}
	if p.nodeID == m.currentID() {
		moveTextCursor(&m.navigator, p.cursor)
	}
	m.reflow()
	m.document.SetYOffset(p.scroll)
}

// Configuration updates refresh labels from authoritative state, retaining the
// menu's row and parent. No optimistic changes to saved model settings.
func (m *model) refreshConfig() {
	d := m.dialog
	if d == nil {
		return
	}
	switch d.kind {
	case "eval-definitions":
		m.openEvaluators()
	case "eval-definition":
		m.openEvaluator(d.args["id"].(string))
	case "loom-policy-behaviors":
		m.openBehaviors()
	case "loom-policy":
		m.openLoomPolicy()
	case "loom-policy-timing":
		m.openMonitorTiming()
	case "loom-policy-dimension":
		m.openDimension(d.args["id"].(string))
	case "sim-openings":
		m.openOpeningConfig()
	case "sim-config":
		m.openSimulatorConfig()
	case "sim-speakers":
		m.speakerPicker()
	case "grow-config":
		if d.title == "Selection policy" {
			m.openSelectionConfig()
		} else {
			m.openGrowConfig()
		}
	case "sim-sampling":
		m.openSampling(d.args["group"].(string))
	default:
		return
	}
	m.dialog.parent = d.parent
	// Keep a filtered choice stable when its saved value refreshes the panel.
	if d.query != "" {
		m.dialog.query = d.query
		m.dialog.allRows = append([]row{}, m.dialog.rows...)
		m.dialog.rows = filterRows(m.dialog.rows, d.query)
	}
	m.dialog.index = min(d.index, max(0, len(m.dialog.rows)-1))
	if len(d.rows) > 0 {
		for i, r := range m.dialog.rows {
			if r.id == d.rows[d.index].id {
				m.dialog.index = i
				break
			}
		}
	}
}
func (m *model) saveDialog(d *dialog, command string, args map[string]any) tea.Cmd {
	m.dialog = d.parent
	if command == "loom-policy.update" {
		return m.updateBehavior(args)
	}
	return m.send(command, args)
}
func (m *model) speakerPicker() tea.Cmd {
	m.dialog = &dialog{kind: "sim-speakers", title: "Simulation models", rows: []row{
		{id: "character_alias", label: "Character · " + m.modelName(m.simString("character_alias"))},
		{id: "visitor_alias", label: "Visitor · " + m.modelName(m.simString("visitor_alias"))},
	}}
	return nil
}
func (m *model) modelName(alias string) string {
	for _, model := range m.data.Models {
		if model.Alias == alias {
			return model.Name
		}
	}
	return alias
}
func (m *model) openSampling(group string) tea.Cmd {
	d := &dialog{kind: "sim-sampling", title: strings.TrimSuffix(group, "_settings") + " sampling", args: map[string]any{"group": group}}
	values, _ := m.data.SimulatorConfig[group].(map[string]any)
	for _, key := range []string{"n_predict", "temperature", "top_p"} {
		d.rows = append(d.rows, row{id: key, label: samplingLabel(key) + " · " + samplingValue(key, values[key])})
	}
	m.dialog = d
	return nil
}
func (m *model) readOnlyAction(id string) bool {
	if m.section == 3 && (id == "select" || id == "clear") {
		return true
	}
	switch id {
	case "evaluations", "grid", "library", "branches", "kept", "simulator", "inspect", "notes", "active", "find", "help", "keys", "cancel", "restart", "quit", "exit":
		return true
	}
	return false
}
func (m *model) openActive() tea.Cmd {
	m.loomGrid = true
	m.gridPinned = false
	if m.activeSimulation != nil && m.data.Busy {
		m.switchSection(3)
		m.simulation = m.activeSimulation
		m.conversationOpen = len(m.simulation.Conversations) == 1
		m.gridSelection = 0
		m.focus = 1
		m.reflow()
		m.document.GotoBottom()
		return nil
	}
	if m.data.ActiveNode != "" {
		m.rememberPage()
		m.section, m.focus, m.filter, m.notesOpen = 1, 0, "", false
		m.restorePage = nil
		for _, n := range m.data.Nodes {
			if n.ID == m.data.ActiveNode {
				m.revealBranch(n.ID)
				break
			}
		}
		return m.send("node.open", map[string]any{"node": m.data.ActiveNode})
	}
	m.status = "No active generation"
	return nil
}
func (m *model) revealBranch(id string) {
	parents := map[string]string{}
	for _, n := range m.data.Nodes {
		parents[n.ID] = n.Parent
	}
	for p := parents[id]; p != ""; p = parents[p] {
		delete(m.collapsed, p)
	}
	for i, r := range m.rows() {
		if r.id == id {
			m.selected = i
			break
		}
	}
}
func documentLabel(n node) string {
	if n.Title != "" {
		return n.Title
	}
	for _, line := range strings.Split(n.Preview+"\n"+n.Text, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return n.Kind + " · " + n.ID[:min(6, len(n.ID))]
}
func runLabel(r runSummary) string {
	date := r.Created
	if t, err := time.Parse(time.RFC3339Nano, date); err == nil {
		date = t.Local().Format("Jan 2 15:04")
	}
	status := strings.ReplaceAll(r.Status, "_", " ")
	if r.Status == "needs_review" {
		status = "finished / early stops"
	}
	if r.PolicyStops > 0 {
		status += fmt.Sprintf(" / %d policy stops", r.PolicyStops)
	}
	return fmt.Sprintf("%s · %s · %d conv · %s", date, status, r.Count, r.Label)
}
func filterRows(rows []row, query string) []row {
	if strings.TrimSpace(query) == "" {
		return rows
	}
	var out []row
	for _, r := range rows {
		hay := strings.ToLower(r.label + " " + r.preview + " " + r.id)
		match := true
		for _, word := range strings.Fields(strings.ToLower(query)) {
			if !strings.Contains(hay, word) {
				match = false
				break
			}
		}
		if match {
			out = append(out, r)
		}
	}
	return out
}
func (m *model) filterDialog(msg tea.KeyPressMsg) bool {
	d := m.dialog
	if len(d.fields) > 0 || d.kind == "keys" || d.kind == "delete" || (strings.HasPrefix(d.kind, "setup-") && d.kind != "setup-files") {
		return false
	}
	if msg.Code != tea.KeyBackspace && (msg.Text == "" || msg.Mod != 0) {
		return false
	}
	if d.allRows == nil {
		d.allRows = append([]row{}, d.rows...)
	}
	if msg.Code == tea.KeyBackspace {
		r := []rune(d.query)
		if len(r) > 0 {
			d.query = string(r[:len(r)-1])
		}
	} else {
		d.query += msg.Text
	}
	d.rows = filterRows(d.allRows, d.query)
	d.index = 0
	return true
}

// Closing an auxiliary command such as Help restores a suspended picker/editor.
func (m *model) closeDialog() tea.Cmd {
	d := m.dialog
	if d != nil && d.kind == "loom-policy-dimension" && d.args["id"] == "draft" {
		m.behaviorDraft = nil
	}
	if d != nil && d.kind == "setup-busy" {
		m.dialog = d.parent
		m.pending = false
		return m.send("cancel", nil)
	}
	if d != nil && d.parent != nil {
		m.dialog = d.parent
		if d.kind == "loom-policy-timing" || d.kind == "loom-policy-dimension" {
			m.refreshConfig()
		}
		return nil
	}
	m.dialog = nil
	if origin := m.commandOrigin; origin != nil {
		m.focus = origin.focus
		if origin.dialog != nil && origin.dialog != d && (d == nil || origin.dialog.kind != d.kind) {
			m.dialog = origin.dialog
		}
		m.commandOrigin = nil
	}
	if m.editing != "" && m.dialog == nil {
		m.focus = 1
		return m.editor.Focus()
	}
	return nil
}

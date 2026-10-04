package main

import (
	tea "charm.land/bubbletea/v2"
	"strings"
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
	PolicyRun             string `json:"policy_run"`
	Loop                  int
	Label                 string `json:"label"`
	Title                 string `json:"title"`
	ShortLabel            string `json:"short_label"`
	OperationTitle        string `json:"operation_title"`
	AlternativeTitle      string `json:"alternative_title"`
	OperationLabel        string `json:"operation_label"`
	OperationShortLabel   string `json:"operation_short_label"`
	AlternativeLabel      string `json:"alternative_label"`
	AlternativeShortLabel string `json:"alternative_short_label"`

	AlternativeScope    *actionScope `json:"alternative_scope"`
	SourceScope         *actionScope `json:"source_scope"`
	AlternativeGroup    string       `json:"alternative_group"`
	AlternativeIndex    int          `json:"alternative_index"`
	AlternativeCount    int          `json:"alternative_count"`
	Parent              *conversationParent
	Conversations       []simulationConversation
	ID, Status, Created string
	Count               int
	PolicyStops         int `json:"policy_stops"`
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
	case "policy":
		m.openPolicy()
	case "behavior-library":
		m.openBehaviorLibrary()
	case "behavior-library-item":
		m.openLibraryBehavior(d.args["id"].(string))
	case "operational-policy-list":
		m.openOperationalPolicies(d.args["purpose"].(string))
	case "eval-policy-list":
		m.openEvaluationPolicies()
	case "eval-policy-config":
		m.openEvaluationPolicy(d.args["id"].(string))
	case "eval-policy-judge":
		m.openPolicyJudge(d.args["id"].(string), d.args["judge"].(int))
	case "eval-policy-behaviors":
		m.openPolicyBehaviors(d.args["id"].(string))
	case "eval-policy-behavior":
		m.openPolicyBehavior(d.args["id"].(string), d.args["behavior"].(int))
	case "eval-collection-config":
		m.openConfig()
	case "eval-definitions":
		m.openEvaluators()
	case "loom-policy-actions-list":
		m.openMonitorActions()
	case "loom-policy-actions":
		m.openMonitorAction(d.args["id"].(string))
	case "loom-policy-judge":
		m.openMonitorJudge()
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
		if group, ok := d.args["settings_group"].(string); ok {
			m.openSimulatorSettings(group)
		} else if d.title == "Prompts" {
			m.openSimulatorPrompts()
		} else if d.title == "Context limits" {
			m.openSimulatorContexts()
		} else {
			m.openConfig()
		}
	case "loom-config":
		m.openConfig()
	case "sim-speakers":
		m.speakerPicker()
	case "grow-config":
		if d.args["selection_behavior"] != nil {
			m.openSelectionBehavior()
			break
		}
		switch d.title {
		case "Selection judge":
			m.openSelectionJudge()
		case "Selection behaviors":
			m.openSelectionBehaviors()
		case "Selection criteria", "Selection behavior":
			m.openSelectionBehavior()
		case "Selection policy":
			m.openSelectionConfig()
		default:
			if strings.HasPrefix(d.title, "Selection policy") {
				m.openSelectionConfig()
			} else {
				m.openGrowConfig()
			}
		}
	case "sim-sampling":
		m.openSampling(d.args["group"].(string))
	default:
		return
	}
	m.dialog.parent = d.parent
	for _, key := range []string{"operational_purpose", "operational_id", "selection_behavior"} {
		if value, ok := d.args[key]; ok {
			if m.dialog.args == nil {
				m.dialog.args = map[string]any{}
			}
			m.dialog.args[key] = value
		}
	}
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
	// Keep validation-sensitive forms open until their own backend reply arrives.
	if command == "evaluation.collection.delete" || d.kind == "eval-run-config" || d.kind == "eval-collection-new" || d.kind == "eval-policy-new" || len(d.fields) > 0 && d.args["field"] == "monitor_local_model" {
		id, cmd := m.dispatch(command, args)
		if cmd != nil {
			if d.args == nil {
				d.args = map[string]any{}
			}
			delete(d.args, "error")
			m.dialogRequest, m.savingDialog = id, d
		}
		return cmd
	}
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
		if key == "n_predict" && group != "opening_settings" {
			continue // Speaker token limits are first-level configuration controls.
		}
		d.rows = append(d.rows, row{id: key, label: samplingLabel(key) + " · " + samplingValue(key, values[key]), preview: map[string]string{
			"n_predict":   "Maximum new tokens for a generated opening.",
			"temperature": "Higher values increase variation; lower values favor likely continuations.",
			"top_p":       "Sample from tokens covering this cumulative probability (0–1).",
		}[key]})
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
		m.gridGroup = m.activeSimulation.AlternativeGroup
		m.conversationOpen = len(m.simulation.Conversations) == 1 && m.gridGroup == ""
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
	if n.Label != "" {
		return n.Label
	}
	for _, line := range strings.Split(n.Preview+"\n"+n.Text, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return n.Kind + " · " + n.ID[:min(6, len(n.ID))]
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
	if len(d.fields) > 0 || d.kind == "judge-prompt-confirm" || d.kind == "keys" || d.kind == "delete" || (strings.HasPrefix(d.kind, "setup-") && d.kind != "setup-files") {
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
	if d != nil && d.kind == "inspection" && d.parent == nil {
		m.dialog = nil
		m.inspectionParent = nil
		m.showInspectionReport()
		return nil
	}
	if d != nil && d.kind == "judge-prompt-confirm" {
		return m.resumeJudgePromptDraft()
	}
	if d != nil && d.kind == "loom-policy-dimension" && d.args["id"] == "draft" {
		m.behaviorDraft = nil
	}
	if d != nil && d.kind == "setup-busy" {
		m.dialog = d.parent
		m.pending = false
		return m.send("cancel", nil)
	}
	if d != nil && d.parent != nil {
		parent := d.parent
		m.dialog = parent
		// Every supported configuration parent reads authoritative state again.
		// refreshConfig is a no-op for other dialogs and preserves row/filter.
		m.refreshConfig()
		if m.dialog != parent {
			*parent = *m.dialog
			m.dialog = parent
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

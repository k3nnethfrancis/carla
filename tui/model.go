package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type workspace struct{ Name, Path string }
type passage struct{ ID, Text, Label string }
type source struct {
	Key, Title string
	Passages   []passage
}
type origin struct {
	Start, End int
	Kind       string
}
type node struct {
	ChangeOffset                                                 int `json:"change_offset"`
	ID, Parent, Kind, Status, Title, Text, Preview, Model, Label string
	Kept                                                         bool
	Origins                                                      []origin
}
type localModel struct{ Name, Alias string }
type settings struct {
	Count       int     `json:"count"`
	Tokens      int     `json:"n_predict"`
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
	Rounds      int     `json:"rounds"`
}
type annotation struct {
	ID, Node, Quote, Verdict, Note string
	Start, End                     int
}
type policyRun struct{ ID, Status, Selected string }
type monitorResult struct {
	EndOfTurn     bool `json:"end_of_turn"`
	Phase         string
	Tokens        int
	Detections    []policyDetection
	Status, Error string
	Scores        map[string]float64
}
type simulationTurn struct {
	MonitorChecks              []monitorResult `json:"monitor_checks"`
	Monitor                    monitorResult
	Role, Text, Status, Origin string
	Model                      localModel
	Flags                      []string
}
type simulationConversation struct {
	Index  int
	Status string
	Turns  []simulationTurn
}
type simulationRun struct {
	OpenConversation  *int `json:"open_conversation"`
	Parent            *conversationParent
	ID, Status, Error string
	Opened            bool
	Forked            bool
	Preview           bool
	Conversations     []simulationConversation
}
type state struct {
	EvaluationSets   []evaluationCollection `json:"evaluation_sets"`
	ActiveEvaluation string                 `json:"active_evaluation"`
	MonitorKeySource string                 `json:"monitor_key_source"`
	Evaluators       []evaluator
	Evaluations      []evaluationSummary
	EvaluationPrompt string `json:"evaluation_prompt"`
	Workspace        workspace
	Workspaces       []workspace
	Selected         []string
	Nodes            []node
	Current          *node
	Models           []localModel
	ModelAlias       string `json:"model_alias"`
	ModelContext     int    `json:"model_context"`
	NativeContext    int    `json:"native_context"`
	Settings         settings
	PolicySpec       string `json:"policy_spec"`
	PolicyPrompt     string `json:"policy_prompt"`
	PolicyModel      string `json:"policy_model"`
	Annotations      []annotation
	SimulatorConfig  map[string]any    `json:"simulator_config"`
	SimulationRuns   []runSummary      `json:"simulation_runs"`
	GrowSettings     settings          `json:"grow_settings"`
	SelectorModels   []localModel      `json:"selector_models"`
	PolicyRuns       []policyRun       `json:"policy_runs"`
	Bindings         map[string]string `json:"bindings"`
	ActiveNode       string            `json:"active_node"`
	Busy             bool
}
type row struct {
	id, label, kind, preview string
	depth                    int
}
type field struct {
	label string
	input textinput.Model
}
type dialog struct {
	query       string
	allRows     []row
	parent      *dialog // Escape returns to the menu that opened this page.
	kind, title string
	adjusting   bool
	previous    string
	rows        []row
	index       int
	fields      []field
	field       int
	args        map[string]any
}

type loomTile struct{ ID, Title, Text, Status string }

type model struct {
	evalCollection         string
	evalCreating           bool
	behaviorDraft          *loomDimension
	behaviorEditID         string
	behaviorCreating       string
	evalViewedID           string
	evaluation             *evaluationRecord
	evaluationRaw          json.RawMessage
	evalSelection          map[string]bool
	evalFilter             string
	evalEditingID          string
	dialogRequest          string // Correlates local service form validation with its backend reply.
	savingDialog           *dialog
	editRequest            string // A draft stays owned by the editor until this request succeeds.
	setupReturn            *dialog
	conversationOpen       bool
	simSelection           *simulationSelection
	conversationEdit       int
	policyPulses           map[string]policyPulse
	policySeen             map[string]bool
	loomTiles              []loomTile
	loomGrid               bool
	gridSelection          int
	gridPinned             bool
	commandOrigin          *commandOrigin
	pages                  [5]*pagePosition
	restorePage            *pagePosition
	activeSimulation       *simulationRun
	backgroundStatus       string
	capacityStatus         string
	editReturn             *dialog
	simulation             *simulationRun
	notesOpen              bool
	notePending            bool
	notesAfterOpen         bool
	noteScroll             int
	commandDocument        string
	commandHistory         []string
	historyPosition        int
	historyDraft           string
	resetSeeds             bool
	branchSelection        map[string]bool
	navigator              textarea.Model
	cursorNode             string
	enterLoom              bool
	keySaving              bool
	keyDraft               map[string]string
	keyCapture             bool
	keyNotice              string
	restarting             bool
	sectionFocus           bool
	collapsed              map[string]bool
	dark                   bool
	client                 *client
	data                   state
	sources                []source
	width, height          int
	focus                  int // navigation, document, inspector, command
	restoringView          bool
	section                int // library, branches, anthology, simulator
	selected, commandIndex int
	command                textinput.Model
	expanded               map[string]bool
	filter                 string
	searching              bool
	search                 textinput.Model
	document, inspector    viewport.Model
	previewChangePending   bool
	inspection             string
	showInspector          bool
	editor                 textarea.Model
	editing                string
	editNode               string
	dialog                 *dialog
	status                 string
	pending, disconnected  bool
}

func newModel(c *client) *model {
	area := textarea.New()
	area.Prompt = ""
	area.ShowLineNumbers = false
	area.CharLimit = 0
	area.MaxHeight = 0
	area.MaxWidth = 0
	area.SetVirtualCursor(true)
	plain := textarea.StyleState{Placeholder: dim, Selection: selectedStyle}
	area.SetStyles(textarea.Styles{Focused: plain, Blurred: plain})
	input := newInput()
	input.Placeholder = "Find passage"
	input.Prompt = "/ "
	command := newInput()
	command.Prompt = ""
	command.Placeholder = "Type / for commands"
	command.Focus()
	navigator := textarea.New()
	navigator.Prompt = ""
	navigator.ShowLineNumbers = false
	navigator.CharLimit = 0
	navigator.MaxHeight = 0
	navigator.MaxWidth = 0
	navigator.Focus()
	return &model{evalSelection: map[string]bool{}, branchSelection: map[string]bool{}, navigator: navigator, dark: true, collapsed: map[string]bool{}, command: command, focus: 3, client: c, expanded: map[string]bool{}, document: viewport.New(), inspector: viewport.New(), editor: area, search: input, status: "Opening workspace…"}
}
func (m *model) Init() tea.Cmd { return tea.Batch(m.client.read(), tea.RequestBackgroundColor) }
func (m *model) send(command string, args map[string]any) tea.Cmd {
	_, cmd := m.dispatch(command, args)
	return cmd
}
func (m *model) dispatch(command string, args map[string]any) (string, tea.Cmd) {
	if m.disconnected {
		m.status = "Backend disconnected. Quit and reopen Carla."
		return "", nil
	}
	if m.pending {
		return "", nil
	}
	m.pending = true
	return m.client.request(command, args)
}
func (m *model) currentID() string {
	if m.data.Current != nil {
		return m.data.Current.ID
	}
	return ""
}
func (m *model) currentText() string {
	if m.data.Current != nil {
		return m.data.Current.Text
	}
	return ""
}
func (m *model) rows() []row {
	if m.notesOpen {
		return m.noteRows()
	}
	rows := []row{}
	switch m.section {
	case 0:
		for _, s := range m.sources {
			marker := "▸ "
			if m.expanded[s.Key] || m.filter != "" {
				marker = "▾ "
			}
			count := m.sourceSelectedCount(s)
			mark := "  "
			if count > 0 {
				mark = "− "
				if count == len(s.Passages) {
					mark = "✓ "
				}
			}
			label := marker + mark + s.Title
			if count > 0 {
				label += fmt.Sprintf(" · %d selected", count)
			}
			rows = append(rows, row{id: s.Key, label: label, kind: "source"})
			if !m.expanded[s.Key] && m.filter == "" {
				continue
			}
			for _, p := range s.Passages {
				if m.filter != "" && !strings.Contains(strings.ToLower(p.ID+" "+p.Text), strings.ToLower(m.filter)) {
					continue
				}
				ref := s.Key + ":" + p.ID
				mark := "  "
				for _, v := range m.data.Selected {
					if v == ref {
						mark = "✓ "
						break
					}
				}
				label := p.Label
				if label == "" {
					label = p.Text
				}
				rows = append(rows, row{id: ref, label: mark + p.ID + " " + strings.Join(strings.Fields(label), " "), kind: "passage", preview: p.Text, depth: 1})
			}
		}
		rows = append(rows, row{id: "import", label: "+ Add document", kind: "library-import"})
	case 1, 2:
		rows = m.branchRows()

	case 3:
		rows = m.simulationRows()
	case 4:
		rows = m.evaluationRows()
	}
	if m.section > 0 {
		rows = filterRows(rows, m.filter)
	}
	return rows
}
func (m *model) activate() tea.Cmd {
	if m.notesOpen {
		return m.activateNote()
	}
	if (m.section == 1 || m.section == 2) && len(m.selectedBranches()) > 0 {
		return m.openSelectionActions()
	}
	rows := m.rows()
	if len(rows) == 0 {
		return nil
	}
	m.selected = min(m.selected, len(rows)-1)
	r := rows[m.selected]
	switch r.kind {
	case "eval-collection", "eval-create", "eval-back", "eval-config", "eval-add", "eval-execute":
		return m.evaluationCollectionAction(r.kind, r.id)
	case "evaluation":
		m.focus = 1
		return m.previewTarget()
	case "eval-definitions":
		return m.openEvaluators()
	case "eval-filter":
		m.dialog = &dialog{kind: "eval-filter", title: "Show evaluations"}
		for _, filter := range []string{"all", "pass", "fail", "unfinished", "training"} {
			m.dialog.rows = append(m.dialog.rows, row{id: filter, label: filter})
		}
		return nil
	case "library-import":
		return m.perform("import")
	case "sim-config":
		return m.perform("sim-config")
	case "sim-run":
		m.simSelection = nil
		return m.perform("simulate")
	case "conversation":
		args, ok := m.conversationTarget()
		if ok {
			cmd := m.send("simulator.open", args)
			if cmd != nil {
				m.selectLoomConversation(args)
			}
			return cmd
		}
		return nil
	case "simulation":
		m.selectSimulation(r.id)
		return m.send("simulator.open", map[string]any{"run": r.id})

	case "source", "passage":
		if len(m.data.Selected) == 0 {
			m.status = "Select passages with " + m.keyLabel("select") + " first"
			return nil
		}
		cmd := m.send("seed.open", nil)
		if cmd != nil {
			m.enterLoom = true
		}
		return cmd
	case "node":
		if r.id == m.currentID() {
			return m.openDocumentWithNotes()
		}
		cmd := m.send("node.open", map[string]any{"node": r.id})
		if cmd != nil {
			m.enterLoom = true
			m.notesAfterOpen = true
		}
		return cmd

	}
	return nil
}
func (m *model) beginEdit(kind string) tea.Cmd {
	if kind == "document" {
		return m.editDocumentWithNotes()
	}
	if m.data.Busy || m.pending {
		return nil
	}
	text := m.currentText()
	if kind == "character_template" || kind == "visitor_template" || kind == "visitor_brief" || kind == "opening_prompt" {
		text = m.simString(kind)
	} else if kind == "monitor_spec" {
		text = m.dimension(m.behaviorEditID).Spec
	} else if kind == "policy_spec" {
		text = m.data.PolicySpec
	} else if kind == "policy_prompt" {
		text = m.data.PolicyPrompt
	} else if m.data.Current == nil {
		return nil
	}
	m.editing = kind
	m.editNode = m.currentID()
	m.editor.SetValue(text)
	m.editor.CursorEnd()
	m.focus = 1
	m.reflow()
	return m.editor.Focus()
}
func (m *model) saveEditor() tea.Cmd {
	if m.pending || m.data.Busy {
		return nil
	}
	kind, text := m.editing, m.editor.Value()
	if kind == "monitor_spec" {
		args := map[string]any{"id": m.behaviorEditID, "spec": text}
		if m.behaviorEditID == "draft" {
			m.behaviorDraft.Spec = text
			m.editing = ""
			m.editor.Blur()
			m.dialog, m.editReturn = m.editReturn, nil
			m.refreshConfig()
			return nil
		}
		return m.submitEditor("loom-policy.update", args)
	}
	if kind == "evaluation-new-spec" {
		if strings.TrimSpace(text) == "" {
			m.status = "Enter criteria before saving"
			return nil
		}
		d := m.editReturn
		args := map[string]any{"name": d.fields[0].input.Value(), "spec": text, "kind": d.args["kind"], "prompt": m.data.EvaluationPrompt, "threshold": 0.8, "model": m.simString("monitor_model")}
		if args["kind"] == "diffusion" {
			args["model"] = m.simString("monitor_local_model")
			args["endpoint"] = "auto"
		}
		if args["kind"] == "llm" {
			args["model"] = m.data.SelectorModels[0].Alias
		}
		return m.submitEditor("evaluation.configure", args)
	}
	if strings.HasPrefix(kind, "evaluation-") {
		if kind == "evaluation-note" {
			return m.submitEditor("evaluation.item.annotate", map[string]any{"collection": m.evalCollection, "ids": []string{m.evalEditingID}, "note": text})
		}
		args := m.evaluator(m.evalEditingID).args()
		args[strings.TrimPrefix(kind, "evaluation-")] = text
		return m.submitEditor("evaluation.configure", args)
	}
	if kind == "conversation" && strings.TrimSpace(text) == "" {
		m.status = "Write a message before saving"
		return nil
	}
	if kind == "conversation" {
		return m.saveConversationEdit(text)
	}
	if kind == "document" {
		return m.submitEditor("node.edit", map[string]any{"node": m.editNode, "text": text})
	}
	if kind == "character_template" || kind == "visitor_template" || kind == "visitor_brief" || kind == "opening_prompt" {
		return m.submitEditor("simulator.configure", map[string]any{kind: text})
	}
	return m.submitEditor("configure", map[string]any{kind: text})
}

// Do not discard or blur the draft on dispatch. Only its correlated state reply
// commits the UI transition; errors and disconnects leave text/cursor intact.
func (m *model) submitEditor(command string, args map[string]any) tea.Cmd {
	id, cmd := m.dispatch(command, args)
	if cmd != nil {
		m.editRequest = id
	}
	return cmd
}

// Python offsets are Unicode code points. Never send display cells or UTF-8 byte
// positions for forks/annotations, including wide or combining characters.
func textOffset(text string, line, column int) int {
	lines := strings.Split(text, "\n")
	offset := 0
	for i := 0; i < min(line, len(lines)); i++ {
		offset += len([]rune(lines[i])) + 1
	}
	if line < len(lines) {
		offset += min(column, len([]rune(lines[line])))
	}
	return offset
}
func (m *model) branch() tea.Cmd {
	if m.editing == "document" {
		return m.generateDraft(true)
	}
	m.focus = 1
	m.reflow()
	return m.generateAtCursor(true)
}
func (m *model) apply(e event) tea.Cmd {
	switch e.Type {
	case "evaluation":
		var record evaluationRecord
		if err := json.Unmarshal(e.Data, &record); err != nil {
			return func() tea.Msg { return failure{err} }
		}
		m.pending = false
		if m.evalViewedID != record.ID {
			m.document.GotoTop()
			m.evalViewedID = record.ID
		}
		m.evaluation = &record
		m.evaluationRaw = append(json.RawMessage(nil), e.Data...)
		m.reflow()
		return m.previewTarget()
	case "setup":
		return m.setupEvent(e.Data)
	case "policy.detection":
		return m.detectPolicy(e.Data)
	case "loom.start":
		var p struct{ Count int }
		json.Unmarshal(e.Data, &p)
		m.loomTiles = make([]loomTile, p.Count)
		for i := range m.loomTiles {
			m.loomTiles[i] = loomTile{Title: fmt.Sprintf("Branch %d", i+1), Status: "queued"}
		}
		m.loomGrid = p.Count > 1
		m.gridSelection, m.gridPinned = 0, false
	case "loom.branch":
		var p struct {
			Index                   int
			ID, Title, Text, Status string
		}
		json.Unmarshal(e.Data, &p)
		if p.Index >= 0 && p.Index < len(m.loomTiles) {
			m.loomTiles[p.Index] = loomTile{p.ID, p.Title, p.Text, p.Status}
		}
	case "loom.end":
		var p struct{ Status string }
		json.Unmarshal(e.Data, &p)
		for i := range m.loomTiles {
			if m.loomTiles[i].Status == "queued" {
				m.loomTiles[i].Status = p.Status
			}
		}

	case "simulation":
		var run simulationRun
		if err := json.Unmarshal(e.Data, &run); err != nil {
			return func() tea.Msg { return failure{err} }
		}
		newView := run.Opened || m.simulation == nil
		if newView {
			m.conversationOpen = run.OpenConversation != nil || len(run.Conversations) == 1
			m.gridSelection = 0
			if run.OpenConversation != nil {
				m.gridSelection = *run.OpenConversation
			}
			m.loomGrid = !m.conversationOpen && len(run.Conversations) > 1
			m.gridPinned = true
			m.focus = 1
		}
		if run.Opened && m.restoringView {
			m.restoringView = false
			m.focus, m.sectionFocus = 3, false
			m.command.Focus()
		}
		if run.Opened {
			if run.Forked && run.OpenConversation != nil {
				m.selectLoomConversation(map[string]any{"run": run.ID, "conversation": *run.OpenConversation})
			}
			m.pending = false
		}
		if !run.Opened {
			if newView {
				m.selectSimulation(run.ID)
			}
			m.activeSimulation = &run
		}
		if run.Opened || m.simulation == nil || m.simulation.ID == run.ID {
			m.simulation = &run
		}
		if newView && !m.conversationOpen {
			m.selectLoomRow()
		}
		if newView && m.conversationOpen {
			m.selectConversationRow()
			m.document.GotoTop()
		}
		m.reflow()
	case "simulation.token":
		var delta struct {
			Run, Text          string
			Conversation, Turn int
		}
		json.Unmarshal(e.Data, &delta)
		appendDelta := func(run *simulationRun) {
			if run == nil || run.ID != delta.Run || delta.Conversation < 0 || delta.Turn < 0 || delta.Conversation >= len(run.Conversations) || delta.Turn >= len(run.Conversations[delta.Conversation].Turns) {
				return
			}
			run.Conversations[delta.Conversation].Turns[delta.Turn].Text += delta.Text
		}
		bottom := m.document.AtBottom()
		appendDelta(m.simulation)
		if m.activeSimulation != m.simulation {
			appendDelta(m.activeSimulation)
		}
		m.reflow()
		if m.section == 3 && m.simulation != nil && m.simulation.ID == delta.Run && bottom {
			m.document.GotoBottom()
		}

	case "capacity":
		var p struct {
			Active, Waiting, Slots int
			WaitReason             string `json:"wait_reason"`
		}
		json.Unmarshal(e.Data, &p)
		m.capacityStatus = fmt.Sprintf("%d active · %d waiting · limit %d", p.Active, p.Waiting, p.Slots)
		if p.WaitReason != "" && p.Waiting > 0 {
			m.capacityStatus += " · " + p.WaitReason
		}
	case "simulation.progress":
		var p struct {
			Conversation, Total int
			Role, Stage         string
		}
		json.Unmarshal(e.Data, &p)
		m.backgroundStatus = fmt.Sprintf("Conversation %d/%d · %s · %s · /active · /stop", p.Conversation, p.Total, p.Role, p.Stage)
	case "library.imported":
		var imported struct{ Key, Title string }
		json.Unmarshal(e.Data, &imported)
		m.dialog = nil
		m.pending = false
		cmd := m.switchSection(0)
		m.filter = ""
		m.focus = 0
		m.expanded[imported.Key] = true
		for i, r := range m.rows() {
			if r.id == imported.Key {
				m.selected = i
				break
			}
		}
		m.status = "Added " + imported.Title + " to Library"
		m.reflow()
		return cmd
	case "library":
		if err := json.Unmarshal(e.Data, &m.sources); err != nil {
			return func() tea.Msg { return failure{err} }
		}
	case "state":
		m.evaluation = nil
		noteSaved := m.notePending
		oldID, oldWorkspace := m.currentID(), m.data.Workspace.Path
		if err := json.Unmarshal(e.Data, &m.data); err != nil {
			return func() tea.Msg { return failure{err} }
		}
		m.pending = false
		if m.dialogRequest != "" && e.ID == m.dialogRequest {
			if m.dialog == m.savingDialog {
				m.dialog = m.savingDialog.parent
			}
			m.dialogRequest, m.savingDialog = "", nil
		}
		if m.editRequest != "" {
			if e.ID == m.editRequest {
				if m.editing == "evaluation-new-spec" && m.editReturn != nil {
					m.editReturn = m.editReturn.parent
				}
				m.enterLoom = m.editing == "document"
				m.editRequest, m.editing = "", ""
				m.dialog, m.editReturn = m.editReturn, nil
				m.editor.Blur()
			} else {
				m.pending = true
			}
		}
		if !m.data.Busy {
			m.capacityStatus = ""
		}
		createdBehavior := ""
		if m.behaviorCreating != "" && e.ID == m.behaviorCreating {
			if items := m.dimensions(); len(items) > 0 {
				createdBehavior = items[len(items)-1].ID
			}
			m.behaviorCreating = ""
			for d := m.dialog; d != nil; d = d.parent {
				if d.kind == "loom-policy-dimension" && d.args["id"] == "draft" {
					m.dialog = d.parent
					break
				}
			}
			m.behaviorDraft = nil
		}
		m.refreshConfig()
		if m.evalCreating {
			m.evalCreating = false
			m.enterCollection(m.data.ActiveEvaluation)
		}
		if createdBehavior != "" && m.dialog != nil && m.dialog.kind == "loom-policy-behaviors" {
			for i, r := range m.dialog.rows {
				if r.id == createdBehavior {
					m.dialog.index = i
					break
				}
			}
		}
		if m.dialog != nil && m.dialog.kind == "keys" && m.keySaving && m.keyDraft != nil {
			if reflect.DeepEqual(m.keyDraft, m.data.Bindings) {
				m.dialog = nil
				m.keyDraft = nil
				m.keySaving = false
				m.status = "Keybindings saved"
			}
		}
		if oldWorkspace != m.data.Workspace.Path {
			m.behaviorDraft = nil
			m.behaviorCreating = ""
			m.evalCollection = ""
			m.evaluation = nil
			m.evalViewedID = ""
			m.evaluationRaw = nil
			m.evalSelection = map[string]bool{}
			m.evalFilter = ""
			m.loomTiles = nil
			m.loomGrid = false
			m.simulation = nil
			m.conversationOpen = false
			m.activeSimulation = nil
			m.simSelection = nil
			m.pages = [5]*pagePosition{}
			m.restorePage = nil
			m.commandOrigin = nil
			m.notesOpen, m.notesAfterOpen = false, false
			m.loadCommandHistory()
			m.editing = ""
			m.editNode = ""
			m.editor.Reset()
			m.cursorNode = ""
			m.commandDocument = ""
			m.collapsed = map[string]bool{}
			m.branchSelection = map[string]bool{}
			m.selected = 0
			m.section = 0
			m.filter = ""
			m.inspection = ""
			m.showInspector = false
			m.status = "Saved locally · " + m.data.Workspace.Path
			if len(m.data.Nodes) > 0 {
				m.section = 1
				// The saved UI location is restored after this snapshot is applied.
			}
		}
		if oldID != m.currentID() {
			m.commandDocument = ""
			m.inspection = ""
			m.showInspector = false
			// Reveal a newly generated or externally opened node inside its tree.
			parents := map[string]string{}
			for _, n := range m.data.Nodes {
				parents[n.ID] = n.Parent
			}
			for parent := parents[m.currentID()]; parent != ""; parent = parents[parent] {
				delete(m.collapsed, parent)
			}
			if m.section == 1 && m.data.Busy && m.focus == 1 {
				for i, row := range m.rows() {
					if row.id == m.currentID() {
						m.selected = i
						break
					}
				}
			}
			m.document.GotoBottom()
			if !m.data.Busy {
				m.document.GotoTop()
			}
		}
		if m.enterLoom {
			m.enterLoom = false
			m.section = 1
			m.focus = 1
			for i, r := range m.rows() {
				if r.id == m.currentID() {
					m.selected = i
					break
				}
			}
		}
		if m.notePending {
			m.notePending = false
			m.editing = ""
			m.editor.Blur()
			m.notesOpen = true
			m.selected = len(m.noteRows()) - 1
		}
		startDocumentEdit := m.notesAfterOpen
		m.notesAfterOpen = false
		valid := map[string]bool{}
		for _, n := range m.data.Nodes {
			valid[n.ID] = true
		}
		for id := range m.branchSelection {
			if !valid[id] {
				delete(m.branchSelection, id)
			}
		}

		m.selected = max(0, min(m.selected, len(m.rows())-1))
		if oldID != m.currentID() {
			m.previewChangePending = (m.section == 1 || m.section == 2) && !m.data.Busy && m.editing == ""
		}
		m.reflow()
		if noteSaved {
			for i := len(m.data.Annotations) - 1; i >= 0; i-- {
				if m.data.Annotations[i].Node == m.currentID() {
					m.revealNote(m.data.Annotations[i])
					break
				}
			}
		}
		m.applyPagePosition()
		if oldWorkspace != m.data.Workspace.Path {
			return m.restoreWorkspaceView()
		}
		if startDocumentEdit {
			return m.openDocumentWithNotes()
		}
	case "token":
		var t struct{ Node, Text string }
		json.Unmarshal(e.Data, &t)
		for i := range m.loomTiles {
			if m.loomTiles[i].ID == t.Node {
				m.loomTiles[i].Text += t.Text
			}
		}
		if m.data.Current != nil && m.data.Current.ID == t.Node {
			atBottom := m.document.AtBottom()
			start := len([]rune(m.data.Current.Text))
			m.data.Current.Text += t.Text
			spans := &m.data.Current.Origins
			if len(*spans) > 0 && (*spans)[len(*spans)-1].Kind == "ai" {
				(*spans)[len(*spans)-1].End += len([]rune(t.Text))
			} else {
				*spans = append(*spans, origin{start, start + len([]rune(t.Text)), "ai"})
			}
			m.reflow()
			if atBottom {
				m.document.GotoBottom()
			}
		}
	case "operation":
		var s struct {
			Stage, Command, Message string
			Index, Count, Round     int
		}
		json.Unmarshal(e.Data, &s)
		if s.Stage == "failed" && strings.HasPrefix(m.status, "Error:") {
			return nil
		}
		m.status = s.Stage
		if s.Message != "" {
			m.status = s.Message
		}
		if s.Count > 0 {
			m.status += fmt.Sprintf(" · branch %d/%d", s.Index, s.Count)
		}
		m.backgroundStatus = m.status + " · /active · /stop"
		if s.Round > 0 {
			m.status += fmt.Sprintf(" · round %d", s.Round)
		}
	case "inspection":
		m.pending = false
		var out strings.Builder
		var data any
		json.Unmarshal(e.Data, &data)
		pretty, _ := json.MarshalIndent(data, "", "  ")
		out.Write(pretty)
		m.inspection = out.String()
		m.showInspector = true
		m.focus = 2
		m.reflow()
		m.inspector.GotoTop()
	case "error":
		if m.editRequest != "" && e.ID != "" && e.ID != m.editRequest {
			return nil
		}
		m.editRequest = ""
		m.notePending = false
		m.enterLoom = false
		m.keySaving = false
		m.restoringView = false
		m.behaviorCreating = ""
		var err struct{ Message string }
		json.Unmarshal(e.Data, &err)
		m.pending = false
		m.status = "Error: " + err.Message
		if m.dialogRequest != "" && e.ID == m.dialogRequest {
			m.savingDialog.args["error"] = err.Message
			m.dialogRequest, m.savingDialog = "", nil
		}
		if m.dialog != nil && m.dialog.kind == "import" {
			m.dialog.args["error"] = err.Message
		}
	case "result":
		var r struct{ Path string }
		json.Unmarshal(e.Data, &r)
		m.status = "Saved: " + r.Path
	case "bye":
		return tea.Quit
	}
	return nil
}

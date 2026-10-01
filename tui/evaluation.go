package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Definitions and result snapshots belong to Python; these structs only render
// them and carry explicit user actions back across the protocol.
type evaluator struct {
	ID, Name, Kind, Spec, Prompt, Model, Endpoint string
	Revision                                      int
	Threshold                                     float64
}
type evaluationSummary struct {
	ID, Title, Kind, Status, Created, Evaluator, Error string
	Passed                                             *bool
	Training                                           bool
	Revision                                           int
}
type evaluationRecord struct {
	GenerationModels                           []string `json:"generation_models"`
	ID, Title, Kind, Text, Status, Note, Error string
	Passed                                     *bool
	Judgments                                  []evaluationRecord
	Evidence                                   []json.RawMessage
	Training                                   bool
	Definition                                 evaluator
	Result                                     struct {
		Passed           bool
		Reason, Evidence string
		Probability      *float64
	}
	Source json.RawMessage
	Trace  json.RawMessage
}

func (e evaluator) args() map[string]any {
	return map[string]any{"id": e.ID, "name": e.Name, "kind": e.Kind, "spec": e.Spec, "prompt": e.Prompt, "model": e.Model, "threshold": e.Threshold, "endpoint": e.Endpoint}
}
func (m *model) evaluator(id string) evaluator {
	for _, e := range m.data.Evaluators {
		if e.ID == id {
			return e
		}
	}
	return evaluator{}
}
func (m *model) openPolicy() tea.Cmd {
	if m.section == 4 {
		return m.openEvaluationPolicies()
	}
	m.dialog = &dialog{kind: "policy", title: "Policy", rows: []row{
		{id: "monitor", label: "Monitoring", preview: "Monitor document continuations and character replies; warn or explicitly stop. Shared across Branches and Simulator."},
		{id: "selection", label: "Selection", preview: selectionTriggerHelp},
		{id: "evaluators", label: "Evaluation policies", preview: "Configure reusable behaviors and models to assess saved documents and conversations."},
	}}
	return nil
}
func (m *model) openEvaluators() tea.Cmd {
	d := &dialog{kind: "eval-definitions", title: "Behaviors"}
	for _, e := range m.data.Evaluators {
		d.rows = append(d.rows, row{id: e.ID, label: e.Name + " · " + judgeLabel(e.Kind), preview: e.Spec})
	}
	d.rows = append(d.rows, row{id: "new", label: "+ New behavior", preview: "Describe what to assess and choose its model. Add behaviors to policies to run them together."})
	m.dialog = d
	return nil
}
func (m *model) openEvaluator(id string) tea.Cmd {
	e := m.evaluator(id)
	rows := []row{{id: "name", label: e.Name, preview: "Name this reusable behavior."}, {id: "kind", label: "Type · " + judgeLabel(e.Kind), preview: "Choose the model family used to assess this behavior."}, {id: "model", label: "Model · " + m.evaluatorModelName(e), preview: "Model that assesses this behavior. Classifiers return probabilities; LLMs return a structured judgment."}, {id: "spec", label: "Behavior spec", preview: e.Spec}}

	if e.Kind == "llm" {
		rows = append(rows, row{id: "prompt", label: "Judge prompt", preview: e.Prompt})
	} else {
		rows = append(rows, row{id: "threshold", label: fmt.Sprintf("Pass threshold · %.0f%%", e.Threshold*100), preview: "P(criteria met). Model estimate, not calibrated certainty."})
	}
	rows = append(rows, row{id: "delete", label: "Delete definition…", preview: "Historical results retain their frozen definition."})
	m.dialog = &dialog{kind: "eval-definition", title: fmt.Sprintf("%s · revision %d", e.Name, e.Revision), rows: rows, args: map[string]any{"id": id}}
	return nil
}
func (m *model) evaluationIDs() []string {
	var ids []string
	for _, e := range m.collectionItems() {
		if m.evalSelection[e.ID] {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 && m.targetRow().kind == "evaluation" {
		ids = append(ids, m.targetRow().id)
	}
	return ids
}
func (m *model) evaluationTargets() []map[string]any {
	var targets []map[string]any
	switch m.section {
	case 1, 2:
		for _, id := range m.actionNodeIDs() {
			targets = append(targets, map[string]any{"node": id})
		}
	case 3:
		for _, target := range m.selectedConversations() {
			targets = append(targets, map[string]any{"run": target.Run, "conversation": target.Conversation})
		}
	case 4:
		for _, id := range m.evaluationIDs() {
			targets = append(targets, map[string]any{"evaluation": id})
		}
	}
	return targets
}
func parseEvalOptions(input string) (string, bool, error) {
	words, err := generationWords(input)
	if err != nil {
		return "", false, err
	}
	name := ""
	auto := false
	for i := 1; i < len(words); i++ {
		key, value, inline := strings.Cut(words[i], "=")
		if key == "--train-on-pass" {
			if !inline {
				i++
				if i >= len(words) {
					return "", false, fmt.Errorf("--train-on-pass needs true or false")
				}
				value = words[i]
			}
			if value != "true" && value != "false" {
				return "", false, fmt.Errorf("--train-on-pass needs true or false")
			}
			auto = value == "true"
		} else if !strings.HasPrefix(key, "-") && name == "" {
			name = words[i]
		} else {
			return "", false, fmt.Errorf("use /eval [name] [--train-on-pass true|false]")
		}
	}
	return name, auto, nil
}
func (m *model) openEval(input string) tea.Cmd {
	name, auto, err := parseEvalOptions(input)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if name == "" {
		name = m.data.ActiveEvaluationPolicy
	}
	policy := m.findEvaluationPolicy(name)
	if policy == nil {
		m.status = "Choose a policy in Evaluate → Policies, or use /eval policy-name"
		return nil
	}
	args := map[string]any{"policy": policy.ID, "train_on_pass": auto}
	if m.section == 4 {
		if m.currentEvaluation() == nil {
			m.status = "Open Data and select a collection to evaluate"
			return nil
		}
		args["collection"] = m.evalCollection
		ids := m.evaluationIDs()
		if len(ids) == 0 {
			for _, item := range m.collectionItems() {
				if item.Status != "complete" || policy.ID != m.data.ActiveEvaluationPolicy {
					ids = append(ids, item.ID)
				}
			}
		}
		if len(ids) == 0 {
			m.status = "Select data or add items to this collection first"
			return nil
		}
		args["items"] = ids
	} else {
		targets := m.evaluationTargets()
		if len(targets) == 0 {
			m.status = "Select a document or conversation first"
			return nil
		}
		args["targets"] = targets
	}

	cmd := m.send("evaluation.collection.run", args)
	if cmd != nil {
		m.dialog = nil
		m.section = 4
		m.evalArea = "runs"
		m.enterCollection("")
	}
	return cmd
}
func evaluationStatus(e evaluationSummary) string {
	if e.Status == "failed" {
		return "ERROR"
	}
	if e.Status != "complete" {
		return e.Status
	}
	if e.Passed == nil {
		return "EVIDENCE"
	}
	if e.Passed != nil && *e.Passed {
		return "PASS"
	}
	return "FAIL"
}
func (m *model) evaluationRows() []row { return m.collectionRows() }
func (m *model) evaluationView(width int) (text string) {
	defer func() { text = ansi.Wrap(safe(text), width, "") }()
	if m.evalArea == "runs" {
		return m.evaluationRunView(width)
	}
	if m.evaluation == nil || m.evaluation.ID != m.targetRow().id {
		if c := m.currentEvaluation(); c != nil {
			return c.Name + "\n\nAdd data · saved documents, conversations or existing judgments.\nCollection settings · name and default destination.\nRun · assess selected or pending items with the active policy.\n\n/policy chooses the behaviors used for assessment.\nSPACE selects · ENTER opens · /keep marks for training"
		}
		return "Evaluate\n\nData · saved documents and conversation traces.\nPolicies · behaviors and models used to assess them.\nRuns · results, preserving the exact inputs and policy.\n\nActive policy · " + m.activePolicyName()
	}
	return m.collectionItemView(m.evaluation, width)
}

func (m *model) evaluationActions() tea.Cmd {
	if len(m.evaluationIDs()) == 0 {
		return nil
	}
	m.dialog = &dialog{kind: "eval-actions", title: fmt.Sprintf("%d selected", len(m.evaluationIDs())), rows: []row{
		{id: "keep", label: "Mark for training"}, {id: "untrain", label: "Unmark for training"}, {id: "remove", label: "Remove from this evaluation"}, {id: "eval", label: "Evaluate again"}, {id: "notes", label: "Edit note (highlighted item)"}, {id: "inspect", label: "Exact inputs and results (highlighted item)"},
	}}
	return nil
}
func (m *model) evalAction(id string) tea.Cmd {
	ids := m.evaluationIDs()
	switch id {
	case "keep", "untrain":
		return m.send("evaluation.item.annotate", map[string]any{"collection": m.evalCollection, "ids": ids, "training": id == "keep"})
	case "remove":
		return m.send("evaluation.collection.remove", map[string]any{"collection": m.evalCollection, "ids": ids})
	case "snapshot":
		return m.exportItems()
	case "notes":
		if m.evaluation == nil || m.evaluation.ID != m.targetRow().id {
			return m.previewTarget()
		}
		m.evalEditingID = m.evaluation.ID
		m.editing = "evaluation-note"
		m.editor.SetValue(m.evaluation.Note)
		m.focus = 1
		m.reflow()
		return m.editor.Focus()
	case "inspect":
		if m.evaluation == nil || m.evaluation.ID != m.targetRow().id {
			return m.previewTarget()
		}
		var pretty bytes.Buffer
		json.Indent(&pretty, m.evaluationRaw, "", "  ")
		m.inspection = pretty.String()
		m.showInspector = true
		m.focus = 2
		m.reflow()
		return nil
	case "eval":
		return m.openEval("/eval")
	}
	return nil
}

// All forms reuse Carla's existing parent/back stack and correlated saves.
func (m *model) submitEvaluation(d *dialog) tea.Cmd {
	if strings.HasPrefix(d.kind, "eval-policy") {
		return m.submitEvaluationPolicy(d)
	}
	if strings.HasPrefix(d.kind, "eval-collection") || d.kind == "eval-add-items" {
		return m.submitCollection(d)
	}
	if len(d.fields) > 0 {
		if d.kind == "eval-new" {
			if strings.TrimSpace(d.fields[0].input.Value()) == "" {
				m.status = "Enter a name"
				return nil
			}
			if d.args["kind"] == "llm" && len(m.data.SelectorModels) == 0 {
				m.status = "Configure a local policy model first"
				return nil
			}
			m.editing = "evaluation-new-spec"
			m.editReturn = d
			m.dialog = nil
			m.editor.SetValue("")
			m.focus = 1
			m.reflow()
			return m.editor.Focus()
		}
		e := m.evaluator(d.args["id"].(string))
		args := e.args()
		field := d.args["field"].(string)
		value := d.fields[0].input.Value()
		if strings.TrimSpace(value) == "" {
			m.status = "Enter a value"
			d.args["error"] = m.status
			return nil
		}
		args[field] = value
		return m.saveDialog(d, "evaluation.configure", args)
	}
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	parent := func(cmd tea.Cmd) tea.Cmd {
		if m.dialog != nil {
			m.dialog.parent = d
		}
		return cmd
	}
	switch d.kind {
	case "policy":
		switch r.id {
		case "monitor":
			return parent(m.openLoomPolicy())
		case "selection":
			return parent(m.openSelectionConfig())
		default:
			return parent(m.openEvaluationPolicies())
		}
	case "eval-definitions":
		if r.id == "new" {
			m.dialog = &dialog{kind: "eval-new-kind", title: "Judge", parent: d, rows: []row{{id: "llm", label: "Local LLM", preview: "Prompt, criteria, boolean pass and quoted evidence."}, {id: "diffusion", label: "DiffusionGemma (local)", preview: "Local OpenJev classifier. Behavior spec and probability threshold; no API key."}, {id: "jev", label: "Jev via OpenRouter", preview: "Behavior spec and probability threshold. Uses OPENROUTER_API_KEY."}}}
			return nil
		}
		return parent(m.openEvaluator(r.id))
	case "eval-new-kind":
		n := &dialog{kind: "eval-new", title: "New behavior", parent: d.parent, args: map[string]any{"kind": r.id}}
		n.add("Name", "")
		m.dialog = n
		return n.fields[0].input.Focus()
	case "eval-definition":
		e := m.evaluator(d.args["id"].(string))
		switch r.id {
		case "threshold":
			m.numberConfig("evaluation", "threshold", e.Threshold*100, e.ID)
			m.dialog.title = "Pass probability · ↑↓ 1% · ←→ 10%"
			m.dialog.parent = d
			return nil
		case "spec", "prompt":
			m.evalEditingID = e.ID
			m.editing = "evaluation-" + r.id
			m.editReturn = d
			m.dialog = nil
			text := e.Spec
			if r.id == "prompt" {
				text = e.Prompt
			}
			m.editor.SetValue(text)
			m.focus = 1
			m.reflow()
			return m.editor.Focus()
		case "kind":
			m.dialog = &dialog{kind: "eval-kind", title: "Judge", parent: d, args: d.args, rows: []row{{id: "llm", label: "Local LLM"}, {id: "diffusion", label: "DiffusionGemma (local)"}, {id: "jev", label: "Jev via OpenRouter"}}}
			return nil
		case "model":
			if e.Kind == "llm" {
				n := &dialog{kind: "eval-model", title: "Local judge", parent: d, args: d.args}
				for _, model := range m.data.SelectorModels {
					n.rows = append(n.rows, row{id: model.Alias, label: model.Name})
				}
				m.dialog = n
				return nil
			}
		case "delete":
			m.dialog = &dialog{kind: "eval-delete", title: "Delete definition? Results are retained", parent: d, args: d.args, rows: []row{{id: "cancel", label: "Cancel"}, {id: "delete", label: "Delete definition"}}}
			return nil
		}
		n := &dialog{kind: "eval-field", title: r.label, parent: d, args: map[string]any{"id": e.ID, "field": r.id}}
		value := e.Name
		if r.id == "model" {
			value = e.Model
		}
		if r.id == "threshold" {
			value = fmt.Sprint(e.Threshold * 100)
		}
		n.add(r.label, value)
		m.dialog = n
		return n.fields[0].input.Focus()
	case "eval-kind", "eval-model":
		e := m.evaluator(d.args["id"].(string))
		args := e.args()
		if d.kind == "eval-model" {
			args["model"] = r.id
		} else {
			args["kind"] = r.id
			if r.id == "diffusion" {
				args["model"] = m.simString("monitor_local_model")
				args["endpoint"] = "auto"
			} else if r.id == "jev" {
				args["model"] = m.simString("monitor_model")
			} else if len(m.data.SelectorModels) > 0 {
				args["model"] = m.data.SelectorModels[0].Alias
			}
		}
		return m.saveDialog(d, "evaluation.configure", args)
	case "eval-delete":
		if r.id == "cancel" {
			m.dialog = d.parent
			return nil
		}
		m.dialog = d.parent.parent
		return m.send("evaluation.definition.delete", d.args)
	case "eval-filter":
		m.evalFilter = r.id
		m.evalSelection = map[string]bool{}
		m.selected = 0
		m.dialog = nil
		m.reflow()
		return m.previewTarget()
	case "eval-actions":
		m.dialog = nil
		return m.evalAction(r.id)
	}
	return nil
}

func judgeLabel(kind string) string {
	if kind == "llm" {
		return "Local LLM"
	}
	return monitorLabel(kind)
}

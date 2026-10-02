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
	JudgeName                                     string `json:"judge_name"`
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
		Passed             bool
		Reason, Evidence   string
		Probability        *float64
		Observed           *bool
		Expected           string
		DesiredProbability *float64 `json:"desired_probability"`
	}
	Source json.RawMessage
	Trace  json.RawMessage
}

func (m *model) openPolicy() tea.Cmd {
	m.dialog = &dialog{kind: "policy", title: "Policy", rows: []row{
		{id: "monitor", label: "Monitoring", preview: "Configure shared monitoring rules. Simulator uses saved On/Off; document Looms require --monitoring on."},
		{id: "selection", label: "Selection (/loom --loops)", preview: selectionTriggerHelp},
		{id: "evaluators", label: "Evals", preview: "Configure reusable behaviors and models to assess saved documents and conversations."},
	}}
	return nil
}
func (m *model) openEvaluators() tea.Cmd { return m.openEvaluationPolicies() }
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
	if m.section == 4 {
		var train *bool
		words, _ := generationWords(input)
		for _, word := range words {
			if word == "--train-on-pass" || strings.HasPrefix(word, "--train-on-pass=") {
				train = &auto
			}
		}
		return m.newEvaluationRun(name, train)
	}
	if name == "" {
		name = m.data.ActiveEvaluationPolicy
	}
	policy := m.findEvaluationPolicy(name)
	if policy == nil {
		m.status = "Choose a policy in Evaluate → Policies, or use /eval policy-name"
		return nil
	}
	args := map[string]any{"policy": policy.ID}
	words, _ := generationWords(input)
	for _, word := range words {
		if word == "--train-on-pass" || strings.HasPrefix(word, "--train-on-pass=") {
			args["train_on_pass"] = auto
		}
	}
	targets := m.evaluationTargets()
	if len(targets) == 0 {
		m.status = "Select a document or conversation first"
		return nil
	}
	args["targets"] = targets

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
	defer func() { text = ansi.Wrap(text, width, "") }()
	if m.evalArea == "runs" {
		return m.evaluationRunView(width)
	}
	if m.evaluation == nil || m.evaluation.ID != m.targetRow().id {
		if c := m.currentEvaluation(); c != nil {
			return safe(c.Name) + "\n\nAdd · saved documents, conversations or existing judgments.\nSettings · rename or remove this dataset.\n/eval configures a new run for this dataset or selected items.\nSPACE selects · ENTER opens · /keep marks for training"
		}
		return "Evaluate\n\nData · saved documents and conversation traces.\nPolicies · judges and the behaviors they assess.\nRuns · recorded assessments with exact inputs and policy.\n+ New run · choose a dataset and policy before starting."
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
		if collection := m.removableEvaluationCollection(); collection != "" {
			return m.confirmRemoveCollection(collection)
		}
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
	if strings.HasPrefix(d.kind, "eval-run-") {
		return m.submitEvaluationRun(d)
	}
	if strings.HasPrefix(d.kind, "eval-policy") {
		return m.submitEvaluationPolicy(d)
	}
	if strings.HasPrefix(d.kind, "eval-collection") || d.kind == "eval-add-items" {
		return m.submitCollection(d)
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
			return parent(m.openOperationalPolicies("monitoring"))
		case "selection":
			return parent(m.openOperationalPolicies("selection"))
		default:
			return parent(m.openEvaluationPolicies())
		}
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

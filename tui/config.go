package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
)

// Configuration is a contextual view, not a separate settings copy per tab.
func (m *model) openConfig() tea.Cmd {
	defer func() {
		if m.dialog != nil {
			m.dialog.rows = append(m.dialog.rows, row{id: "workspaces", label: "Workspace"}, row{id: "keys", label: "Keybindings"})
		}
	}()
	if m.section == 4 {
		if m.evalArea == "policies" {
			return m.openEvaluationPolicies()
		}
		cmd := m.openCollectionConfig()
		if m.dialog == nil {
			m.dialog = &dialog{kind: "loom-config", title: "Configuration"}
		}
		return cmd
	}
	if m.section == 3 {
		return m.openSimulatorConfig()
	}
	m.dialog = &dialog{kind: "loom-config", title: "Document defaults", rows: []row{
		{id: "tokens", label: "Tokens · " + budgetLabel(m.data.Settings.Tokens), preview: "New tokens per continuation, per alternative and loop. --tokens overrides one run; Max uses remaining context."},
		{id: "models", label: "Generation model"},
		{id: "settings", label: "Advanced settings", preview: "Temperature, top-p and context capacity, with the same token ceiling. Bare /loom continues once; /loom N sets the alternatives for that run."},
	}}
	return nil
}
func (m *model) openSelectionConfig() tea.Cmd {
	label := "Status · Inactive"
	if m.selectionEnabled() {
		label = "Status · Active"
	}
	d := &dialog{kind: "grow-config", title: "Selection policy", rows: []row{
		{id: "status", label: label, preview: operationalStatusHelp + "\n" + selectionTriggerHelp},
		{id: "behaviors", label: fmt.Sprintf("Behaviors · %d", len(m.selectionBehaviors())), preview: "Define criteria used to qualify alternatives and choose what continues."},
		{id: "judge", label: "Judge · " + m.selectionModelName() + " (LLM)", preview: "Choose the model and prompt used to compare candidates against these behaviors."},
	}}
	defer orderOperationalRows(d)
	m.appendOperationalControls(d)
	m.dialog = d
	return nil
}

// Keep the trigger first so compact dialogs show it before any clipped detail.
const selectionTriggerHelp = "Assesses alternatives against your behaviors, then chooses a qualifying branch to continue.\nUse /loom 4 --loops 2: select from four alternatives, then branch from the winner for the next loop. One loop selects once.\nRequires 2+ alternatives and explicit --loops. If none qualify, stop; all generated paths stay saved.\nSimulator uses the saved switch. Branches requires --selection on; --selection off skips it for a run."

func (m *model) openSelectionJudge() tea.Cmd {
	m.dialog = &dialog{kind: "grow-config", title: "Selection judge", rows: []row{
		{id: "selector", label: "Model · " + m.selectionModelName() + " (LLM)", preview: "The local instruction-following model used to assess alternatives."},
		{id: "selection_call_mode", label: "Assessment call mode · " + selectionCallModeLabel(m.selectionString("selection_call_mode")), preview: "Separate makes one assessment request per behavior per candidate; Bundled assesses a candidate’s behaviors together. Branch selection follows in one additional call."},
		{id: "assessment-prompt", label: "Behavior assessment template", preview: "Assess behavior presence in each complete candidate. Every enabled behavior must meet its Pass when condition to qualify. Saving changes requires confirmation."},
		{id: "prompt", label: "Branch selection template", preview: "Compare qualifying candidates using the behaviors and assessment results; choose one to continue, or none. Saving changes requires confirmation."},
	}}
	return nil
}
func (m *model) openSelectionBehaviors() tea.Cmd {
	d := &dialog{kind: "grow-config", title: "Selection behaviors"}
	for _, b := range m.selectionBehaviors() {
		id, _ := b["id"].(string)
		name, _ := b["name"].(string)
		spec, _ := b["spec"].(string)
		d.rows = append(d.rows, row{id: id, label: name + behaviorEnabledSuffix(b), preview: "Criteria applied to complete candidates. " + spec})
	}
	d.rows = append(d.rows, row{id: "new-behavior", label: "+ New behavior", preview: "Add another named criterion to this policy."}, row{id: "library-behavior", label: "From library", preview: "Import Off. Review whole-trace wording and Pass when before enabling."})
	m.dialog = d
	return nil
}
func (m *model) openSelectionBehavior() tea.Cmd {
	id := m.operationalBehaviorID()
	if id == "" {
		id = "criteria"
	}
	name, spec, expected, enabled := "Selection criteria", "", "present", false
	var librarySpec evaluationBehavior
	for _, b := range m.selectionBehaviors() {
		if b["id"] == id {
			raw, _ := json.Marshal(b)
			_ = json.Unmarshal(raw, &librarySpec)
			name, _ = b["name"].(string)
			spec, _ = b["spec"].(string)
			expected, _ = b["expected"].(string)
			enabled, _ = b["enabled"].(bool)
		}
	}
	state := "Off"
	if enabled {
		state = "On"
	}
	m.dialog = &dialog{kind: "grow-config", title: name, args: map[string]any{"selection_behavior": id}, rows: []row{
		{id: "behavior-enabled", label: "Enabled · " + state, preview: "Include this criterion in the selection judge’s candidate assessment."},
		{id: "behavior-name", label: "Name · " + name, preview: "Name this criterion independently of the policy and judge."},
		{id: "spec", label: "Behavior spec", preview: "Describe behavior presence in the complete candidate. " + spec},
		{id: "behavior-expected", label: "Pass when · " + expectedLabel(expected), preview: expectedHelp},
		m.behaviorLibraryRow(librarySpec),
		{id: "behavior-remove", label: "Remove behavior…", preview: "Remove this criterion from the policy. Existing results retain their saved spec."},
	}}
	return nil
}

const expectedHelp = "Judges detect whether the behavior is present. Choose Present to require it, or Absent to reject it. This changes pass/fail, not the recorded detection."

func expectedLabel(value string) string {
	if value == "absent" {
		return "Absent"
	}
	return "Present"
}
func nextExpected(value string) string {
	if value == "absent" {
		return "present"
	}
	return "absent"
}
func selectionCallModeLabel(value string) string {
	if value == "bundled" {
		return "Bundled"
	}
	return "Separate"
}
func behaviorEnabledSuffix(b map[string]any) string {
	if b["enabled"] == false {
		return " · off"
	}
	return ""
}

func (m *model) submitSelectionCallMode(d *dialog) tea.Cmd {
	if len(d.rows) == 0 {
		return nil
	}
	id := d.rows[d.index].id
	if d.kind == "selection-call-mode" && id == "bundled" {
		m.dialog = &dialog{kind: "selection-call-bundled", title: "Use bundled calls?", parent: d.parent, rows: []row{{id: "cancel", label: "Keep Separate", preview: "One independent assessment per behavior."}, {id: "confirm", label: "Use Bundled", preview: "Fewer requests, but behaviors share one response and can affect each other’s scores. Compare results before relying on them."}}}
		return nil
	}
	if id == "cancel" {
		m.dialog = d.parent
		return nil
	}
	mode := "separate"
	if id == "confirm" {
		mode = "bundled"
	}
	return m.saveDialog(d, "configure", map[string]any{"selection_call_mode": mode})
}

// Quick changes and Enter's picker share the same warning and save path.
func (m *model) quickJudgeCallMode(d *dialog) (tea.Cmd, bool) {
	if len(d.rows) == 0 || m.pending {
		return nil, false
	}
	id := d.rows[d.index].id
	if d.kind == "grow-config" && id == "selection_call_mode" {
		mode := "bundled"
		if m.selectionString("selection_call_mode") == "bundled" {
			mode = "separate"
		}
		return m.submitSelectionCallMode(&dialog{kind: "selection-call-mode", parent: d, rows: []row{{id: mode}}}), true
	}
	if d.kind == "eval-policy-judge" && id == "call_mode" {
		pid, _ := d.args["id"].(string)
		p := m.findEvaluationPolicy(pid)
		j, ok := d.args["judge"].(int)
		if p == nil || !ok || j < 0 || j >= len(p.Judges) {
			return nil, true
		}
		mode := "bundled"
		if p.Judges[j].CallMode == "bundled" {
			mode = "separate"
		}
		return m.submitPolicyJudge(&dialog{kind: "eval-policy-judge-call", parent: d, args: d.args, rows: []row{{id: mode}}}), true
	}
	return nil, false
}

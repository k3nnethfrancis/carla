package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Inspection is a read-only projection of saved evidence. The full payload stays
// available; transport events are only expanded when explicitly requested.
type inspectionPage struct {
	title, text, retry string
	children           []*inspectionPage
}

func inspectionMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func inspectionList(v any) []any         { a, _ := v.([]any); return a }
func inspectionJSON(v any) string        { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
func inspectionLabel(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return "Record"
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
func inspectionScalar(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return "Not recorded"
	}
	if m := inspectionMap(v); len(m) > 0 {
		for _, key := range []string{"name", "alias", "id"} {
			if name, ok := m[key].(string); ok && name != "" {
				return name
			}
		}
		return "Saved configuration"
	}
	return fmt.Sprint(v)
}
func inspectionIdentity(v any, fallback string) string {
	m := inspectionMap(v)
	for _, key := range []string{"title", "name", "label", "id", "role"} {
		if s, ok := m[key].(string); ok && s != "" {
			return s
		}
	}
	return fallback
}
func inspectionCount(n int, singular string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %ss", n, singular)
}
func inspectionSummary(m map[string]any) string {
	fields := map[string]any{}
	for key, value := range m {
		fields[key] = value
	}
	for _, key := range []string{"passed", "reason", "evidence"} {
		if v, ok := inspectionMap(m["result"])[key]; ok {
			fields[key] = v
		}
	}
	for _, key := range []string{"name", "model"} {
		if _, ok := fields[key]; !ok {
			if v, exists := inspectionMap(m["definition"])[key]; exists {
				fields[key] = v
			}
		}
	}
	m = fields
	var lines []string
	for _, k := range []string{"title", "name", "id", "status", "passed", "error", "reason", "evidence", "created", "finished", "model", "provider", "call_mode", "phase", "tokens", "parent", "retry_of", "selected"} {
		if v, ok := m[k]; ok && v != nil {
			label := inspectionLabel(k)
			if k == "passed" {
				label = "Verdict passed"
				if status, ok := m["status"].(string); ok && status != "complete" {
					lines = append(lines, "Verdict: unavailable (assessment incomplete)")
					continue
				}
			}
			lines = append(lines, label+": "+inspectionScalar(v))
		}
	}
	if len(lines) == 0 {
		return "Saved record. Open a field below to inspect its evidence."
	}
	return strings.Join(lines, "\n")
}
func inspectionRecord(title string, value any) *inspectionPage {
	p := &inspectionPage{title: title}
	switch v := value.(type) {
	case map[string]any:
		p.text = inspectionSummary(v)
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			field := v[key]
			switch field.(type) {
			case map[string]any, []any:
				if key == "events" {
					p.children = append(p.children, &inspectionPage{title: fmt.Sprintf("Raw token events · %d", len(inspectionList(field))), text: inspectionJSON(field)})
					continue
				}
				p.children = append(p.children, inspectionRecord(inspectionLabel(key), field))
			default:
				if field != nil {
					p.children = append(p.children, &inspectionPage{title: inspectionLabel(key), text: inspectionScalar(field)})
				}
			}
		}
	case []any:
		p.text = fmt.Sprintf("%d saved records", len(v))
		for i, item := range v {
			p.children = append(p.children, inspectionRecord(fmt.Sprintf("%d · %s", i+1, inspectionIdentity(item, "Record")), item))
		}
	default:
		p.text = inspectionScalar(value)
	}
	return p
}
func inspectionMonitoring(records []any) *inspectionPage {
	p := &inspectionPage{title: "Monitoring · no result recorded", text: "No monitoring result was saved for this generation. Historical records may not contain the run's policy settings."}
	state := "complete"
	for _, raw := range records {
		record := inspectionMap(raw)
		checks := inspectionList(record["monitor_checks"])
		if len(checks) == 0 && len(inspectionMap(record["monitor"])) > 0 {
			checks = []any{record["monitor"]}
		}
		for _, check := range checks {
			c := inspectionMap(check)
			status := inspectionScalar(c["status"])
			switch status {
			case "unavailable", "failed", "partial", "error":
				state = "error"
			default:
				if state != "error" && status != "complete" {
					state = status
				}
			}
			p.children = append(p.children, inspectionRecord(fmt.Sprintf("%s · check %d · %s", inspectionIdentity(record, "Generation"), len(p.children)+1, status), check))
		}
	}
	if len(p.children) > 0 {
		p.title = "Monitoring · " + state + " · " + inspectionCount(len(p.children), "check")
		p.text = "Monitoring checks are separate from generation. An unavailable judge is an execution error, not a behavior verdict."
	}
	return p
}
func inspectionGeneration(record map[string]any, inherited bool) *inspectionPage {
	p := &inspectionPage{title: inspectionIdentity(record, "Generation"), text: inspectionSummary(record)}
	if inherited {
		p.text = "Inherited generation evidence. This document has no generation trace of its own.\n\n" + p.text
	}
	for _, key := range []string{"prompt", "text", "settings", "policy_config", "trace"} {
		if v, ok := record[key]; ok {
			label := inspectionLabel(key)
			if key == "text" {
				label = "Saved text"
			}
			p.children = append(p.children, inspectionRecord(label, v))
		}
	}
	return p
}
func inspectionTree(data map[string]any) *inspectionPage {
	root := &inspectionPage{title: "Inspect"}
	overview := &inspectionPage{title: "Overview", text: inspectionSummary(data)}
	generation := &inspectionPage{title: "Generation", text: "No generation evidence recorded."}
	records := []any{}
	selection := inspectionList(data["selection_runs"])
	evaluations := inspectionList(data["evaluations"])
	if node := inspectionMap(data["node"]); len(node) > 0 {
		root.title = "Inspect · " + inspectionIdentity(node, "Document")
		overview.text = inspectionSummary(node)
		if len(inspectionMap(node["policy_config"])) == 0 {
			overview.text += "\nPolicy settings: not recorded (historical run)."
		}
		if config := inspectionMap(node["policy_config"]); len(config) > 0 {
			overview.children = append(overview.children, inspectionRecord("Saved policy settings", config))
		}
		source := inspectionMap(data["generation"])
		if len(source) == 0 {
			source = node
		}
		inherited, _ := data["generation_inherited"].(bool)
		generation = inspectionGeneration(source, inherited)
		generation.title = "Generation"
		records = append(records, node)
	} else if convos := inspectionList(data["conversations"]); len(convos) > 0 {
		root.title = "Inspect · " + inspectionIdentity(data, "Simulation")
		if index, ok := data["inspected_conversation"].(float64); ok {
			label := inspectionIdentity(convos[0], fmt.Sprintf("Conversation %.0f", index+1))
			root.title = "Inspect · " + label
			overview.text += fmt.Sprintf("\nConversation: %.0f · %s", index+1, label)
		}
		generation.text = "Generation evidence for the inspected conversations."
		for i, raw := range convos {
			convo := inspectionMap(raw)
			page := &inspectionPage{title: inspectionIdentity(convo, fmt.Sprintf("Conversation %d", i+1)), text: inspectionSummary(convo)}
			for j, turn := range inspectionList(convo["turns"]) {
				t := inspectionMap(turn)
				g := inspectionGeneration(t, false)
				g.title = fmt.Sprintf("Turn %d · %s", j+1, inspectionScalar(t["role"]))
				page.children = append(page.children, g)
				records = append(records, t)
			}
			generation.children = append(generation.children, page)
		}
	} else if data["kind"] == "selection" || data["steps"] != nil {
		selection = []any{data}
	} else if results := inspectionList(data["results"]); results != nil {
		evaluations = results
		root.title = "Inspect · " + inspectionIdentity(data, "Evaluation run")
		overview.children = append(overview.children, inspectionRecord("Saved evaluation policy", data["policy"]))
	} else {
		evaluations = []any{data}
	}
	monitor := inspectionMonitoring(records)
	policies := &inspectionPage{title: "Selection · no result recorded", text: "No selection attempt is linked to this exact item."}
	for i, raw := range selection {
		r := inspectionMap(raw)
		p := inspectionRecord(fmt.Sprintf("Attempt %d · %s", i+1, inspectionScalar(r["status"])), raw)
		if id, ok := r["id"].(string); ok && len(inspectionList(r["steps"])) > 0 {
			p.retry = id
		}
		policies.children = append(policies.children, p)
	}
	if len(selection) > 0 {
		policies.title = "Selection · " + inspectionCount(len(selection), "attempt")
		policies.text = "Saved candidate assessments, branch choices and retries. Each attempt retains its original policy and evidence."
	}
	evals := &inspectionPage{title: "Evaluations · " + inspectionCount(len(evaluations), "result"), text: "Evaluations judge saved content separately from generation. Execution errors are distinct from pass/fail verdicts."}
	for i, raw := range evaluations {
		evals.children = append(evals.children, inspectionRecord(fmt.Sprintf("%d · %s", i+1, inspectionIdentity(raw, "Evaluation")), raw))
	}
	if runs := inspectionList(data["evaluation_runs"]); len(runs) > 0 {
		evals.children = append(evals.children, inspectionRecord("Evaluation runs", runs))
	}
	if summaries := inspectionList(data["selection_summaries"]); len(summaries) > 0 {
		summaryText := []string{}
		for _, raw := range summaries {
			r := inspectionMap(raw)
			lines := []string{inspectionSummary(r)}
			for _, stepRaw := range inspectionList(r["steps"]) {
				step := inspectionMap(stepRaw)
				outcomes := inspectionMap(step["outcomes"])
				reasons := inspectionMap(step["reasons"])
				keys := []string{}
				for key := range outcomes {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					line := fmt.Sprintf("Loop %v · candidate %s · %s", step["loop"], key, inspectionScalar(outcomes[key]))
					if reason, ok := reasons[key].(string); ok && reason != "" {
						line += "\n" + reason
					}
					lines = append(lines, line)
				}
			}
			text := strings.Join(lines, "\n\n")
			summaryText = append(summaryText, text)
			for _, attempt := range policies.children {
				if attempt.retry == inspectionScalar(r["id"]) {
					attempt.text = text
				}
			}
		}
		policies.text = strings.Join(summaryText, "\n\n────────────────\n\n")
	}
	overview.text += "\n\n" + monitor.title + "\n" + policies.title + "\n" + evals.title
	config := inspectionMap(inspectionMap(data["node"])["policy_config"])
	if len(config) == 0 {
		config = inspectionMap(data["config"])
	}
	if mode, ok := config["monitor_mode"]; ok {
		overview.text += "\nSaved monitoring setting: " + inspectionScalar(mode)
	}
	if enabled, ok := config["selection_enabled"]; ok {
		overview.text += "\nSaved selection enabled: " + inspectionScalar(enabled)
	}
	root.children = []*inspectionPage{overview, generation, monitor, policies, evals, {title: "Raw data", text: inspectionJSON(data)}}
	return root
}
func (m *model) openInspection(raw json.RawMessage) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		m.status = "Unable to read saved inspection record"
		return
	}
	m.inspectionOrigin = m.focus
	m.inspection = ""
	m.showInspector = false
	m.inspectionParent = nil
	m.showInspectionPage(inspectionTree(data), nil)
}
func inspectionHeading(s string) string { return strings.Join(strings.Fields(safe(s)), " ") }
func (m *model) showInspectionPage(p *inspectionPage, parent *dialog) {
	d := &dialog{kind: "inspection", title: inspectionHeading(p.title), parent: parent, args: map[string]any{"page": p}}
	if p.text != "" {
		d.rows = append(d.rows, row{id: "summary", label: "Summary", preview: p.text})
	}
	for i, child := range p.children {
		d.rows = append(d.rows, row{id: fmt.Sprint(i), label: inspectionHeading(child.title), preview: child.text})
	}
	if p.retry != "" {
		d.rows = append(d.rows, row{id: "retry", label: "Retry selection", preview: "Reassess saved candidates using this attempt's frozen policy. No generation or additional loops."})
	}
	m.dialog = d
}
func (m *model) submitInspection() tea.Cmd {
	d := m.dialog
	p := d.args["page"].(*inspectionPage)
	id := d.rows[d.index].id
	if id == "retry" {
		m.dialog = &dialog{kind: "selection-results", title: "Retry saved selection?", parent: d, args: map[string]any{"run": p.retry}, rows: []row{{id: "cancel", label: "Cancel"}, {id: "confirm", label: "Retry assessment and choice"}}}
		return nil
	}
	child := p
	if id != "summary" {
		for i, c := range p.children {
			if fmt.Sprint(i) == id {
				child = c
				break
			}
		}
	}
	if id != "summary" && len(child.children) > 0 {
		m.showInspectionPage(child, d)
		return nil
	}
	m.inspection = child.title + "\n\n" + child.text
	m.inspectionParent = d
	m.dialog = nil
	m.showInspector = true
	m.focus = 2
	m.reflow()
	m.inspector.GotoTop()
	return nil
}
func (m *model) backFromInspection() bool {
	if m.focus != 2 || m.inspectionParent == nil {
		return false
	}
	m.dialog = m.inspectionParent
	m.inspectionParent = nil
	m.showInspector = false
	m.reflow()
	return true
}

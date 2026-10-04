package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Inspection is a read-only projection of saved evidence. The full payload stays
// available; transport events are only expanded when explicitly requested.
type inspectionPage struct {
	title, text, retry string
	children           []*inspectionPage
	tabs               []*inspectionPage
}

// An asynchronous report may open only while its original navigation context
// remains current. A reply must not take focus back from a different item/tab.
type inspectionLocation struct {
	workspace, run, group string
	command, editing      string
	dialog                *dialog
	view                  workspaceView
	focus, tile           int
	outer                 bool
}
type inspectionRequest struct {
	id       string
	location inspectionLocation
}

func (m *model) inspectionLocation() inspectionLocation {
	location := inspectionLocation{workspace: m.data.Workspace.Path, view: m.workspaceView(), focus: m.focus, outer: m.sectionFocus, dialog: m.dialog, editing: m.editing, command: m.command.Value()}
	if m.section == 3 {
		location.tile, location.group = m.gridSelection, m.gridGroup
		if m.simulation != nil {
			location.run = m.simulation.ID
		}
	}
	return location
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
	for _, k := range []string{"title", "name", "status", "passed", "error", "reason", "evidence", "model", "provider", "call_mode", "phase", "tokens", "parent", "retry_of", "selected"} {
		if v, ok := m[k]; ok && v != nil {
			label := inspectionLabel(k)
			if k == "passed" {
				label = "Verdict"
				if passed, ok := v.(bool); ok {
					if passed {
						v = "PASS"
					} else {
						v = "FAIL"
					}
				}
				if status, ok := m["status"].(string); ok && status != "complete" {
					lines = append(lines, "Verdict: unavailable (assessment incomplete)")
					continue
				}
			}
			lines = append(lines, label+": "+inspectionScalar(v))
		}
	}
	if len(lines) == 0 {
		return "No summary fields recorded."
	}
	return strings.Join(lines, "\n")
}
func inspectionRecord(title string, value any) *inspectionPage {
	p := &inspectionPage{title: title}
	if record := inspectionMap(value); record != nil {
		p.text = inspectionSummary(record)
	} else {
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
			page := inspectionRecord(fmt.Sprintf("%s · check %d · %s", inspectionIdentity(record, "Generation"), len(p.children)+1, status), check)
			if scores := inspectionMap(c["scores"]); len(scores) > 0 {
				page.text += "\n" + inspectionSettings(scores)
			}
			for _, rawCall := range inspectionList(c["calls"]) {
				call := inspectionMap(rawCall)
				if call["error"] != nil {
					page.text += "\n" + inspectionSummary(call)
				}
			}
			p.children = append(p.children, page)
		}
	}
	if len(p.children) > 0 {
		p.title = "Monitoring · " + state + " · " + inspectionCount(len(p.children), "check")
		p.text = "Monitoring checks are separate from generation. An unavailable judge is an execution error, not a behavior verdict."
	}
	return p
}
func inspectionGeneration(record map[string]any, inherited bool) *inspectionPage {
	p := &inspectionPage{title: inspectionIdentity(record, "Generation")}
	fields := map[string]any{}
	for _, key := range []string{"model", "provider", "error"} {
		if value, ok := record[key]; ok {
			fields[key] = value
		} else if value, ok := inspectionMap(record["trace"])[key]; ok {
			fields[key] = value
		}
	}
	if len(fields) > 0 {
		p.text = inspectionSummary(fields)
	}
	if inherited {
		p.text = "Inherited generation evidence. This document has no generation trace of its own.\n\n" + p.text
	}
	for _, key := range []string{"prompt", "settings"} {
		if v, ok := record[key]; ok {
			label := inspectionLabel(key)
			text := inspectionScalar(v)
			if fields := inspectionMap(v); fields != nil {
				text = inspectionSettings(fields)
			}
			p.children = append(p.children, &inspectionPage{title: label, text: text})
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
	root.tabs = []*inspectionPage{overview}
	var absent []string
	if generation.text != "No generation evidence recorded." && (strings.TrimSpace(generation.text) != "" || len(generation.children) > 0) {
		root.tabs = append(root.tabs, &inspectionPage{title: "Generation", text: inspectionSection(generation)})
	}
	if len(monitor.children) > 0 {
		text := monitor.title
		for _, check := range monitor.children {
			text += "\n\n" + inspectionSection(check)
		}
		root.tabs = append(root.tabs, &inspectionPage{title: "Monitoring", text: text})
	} else {
		absent = append(absent, "monitoring")
	}
	if len(selection) > 0 {
		text := policies.text
		if len(inspectionList(data["selection_summaries"])) == 0 {
			for _, attempt := range policies.children {
				text += "\n\n" + attempt.text
			}
		}
		root.tabs = append(root.tabs, &inspectionPage{title: "Selection", text: text})
	} else {
		absent = append(absent, "selection")
	}
	if len(evaluations) > 0 {
		text := ""
		for _, result := range evals.children {
			text += result.text + "\n\n"
		}
		root.tabs = append(root.tabs, &inspectionPage{title: "Evaluations", text: strings.TrimSpace(text)})
	} else {
		absent = append(absent, "evaluations")
	}
	if len(absent) > 0 {
		overview.text += "\n\nNot recorded: " + strings.Join(absent, ", ") + "."
	}
	root.text = overview.text
	root.tabs = append(root.tabs, &inspectionPage{title: "Raw", text: "Open saved records and token streams with Enter. Exact evidence is preserved separately from the readable sections."})
	raw, events := inspectionRaw(data, "Record")
	root.children = append(root.children, &inspectionPage{title: "Raw record · without token events", text: inspectionJSON(raw)})
	root.children = append(root.children, events...)
	for _, attempt := range policies.children {
		if attempt.retry != "" {
			root.children = append(root.children, &inspectionPage{title: "Retry selection · " + attempt.retry, retry: attempt.retry})
		}
	}
	return root
}

func inspectionSettings(fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := []string{}
	for _, k := range keys {
		lines = append(lines, inspectionLabel(k)+": "+inspectionScalar(fields[k]))
	}
	return strings.Join(lines, "\n")
}
func inspectionSection(p *inspectionPage) string {
	text := p.text
	for _, child := range p.children {
		text += "\n\n" + child.title + "\n" + inspectionSection(child)
	}
	return strings.TrimSpace(text)
}

// Keep large streams out of both the readable report and the raw record preview.
func inspectionRaw(value any, path string) (any, []*inspectionPage) {
	var events []*inspectionPage
	switch v := value.(type) {
	case map[string]any:
		result := map[string]any{}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "events" {
				events = append(events, &inspectionPage{title: fmt.Sprintf("Token events · %s · %d", path, len(inspectionList(v[k]))), text: inspectionJSON(v[k])})
				result[k] = "Open Token events for the full saved stream"
			} else {
				child, streams := inspectionRaw(v[k], path+"/"+k)
				result[k] = child
				events = append(events, streams...)
			}
		}
		return result, events
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			child, streams := inspectionRaw(item, fmt.Sprintf("%s/%d", path, i+1))
			result[i] = child
			events = append(events, streams...)
		}
		return result, events
	}
	return value, events
}
func (m *model) openInspection(raw json.RawMessage) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		m.status = "Unable to read saved inspection record"
		return
	}
	m.inspectionOrigin = m.focus
	m.inspectionRoot = inspectionTree(data)
	m.inspectionTab = 0
	m.inspectionScroll = make([]int, len(m.inspectionRoot.tabs))
	m.inspectionParent = nil
	m.dialog = nil
	m.showInspectionReport()
}
func (m *model) showInspectionReport() {
	m.inspection = m.inspectionRoot.tabs[m.inspectionTab].text
	m.showInspector = true
	m.focus = 2
	m.reflow()
	m.inspector.SetYOffset(m.inspectionScroll[m.inspectionTab])
}
func inspectionHeading(s string) string { return strings.Join(strings.Fields(safe(s)), " ") }
func (m *model) openInspectionRaw() {
	if m.inspectionRoot == nil {
		return
	}
	m.inspectionScroll[m.inspectionTab] = m.inspector.YOffset()
	d := &dialog{kind: "inspection", title: "Raw evidence", args: map[string]any{"page": m.inspectionRoot}}
	for i, child := range m.inspectionRoot.children {
		d.rows = append(d.rows, row{id: fmt.Sprint(i), label: inspectionHeading(child.title), preview: "ENTER opens saved evidence. ESC returns to the report."})
	}
	m.dialog = d
}
func (m *model) submitInspection() tea.Cmd {
	d := m.dialog
	if len(d.rows) == 0 {
		return nil
	}
	p := d.args["page"].(*inspectionPage)
	for i, child := range p.children {
		if fmt.Sprint(i) != d.rows[d.index].id {
			continue
		}
		if child.retry != "" {
			m.dialog = &dialog{kind: "selection-results", title: "Retry saved selection?", parent: d, args: map[string]any{"run": child.retry}, rows: []row{{id: "cancel", label: "Cancel"}, {id: "confirm", label: "Retry assessment and choice"}}}
			return nil
		}
		m.inspection = child.title + "\n\n" + child.text
		m.inspectionParent = d
		m.dialog = nil
		m.reflow()
		m.inspector.GotoTop()
		break
	}
	return nil
}
func (m *model) backFromInspection() bool {
	if m.focus != 2 || !m.showInspector || m.inspectionRoot == nil {
		return false
	}
	if m.inspectionParent != nil {
		m.dialog = m.inspectionParent
		m.inspectionParent = nil
	} else {
		m.showInspector = false
		m.focus = m.inspectionOrigin
		m.reflow()
	}
	return true
}

func (m *model) cycleInspectionTab(step int) {
	if m.inspectionRoot == nil || m.inspectionParent != nil {
		return
	}
	m.inspectionScroll[m.inspectionTab] = m.inspector.YOffset()
	m.inspectionTab = (m.inspectionTab + step + len(m.inspectionRoot.tabs)) % len(m.inspectionRoot.tabs)
	m.showInspectionReport()
}
func (m *model) inspectionTabBar(width int) string {
	if m.inspectionRoot == nil || m.inspectionParent != nil {
		return ""
	}
	labels := make([]string, len(m.inspectionRoot.tabs))
	for i, tab := range m.inspectionRoot.tabs {
		labels[i] = tab.title
		if i == m.inspectionTab {
			labels[i] = "[" + labels[i] + "]"
		}
	}
	start, end := 0, len(labels)
	for end-start > 1 && ansi.StringWidth(strings.Join(labels[start:end], "  "))+4 > width {
		if m.inspectionTab-start > end-1-m.inspectionTab {
			start++
		} else {
			end--
		}
	}
	text := strings.Join(labels[start:end], "  ")
	if start > 0 {
		text = "‹ " + text
	}
	if end < len(labels) {
		text += " ›"
	}
	return text
}

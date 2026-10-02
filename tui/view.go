package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var bold = lipgloss.NewStyle().Bold(true)
var dim = lipgloss.NewStyle().Faint(true)
var selectedStyle = lipgloss.NewStyle().Reverse(true)

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

type panel struct {
	kind int
	box  rect
}
type layout struct {
	panels              []panel
	bodyHeight, actionY int
}

// All panels, hit targets and viewports derive from the same geometry. At narrow
// widths the focused pane takes the body, rather than squeezing unreadable text.
func (m *model) layout() layout {
	height := max(4, m.height-10-m.suggestionCount()-len(m.commandHints()))
	l := layout{bodyHeight: height, actionY: 4 + height}
	width := max(1, m.width-2)
	if m.adaptiveBranches() {
		nav := m.branchPaneWidth(width)
		l.panels = []panel{{0, rect{1, 3, nav, height}}}
		if nav < width {
			kind := 1
			if m.showInspector && m.focus == 2 {
				kind = 2
			}
			l.panels = append(l.panels, panel{kind, rect{nav + 2, 3, max(1, width-nav-1), height}})
		}
		return l
	}
	if (m.editing != "" && m.editing != "document") || m.width < 90 {
		focus := m.focus
		if focus == 3 {
			focus = 1
		}
		if m.editing != "" && !(m.notesOpen && m.focus == 0) {
			focus = 1
		}
		l.panels = []panel{{focus, rect{1, 3, width, height}}}
		return l
	}
	nav := 26
	l.panels = append(l.panels, panel{0, rect{1, 3, nav, height}})
	available := width - nav - 1
	if m.showInspector && m.width >= 132 {
		inspector := 34
		document := available - inspector - 1
		l.panels = append(l.panels, panel{1, rect{nav + 2, 3, document, height}}, panel{2, rect{nav + document + 3, 3, inspector, height}})
	} else {
		kind := 1
		if m.showInspector && m.focus == 2 {
			kind = 2
		}
		l.panels = append(l.panels, panel{kind, rect{nav + 2, 3, available, height}})
	}
	return l
}
func safe(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			return r
		}
		return -1
	}, s)
}
func line(s string, w int) string {
	s = ansi.Truncate(s, max(0, w), "…")
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}
func box(title, body string, r rect, active bool) string {
	inner := max(1, r.w-4)
	heading := "  " + title
	if active {
		heading = "› " + title
	}
	titleLine := bold.Render(line(heading, inner))
	if active {
		titleLine = selectedStyle.Render(line(heading, inner))
	}
	contents := []string{titleLine, ""}
	contents = append(contents, strings.Split(body, "\n")...)
	for len(contents) < r.h-2 {
		contents = append(contents, "")
	}
	contents = contents[:min(len(contents), max(1, r.h-2))]
	for i := range contents {
		contents[i] = line(contents[i], inner)
	}
	border := lipgloss.NormalBorder()
	style := lipgloss.NewStyle().Border(border).Padding(0, 1)
	return style.Render(strings.Join(contents, "\n"))
}
func (m *model) renderDocument(width int) string {
	if m.section == 4 && m.editing == "" {
		return m.evaluationView(width)
	}
	if m.section == 3 && m.editing == "" {
		return m.conversationDocument(width)
	}
	if m.section == 0 && m.editing == "" {
		r := m.targetRow()
		if r.kind == "passage" {
			return ansi.Wrap(safe(r.preview), width, "")
		}
		if r.kind == "source" {
			return ansi.Wrap(safe(strings.TrimLeft(r.label, "▸▾ "))+"\n\nExpand with →. Space selects passages; Enter opens the selected set in Branches.\n\n"+m.seedSummary(), width, "")
		}
	}
	text := m.currentText()
	if text == "" && m.data.Current != nil && m.editing == "" {
		return lipgloss.Place(width, m.document.Height(), lipgloss.Center, lipgloss.Center, dim.Render("Empty document"))
	}
	if text == "" && m.data.Current == nil {
		return "Start with a seed.\n\nOpen Library, expand a document and select the passages you want to explore.\n\nContinue writes from the end. Branch lets you choose a position in the text."
	}
	runes := []rune(text)
	var out strings.Builder
	end := 0
	for _, span := range m.data.Current.Origins {
		start, stop := max(end, span.Start), min(len(runes), span.End)
		if start > stop {
			continue
		}
		if start > end {
			out.WriteString(m.documentSpan(runes, end, start, lipgloss.NewStyle()))
		}
		style := lipgloss.NewStyle()
		switch span.Kind {
		case "ai":
			style = m.aiStyle()
		case "edited":
			style = m.humanStyle()
		}
		out.WriteString(m.documentSpan(runes, start, stop, style))
		end = stop
	}
	if end < len(runes) {
		out.WriteString(m.documentSpan(runes, end, len(runes), lipgloss.NewStyle()))
	}
	if m.cursorActive() && m.cursorOffset() == len(runes) {
		out.WriteString(selectedStyle.Render(" "))
	}
	return ansi.Wrap(out.String(), max(1, width), "")
}
func (m *model) reflow() {
	m.command.SetWidth(max(1, m.width-7))
	panels := m.layout().panels
	if m.adaptiveBranches() && len(panels) == 1 && (m.focus == 1 || m.focus == 2) {
		m.focus = 0
	}
	for _, p := range panels {
		if p.kind == 0 && m.adaptiveBranches() {
			m.branchScroll = m.branchHorizontalOffset(p.box)
			rows := m.rows()
			if len(rows) > 0 {
				m.branchScrollRow = rows[min(m.selected, len(rows)-1)].id
			}
		}
		width, height := max(1, p.box.w-4), max(1, p.box.h-5)
		switch p.kind {
		case 1:
			m.document.SetWidth(width)
			m.document.SetHeight(height)
			m.syncCursor(width, height)
			m.document.SetContent(m.renderDocument(width))
			if m.previewChangePending {
				m.previewChangePending = false
				m.revealVersionChange()
			}
			m.editor.SetWidth(width)
			m.editor.SetHeight(height)
			if m.editing != "" {
				// Rewrap changes visual row offsets; keep the cursor visible on resize.
				m.editor, _ = m.editor.Update(nil)
			}
		case 2:
			m.inspector.SetWidth(width)
			m.inspector.SetHeight(height)
			m.inspector.SetContent(ansi.Wrap(safe(m.inspection), width, ""))
		}
	}
	m.revealCursor()
	if m.dialog != nil {
		for i := range m.dialog.fields {
			m.dialog.fields[i].input.SetWidth(max(10, m.dialogRect().w-6))
		}
	}
}
func (m *model) navStart(visible int) int { return max(0, m.selected-max(1, visible)+1) }
func (m *model) navigation(r rect) string {
	if m.notesOpen {
		return m.notesView(r)
	}
	rows := m.rows()
	footer := m.navigationFooter(r)
	// Reserve every footer row so hints wrap instead of being clipped by the panel.
	visible := m.navigationRows(r)
	mIndex := min(m.selected, max(0, len(rows)-1))
	start := max(0, mIndex-visible+1)
	var lines []string
	for i := start; i < min(len(rows), start+visible); i++ {
		item := rows[i]
		label := m.branchRowText(item, r)
		if i == mIndex && m.focus == 0 && !m.sectionFocus && m.dialog == nil && !m.searching {
			label = m.scrollingLabel(m.navigationLabel(item, r), r.w-4)
		} else {
			label = line(label, r.w-4)
		}
		if item.kind == "evaluation" {
			if strings.Contains(item.label, "PASS") {
				label = m.aiStyle().Render(label)
			} else if strings.Contains(item.label, "FAIL") || strings.Contains(item.label, "ERROR") {
				label = m.accent("#A84F39", "#DB937C").Render(label)
			}
		}
		if i == mIndex {
			label = selectedStyle.Render(label)
		}
		if item.kind == "node" {
			label = m.pulseLabel(conversationKey(item.id, 0), label)
		}
		if item.kind == "conversation" && strings.Contains(item.label, "! ") {
			label = m.pulseLabel(item.id, label)
		}
		lines = append(lines, label)
	}
	if len(rows) == 0 {
		lines = append(lines, "Nothing here yet.")
	}
	for len(lines) < visible {
		lines = append(lines, "")
	}

	return strings.Join(append(lines, dim.Render(footer)), "\n")
}
func (m *model) dialogRect() rect {
	width := min(76, m.width-4)
	height := min(m.height-6, 18)
	if m.dialog != nil && strings.HasPrefix(m.dialog.kind, "setup-") && len(m.dialog.rows) > 0 {
		height = min(height, len(m.dialog.rows)+8)
	}
	if m.dialog != nil && len(m.dialog.fields) > 0 {
		extra := 0
		if message, ok := m.dialog.args["error"].(string); ok {
			extra = len(strings.Split(ansi.Wrap(safe("Error: "+message), width-4, ""), "\n"))
		}
		height = min(m.height-4, len(m.dialog.fields)*3+6+extra)
	}
	return rect{(m.width - width) / 2, (m.height - height) / 2, width, height}
}
func (m *model) dialogPreviewLines() []string {
	d := m.dialog
	if d == nil || len(d.rows) == 0 {
		return nil
	}
	r := m.dialogRect()
	lines := strings.Split(ansi.Wrap(safe(d.rows[d.index].preview), r.w-4, ""), "\n")
	limit := max(1, min(4, r.h-7))
	if d.kind == "loom-policy-bundle-confirm" {
		limit = max(1, r.h-7)
	}
	if len(lines) > limit {
		lines = append(lines[:limit-1], "… ENTER to open")
	}
	return lines
}
func (m *model) dialogVisibleRows() int {
	return max(1, m.dialogRect().h-5-len(m.dialogPreviewLines()))
}
func (m *model) dialogStart(visible int) int { return max(0, m.dialog.index-max(1, visible)+1) }
func (m *model) renderDialog() string {
	d := m.dialog
	r := m.dialogRect()
	var body []string
	if d.kind == "help-detail" {
		lines := m.helpLines()
		height := r.h - 5
		start := min(d.index, max(0, len(lines)-height))
		body = append(body, lines[start:min(len(lines), start+height)]...)
		for len(body) < height {
			body = append(body, "")
		}
		body = append(body, m.keyLabel("nav.up")+"/"+m.keyLabel("nav.down")+" scroll · "+m.keyLabel("nav.back")+" back")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box(d.title, strings.Join(body, "\n"), r, true))
	}
	if len(d.rows) > 0 || d.allRows != nil {
		visible := m.dialogVisibleRows()
		start := m.dialogStart(visible)
		for i := start; i < min(len(d.rows), start+visible); i++ {
			label := line(safe(d.rows[i].label), r.w-4)
			if i == d.index {
				label = selectedStyle.Render(m.scrollingLabel(safe(d.rows[i].label), r.w-4))
			}
			body = append(body, label)
		}
		for len(body) < visible {
			body = append(body, "")
		}
		for _, text := range m.dialogPreviewLines() {
			style := dim
			if d.kind == "setup-error" || d.kind == "loom-policy-bundle-confirm" {
				style = m.accent("#A84F39", "#DB937C")
			}
			body = append(body, style.Render(line(text, r.w-4)))
		}
		footer := m.keyLabel("nav.enter") + " open · " + m.keyLabel("nav.back") + " close"
		if d.parent != nil {
			footer = m.keyLabel("nav.enter") + " open · " + m.keyLabel("nav.back") + " back"
		}
		if strings.HasPrefix(d.kind, "setup-") {
			footer = "↑↓ select · ENTER choose · ESC back"
			if d.kind == "setup-home" && d.parent == nil {
				footer = "↑↓ select · ENTER choose · ESC skip"
			}
		}
		if d.kind == "setup-busy" {
			footer = "ESC cancel"
		}
		if d.kind == "help" {
			footer = m.keyLabel("nav.up") + "/" + m.keyLabel("nav.down") + " · " + m.keyLabel("nav.enter") + " details · " + m.keyLabel("nav.back") + " return"
		}
		if d.kind == "keys" {
			footer = "ENTER bind · CTRL+S save · ESC cancel"
		}
		if d.kind == "loom-policy-timing" {
			action := "toggle"
			if len(d.rows) > 0 && d.rows[d.index].id == "interval" {
				action = "edit interval"
			}
			footer = "↑↓ choose · " + m.keyLabel("nav.enter") + " " + action + " · " + m.keyLabel("nav.back") + " back"
		}
		if choice, ok := m.dialogChoice(); ok {
			footer = "SPACE / ←→ change · ENTER open · ESC back"
			if choice.toggle {
				footer = "SPACE / ←→ / ENTER toggle · ESC back"
			} else if d.kind == "operational-policy-list" {
				footer = "SPACE / ←→ toggle · ENTER open · ESC back"
			}
		}
		if d.choicePicker() {
			footer = "←→ choose · SPACE / ENTER apply · ESC back"
		}
		if d.kind == "loom-policy-bundle-confirm" {
			footer = "↑↓ choose · ENTER select · ESC back"
		}
		if d.kind == "judge-prompt-confirm" {
			footer = "↑↓ choose · " + m.keyLabel("nav.enter") + " select · " + m.keyLabel("nav.back") + " keep editing"
		}
		if d.kind == "sim-documents" || d.kind == "eval-add-items" {
			footer = "SPACE select · CTRL+S save · ESC cancel"
		}
		if d.query != "" {
			footer = "Filter: " + d.query + " · " + footer
		} else if d.kind != "keys" && d.kind != "delete" && d.kind != "loom-policy-bundle-confirm" && d.kind != "loom-policy-timing" && !strings.HasPrefix(d.kind, "setup-") {
			if d.kind != "help" && d.kind != "judge-prompt-confirm" && ansi.StringWidth("Type to filter · "+footer) <= r.w-4 {
				footer = "Type to filter · " + footer
			}
		}
		body = append(body, footer)
	} else {
		// Fields scroll with focus on small terminals; form chrome has one size.
		visible := max(1, (r.h-6)/3)
		start := max(0, d.field-visible+1)
		for i := start; i < min(len(d.fields), start+visible); i++ {
			f := d.fields[i]
			label := f.label
			if i == d.field {
				label = "› " + label
			}
			value := f.input.View()
			if d.kind == "settings" {
				value = f.input.Value() + " ▾"
				if i == d.field {
					value = selectedStyle.Render(value)
				}
			}
			body = append(body, bold.Render(label), value, "")
		}
		footer := m.keyLabel("nav.enter") + " next · " + m.keyLabel("save") + " save · " + m.keyLabel("nav.back") + " cancel"
		if d.kind == "loom-policy-key" {
			footer = m.keyLabel("nav.enter") + " save & enable · " + m.keyLabel("nav.back") + " cancel"
		}
		if d.kind == "import" {
			footer = "ENTER next/import · CTRL+ENTER import · ESC cancel"
		}
		if message, ok := d.args["error"].(string); ok {
			body = append(body, m.accent("#A84F39", "#DB937C").Render(ansi.Wrap(safe("Error: "+message), r.w-4, "")))
		}
		if d.kind == "setup-input" {
			footer = "ENTER continue · ESC back"
		}
		if d.kind == "note-new" || d.kind == "note-edit" {
			footer = "ENTER save · CTRL+ENTER save · ESC cancel"
		}
		if d.kind == "settings" {
			footer = "↑↓ field · ENTER change · CTRL+S save · ESC cancel"
			if d.adjusting {
				footer = "↑↓ adjust · ←→ ×10 · ENTER set · ESC back"
			}
			body = append(body, dim.Render(line(settingHelp(d.field), r.w-4)))
		}
		if d.kind == "config-number" {
			footer = "↑↓ adjust · ←→ ×10 · ENTER save · ESC cancel"
		}
		if choice, ok := m.dialogChoice(); ok {
			footer = "SPACE / ←→ change · ENTER open · ESC back"
			if choice.toggle {
				footer = "SPACE / ←→ / ENTER toggle · ESC back"
			} else if d.kind == "operational-policy-list" {
				footer = "SPACE / ←→ toggle · ENTER open · ESC back"
			}
		}
		if d.choicePicker() {
			footer = "←→ choose · SPACE / ENTER apply · ESC back"
		}
		if d.kind == "sim-documents" || d.kind == "eval-add-items" {
			footer = "No anthology documents. Keep a branch first. ESC close"
		}
		body = append(body, line(footer, r.w-4))
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box(d.title, strings.Join(body, "\n"), r, true))
}
func (m *model) View() tea.View {
	if m.width < 60 || m.height < 18 {
		v := tea.NewView(lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, fmt.Sprintf("Carla needs 60 × 18 cells\nCurrent: %d × %d\nResize · ESC discard draft · CTRL+C quit", m.width, m.height)))
		v.AltScreen = true
		return v
	}
	if m.dialog != nil {
		v := tea.NewView(m.renderDialog())
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}
	l := m.layout()
	parts := []string{}
	sections := []string{"Library", "Branches", "Anthology", "Simulator", "Evaluate"}
	for _, p := range l.panels {
		title, body := "", ""
		switch p.kind {
		case 0:
			title = sections[m.section]
			if m.section == 4 && m.evalArea != "" {
				title = strings.Title(m.evalArea)
			}
			if m.notesOpen {
				title = "Notes"
			}
			body = m.navigation(p.box)
		case 1:
			if m.gridVisible() {
				parts = append(parts, m.renderGrid(p.box))
				continue
			}
			title = m.nodeTitle()
			if m.section == 4 {
				title = "Evaluate"
				if m.evalArea != "" {
					title += " / " + strings.Title(m.evalArea)
				}
				if c := m.currentEvaluation(); c != nil {
					title += " / " + c.Name
				}
			}
			if m.section == 3 && m.editing == "" {
				title = "Simulator"
				if m.conversationOpen {
					title = m.conversationHeading(p.box.w - 6)
				}
			}
			body = m.document.View() + "\n" + "Source · " + m.aiStyle().Render("AI") + " · " + m.humanStyle().Render("Human edits")
			if (m.section == 1 || m.section == 2) && m.data.Current != nil {
				if info := monitorSummary(m.data.Current.Monitor); info != "" {
					body = m.document.View() + "\n" + dim.Render(safe(info))
				}
			}
			if m.section == 3 || m.section == 4 {
				body = m.document.View()
			}
			if m.editing != "" {
				if (strings.HasPrefix(m.editing, "policy-") || m.editing == "policy_prompt" || m.editing == "selection_assessment_prompt" || m.editing == "library-spec") && m.editReturn != nil {
					title = m.editReturn.title + " · Spec"
					if m.editing == "policy-judge-prompt" || m.editing == "policy_prompt" || m.editing == "selection_assessment_prompt" {
						title = m.editReturn.title + " · Prompt template"
						if m.editing == "selection_assessment_prompt" {
							title = m.editReturn.title + " · Assessment template"
						}
						if m.editing == "policy_prompt" {
							title = m.editReturn.title + " · Choice template"
						}
					}
					if (m.editing == "policy-behavior-new" || m.editing == "library-spec") && len(m.editReturn.fields) > 0 {
						title = m.editReturn.fields[0].input.Value() + " · Spec"
					}
				}
				title += fmt.Sprintf(" · line %d/%d", m.editor.Line()+1, m.editor.LineCount())
				body = m.editor.View()
			}
		case 2:
			title = "Inspector"
			body = m.inspector.View()
			if m.inspection == "" {
				body = "Exact inputs, source attribution and policy decisions appear here."
			}
		}
		parts = append(parts, box(title, body, p.box, m.focus == p.kind || m.editing != "" && p.kind == 1))
	}
	heading := "carla / " + safe(m.data.Workspace.Name)
	modelName := ""
	for _, model := range m.data.Models {
		if model.Alias == m.data.ModelAlias {
			modelName = model.Name
		}
	}
	if m.section == 3 {
		character, visitor := m.modelName(m.simString("character_alias")), m.modelName(m.simString("visitor_alias"))
		prefix := "Next "
		if m.simulation != nil {
			prefix = "Run "
			for _, c := range m.simulation.Conversations {
				for _, t := range c.Turns {
					if t.Role == "character" && t.Model.Name != "" {
						character = t.Model.Name
					}
					if t.Role == "visitor" && t.Model.Name != "" {
						visitor = t.Model.Name
					}
				}
			}
		}
		modelName = prefix + "C: " + strings.Split(character, " · ")[0] + " · V: " + strings.Split(visitor, " · ")[0]
	} else {
		modelName = "Next: " + modelName
	}
	if m.section == 4 {
		items, training := 0, 0
		for _, group := range m.data.EvaluationSets {
			if m.evalCollection != "" && group.ID != m.evalCollection {
				continue
			}
			for _, item := range group.Items {
				items++
				if item.Training {
					training++
				}
			}
		}
		modelName = fmt.Sprintf("%d items · %d training · policy: %s", items, training, m.activePolicyName())
	}

	activeDocuments := 0
	for _, n := range m.data.Nodes {
		if n.Status == "generating" || n.Status == "running" || n.Status == "queued" {
			activeDocuments++
		}
	}
	if activeDocuments > 0 {
		heading += fmt.Sprintf(" · ▶ %d active docs", activeDocuments)
	}
	header := line(bold.Render(heading), max(20, m.width/2)) + dim.Render(line(safe(modelName), max(1, m.width-2-max(20, m.width/2))))
	lines := []string{"", " " + header, m.sectionBar(), lipgloss.NewStyle().PaddingLeft(1).Render(lipgloss.JoinHorizontal(lipgloss.Top, joinPanels(parts)...)), " " + dim.Render(line(m.targetLabel(), m.width-2))}
	if m.section == 0 && m.editing == "" {
		lines[len(lines)-1] = " " + dim.Render(line(m.seedSummary(), m.width-2))
	}
	lines = append(lines, m.commandView())
	status := m.status
	if m.data.Busy && m.backgroundStatus != "" && !strings.HasPrefix(status, "Error:") {
		status = m.backgroundStatus
		if m.capacityStatus != "" {
			status = m.capacityStatus + " · " + status
		}
	}
	if m.editing != "" {
		status = "Editing · " + m.keyLabel("save") + " / " + m.keyLabel("save.alt") + " save · " + m.keyLabel("nav.back") + " cancel"
		if m.editing == "document" {
			status += " · /save · /cancel"
		}
	}
	saved := ""
	if strings.HasPrefix(status, "Saved locally") {
		saved, status = "Saved locally", ""
	}
	lines = append(lines, " "+m.statusView(status))
	legend := m.keyLabel("nav.next") + " next · " + m.keyLabel("nav.prev") + " back · " + m.keyLabel("nav.back") + " out · /keys"
	if !m.sectionFocus && m.editing == "" && m.focus != 3 {
		if m.section == 0 {
			legend = m.keyLabel("select") + " select · " + m.keyLabel("nav.enter") + " to Branches · " + m.keyLabel("nav.next") + " next"
		} else if m.section == 4 {
			legend = m.keyLabel("select") + " select · " + m.keyLabel("nav.enter") + " opens · /eval · /config"
			if m.focus == 1 {
				legend = "↑↓ scroll · TAB next · /inspect · /notes"
			}
		} else if m.section == 3 {
			legend = m.keyLabel("select") + " select/clear · " + m.keyLabel("nav.enter") + " select & open · /clear"
		} else if m.section == 1 || m.section == 2 {
			legend = m.keyLabel("select") + " select · " + m.keyLabel("nav.enter") + " open/actions · " + m.keyLabel("nav.next") + " next"
			if m.focus == 1 {
				legend = "ENTER actions · " + m.keyLabel("continue") + " continue · " + m.keyLabel("branch") + " fork"
			}
		}
	}

	if m.gridVisible() && m.focus == 1 && !m.sectionFocus {
		legend = "Arrows select · " + m.keyLabel("nav.enter") + " open · /grid return · " + m.keyLabel("nav.next") + " next"
		if m.section == 3 {
			legend = "Arrows browse · " + m.keyLabel("select") + " select/clear · " + m.keyLabel("nav.enter") + " open · ESC list"
		}
	}
	if m.section == 3 && m.conversationOpen && m.focus == 1 && m.editing == "" {
		legend = "ESC grid/list · SPACE select/clear · /continue · /loom · /branch"
	}
	if m.editing != "" && m.editing != "document" {
		legend = "↑↓ / PGUP/PGDN scroll · /save · ESC cancel"
	}
	if m.notesOpen && m.focus == 0 {
		legend = "↑↓ notes · ENTER edit note · + new · PGUP/PGDN scroll · TAB document"
	}
	if m.sectionFocus {
		legend = m.keyLabel("nav.left") + "/" + m.keyLabel("nav.right") + " sections · " + m.keyLabel("nav.enter") + " open · /keys"
	}
	if m.focus == 3 && len(m.commandHints()) > 0 {
		legend = "↑↓ select · ENTER run · ESC return · /keys"
		if len(strings.Fields(m.command.Value())) > 1 || m.historyPosition > 0 {
			legend = "↑↓ history · ENTER run · ESC return · /keys"
		}
	}
	lines = append(lines, m.footerView(legend, saved))
	content := strings.Join(lines, "\n")
	// JoinHorizontal emits multiline panels; outer padding belongs to every row.
	content = lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(content)
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "carla · " + m.data.Workspace.Name
	return v
}
func joinPanels(parts []string) []string {
	out := []string{}
	for i, p := range parts {
		if i > 0 {
			out = append(out, " ")
		}
		out = append(out, p)
	}
	return out
}

var sectionNames = []string{"Library", "Branches", "Anthology", "Simulator", "Evaluate"}

func (m *model) sectionLabels() []string {
	labels := []string{}
	for _, name := range sectionNames {
		labels = append(labels, " "+name+" ")
	}
	return labels
}
func (m *model) sectionRects() []rect {
	var out []rect
	x := 1
	for _, s := range m.sectionLabels() {
		w := ansi.StringWidth(s)
		out = append(out, rect{x, 2, w, 1})
		x += w + 1
	}
	return out
}
func (m *model) sectionBar() string {
	labels := m.sectionLabels()
	for i, s := range labels {
		if i == m.section {
			if m.sectionFocus {
				s = bold.Underline(true).Render(s)
			}
			labels[i] = selectedStyle.Render(s)
		}
	}
	bar := " " + strings.Join(labels, " ")
	if m.gridVisible() {
		summary := m.gridSummary()
		room := m.width - 1 - ansi.StringWidth(bar)
		if room >= ansi.StringWidth(summary)+2 {
			bar += strings.Repeat(" ", room-ansi.StringWidth(summary)) + m.helpStyle().Render(summary)
		}
	}
	return bar
}

func (m *model) navigationRows(r rect) int {
	return max(1, r.h-6-strings.Count(m.navigationFooter(r), "\n"))
}
func (m *model) navigationFooter(r rect) string {
	rows := m.rows()
	footer := fmt.Sprintf("%d selected · CTRL+F filter", len(m.data.Selected))
	if m.section != 0 {
		footer = fmt.Sprintf("%d items", len(rows))
		if m.section == 3 {
			count := len(m.selectedConversations())
			footer = fmt.Sprintf("%d selected · /clear", count)
		}
		if m.section == 4 {
			count := 0
			for _, selected := range m.evalSelection {
				if selected {
					count++
				}
			}
			footer = fmt.Sprintf("%d selected · ENTER opens", count)
		}
		if m.section == 1 || m.section == 2 {
			footer = fmt.Sprintf("%d selected · %s actions", len(m.selectedBranches()), m.keyLabel("nav.enter"))
		}
	}
	if m.adaptiveBranches() && m.branchHorizontalLimit(r) > 0 {
		footer += "\n" + m.keyLabel("tree.scroll-left") + "/" + m.keyLabel("tree.scroll-right") + " scroll"
	}
	if m.searching {
		footer = m.search.View()
	} else if m.filter != "" {
		footer = "Filter: " + m.filter
	}
	if !m.searching && m.filter == "" && ansi.StringWidth(footer) > r.w-4 {
		footer = strings.ReplaceAll(footer, " · ", "\n")
	}
	footer = ansi.Wrap(footer, max(1, r.w-4), "")
	return footer
}

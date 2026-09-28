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
	height := max(6, m.height-10-m.suggestionCount()-len(m.commandHints()))
	l := layout{bodyHeight: height, actionY: 4 + height}
	width := max(1, m.width-2)
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
	if text == "" {
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
	for _, p := range m.layout().panels {
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
	visible := max(1, r.h-6)
	mIndex := min(m.selected, max(0, len(rows)-1))
	start := max(0, mIndex-visible+1)
	var lines []string
	for i := start; i < min(len(rows), start+visible); i++ {
		item := rows[i]
		label := strings.Repeat(" ", item.depth) + safe(item.label)
		label = line(label, r.w-4)
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
	footer := fmt.Sprintf("%d selected · CTRL+F filter", len(m.data.Selected))
	if m.section != 0 {
		footer = fmt.Sprintf("%d items", len(rows))
		if m.section == 3 {
			count := 0
			if m.loomConversation != nil {
				count = 1
			}
			footer = fmt.Sprintf("%d selected · /clear", count)
		}
		if m.section == 4 {
			count := 0
			for _, selected := range m.evalSelection {
				if selected {
					count++
				}
			}
			footer = fmt.Sprintf("%d selected · ENTER actions", count)
		}
		if m.section == 1 || m.section == 2 {
			footer = fmt.Sprintf("%d selected · %s actions", len(m.selectedBranches()), m.keyLabel("nav.enter"))
		}
	}
	if m.searching {
		footer = m.search.View()
	} else if m.filter != "" {
		footer = "Filter: " + m.filter
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
		height = min(m.height-4, len(m.dialog.fields)*3+6)
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
	if len(d.rows) > 0 || d.allRows != nil {
		visible := m.dialogVisibleRows()
		start := m.dialogStart(visible)
		for i := start; i < min(len(d.rows), start+visible); i++ {
			label := line(safe(d.rows[i].label), r.w-4)
			if i == d.index {
				label = selectedStyle.Render(label)
			}
			body = append(body, label)
		}
		for len(body) < visible {
			body = append(body, "")
		}
		for _, text := range m.dialogPreviewLines() {
			style := dim
			if d.kind == "setup-error" {
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
			footer = m.keyLabel("nav.up") + "/" + m.keyLabel("nav.down") + " commands · " + m.keyLabel("nav.back") + " return"
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
			}
		}
		if d.choicePicker() {
			footer = "←→ choose · SPACE / ENTER apply · ESC back"
		}
		if d.kind == "sim-documents" {
			footer = "SPACE select · CTRL+S save · ESC cancel"
		}
		if d.query != "" {
			footer = "Filter: " + d.query + " · " + footer
		} else if d.kind != "keys" && d.kind != "delete" && d.kind != "loom-policy-timing" && !strings.HasPrefix(d.kind, "setup-") {
			footer = "Type to filter · " + footer
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
			if message, ok := d.args["error"].(string); ok {
				body = append(body, m.accent("#A84F39", "#DB937C").Render(line("Error: "+message, r.w-4)))
			}
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
			}
		}
		if d.choicePicker() {
			footer = "←→ choose · SPACE / ENTER apply · ESC back"
		}
		if d.kind == "sim-documents" {
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
			if m.section == 3 && m.editing == "" {
				title = "Simulator"
				if m.conversationOpen {
					title = m.conversationHeading(p.box.w - 6)
				}
			}
			body = m.document.View() + "\n" + "Source · " + m.aiStyle().Render("AI") + " · " + m.humanStyle().Render("Human edits")
			if m.section == 3 || m.section == 4 {
				body = m.document.View()
			}
			if m.editing != "" {
				title += " · editing"
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
		training := 0
		for _, e := range m.data.Evaluations {
			if e.Training {
				training++
			}
		}
		modelName = fmt.Sprintf("%d evaluations · %d training", len(m.data.Evaluations), training)
	}
	header := line(bold.Render(heading), max(20, m.width/2)) + dim.Render(line(safe(modelName), max(1, m.width-2-max(20, m.width/2))))
	lines := []string{"", " " + header, m.sectionBar(), lipgloss.NewStyle().PaddingLeft(1).Render(lipgloss.JoinHorizontal(lipgloss.Top, joinPanels(parts)...)), " " + dim.Render(line(m.targetLabel(), m.width-2))}
	if m.section == 0 && m.editing == "" {
		lines[len(lines)-1] = " " + dim.Render(line(m.seedSummary(), m.width-2))
	}
	if m.selectionVisible() {
		lines[len(lines)-1] = m.selectionBar()
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
		status = "Editing · " + m.keyLabel("save") + " save · " + m.keyLabel("nav.back") + " cancel"
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
			legend = m.keyLabel("select") + " select · " + m.keyLabel("nav.enter") + " actions · /eval · /policy"
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
	if m.section == 3 && m.conversationOpen && m.focus == 1 {
		legend = "ESC grid/list · SPACE select/clear · /loom · /fork · /edit · /visitor"
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

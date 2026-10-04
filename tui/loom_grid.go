package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func conversationStatus(c simulationConversation) string {
	if c.Status == "policy_stopped" {
		return "Stopped by policy"
	}
	if c.Status != "needs_review" {
		return c.Status
	}
	reasons := []string{}
	for _, t := range c.Turns {
		for _, flag := range t.Flags {
			switch flag {
			case "token_limit":
				reasons = append(reasons, "token cap reached")
			case "empty":
				reasons = append(reasons, "empty reply")
			case "unexpected_role_boundary":
				reasons = append(reasons, "unexpected speaker marker")
			}
		}
	}
	if len(reasons) == 0 {
		return "Stopped early · output flagged"
	}
	return "Stopped early · " + strings.Join(reasons, ", ")
}
func (m *model) gridItems() []loomTile {
	if m.section == 1 {
		items := append([]loomTile(nil), m.loomTiles...)
		for i := range items {
			for _, n := range m.data.Nodes {
				if n.ID == items[i].ID {
					items[i].Title = documentStatusLabel(n, items[i].Title)
					if info := monitorSummary(n.Monitor); info != "" {
						items[i].Status = strings.Trim(items[i].Status+" · "+info, " ·")
					}
				}
			}
		}
		return items
	}
	if m.section != 3 || m.simulation == nil {
		return nil
	}
	items := []loomTile{}
	targets := []conversationParent{}
	if m.gridGroup != "" {
		if scope := m.simulationGroupScope(m.gridGroup); scope != nil {
			targets = simulationScopeLeaves(*scope)
		}
	}
	if len(targets) == 0 {
		for _, c := range m.simulation.Conversations {
			targets = append(targets, conversationParent{Run: m.simulation.ID, Conversation: c.Index})
		}
	}
	for _, target := range targets {
		run := m.simulationViews[target.Run]
		if m.simulation.ID == target.Run {
			run = m.simulation
		}
		c := simulationConversation{Index: target.Conversation, Status: "loading"}
		if run != nil && target.Conversation < len(run.Conversations) {
			c = run.Conversations[target.Conversation]
		}

		var body strings.Builder
		for _, t := range c.Turns {
			speaker := "Visitor"
			if t.Role == "character" {
				speaker = "Character"
			}
			fmt.Fprintf(&body, "%s\n%s\n", speaker, t.Text)
			fmt.Fprintln(&body)
		}
		title := conversationName(c, false)
		summary := m.selectionRunSummary(target.Run)
		outcome := m.selectionOutcome(summary)
		if summary.Loop > 0 {
			title = fmt.Sprintf("L%d · B%d · %s", summary.Loop, summary.AlternativeIndex+1, title)
		}
		status := conversationStatus(c)
		if info := conversationMonitorStatus(c); info != "" {
			status += " · " + info
		}
		if outcome != "" {
			status += " · " + outcome
		}
		if flags := conversationFlags(c); flags != "" {
			status += " · " + flags
		}

		if m.conversationSelected(target.Run, c.Index) {
			title = "✓ " + title
		}
		items = append(items, loomTile{
			ID: conversationKey(target.Run, c.Index), Title: title, Text: body.String(), Status: status,
		})
	}
	return items
}

func (m *model) gridVisible() bool {
	return m.loomGrid && m.editing == "" && len(m.gridItems()) > 1
}
func (m *model) gridGeometry(r rect) (cols, rows, size int) {
	cols = 1
	if r.w >= 64 {
		cols = 2
	}
	rows = min(2, max(1, r.h/7))
	size = cols * rows
	return
}

// Tiles use the same rectangles for rendering and clicking, including resize.
func (m *model) gridRects(r rect) []rect {

	cols, _, size := m.gridGeometry(r)
	count := min(size, len(m.gridItems())-m.gridPage(r)*size)
	rows := (count + cols - 1) / cols
	out := []rect{}
	usableH := r.h - rows + 1
	for y := 0; y < rows; y++ {
		top, bottom := y*usableH/rows, (y+1)*usableH/rows
		rowCols := min(cols, count-y*cols)
		usableW := r.w - rowCols + 1
		for x := 0; x < rowCols; x++ {
			left, right := x*usableW/rowCols, (x+1)*usableW/rowCols
			out = append(out, rect{r.x + left + x, r.y + top + y, right - left, bottom - top})
		}
	}
	return out
}
func (m *model) gridPage(r rect) int {
	items := m.gridItems()
	_, _, size := m.gridGeometry(r)
	selected := min(m.gridSelection, max(0, len(items)-1))
	if m.gridFollow {
		for i, item := range items {
			if strings.HasPrefix(item.Status, "generating") || strings.HasPrefix(item.Status, "running") {
				selected = i
				break
			}
		}
	}
	return selected / size
}
func (m *model) renderGrid(r rect) string {
	items := m.gridItems()
	cols, _, size := m.gridGeometry(r)
	page := m.gridPage(r)
	lines := []string{}
	rects := m.gridRects(r)
	rows := (len(rects) + cols - 1) / cols
	for y := 0; y < rows; y++ {
		tiles := []string{}
		for x := 0; x < cols; x++ {
			slot := y*cols + x
			if slot >= len(rects) {
				break
			}
			rr := rects[slot]
			i := page*size + slot
			tile := strings.Repeat(" ", rr.w)
			if i < len(items) {
				item := items[i]
				status := item.Status
				text := strings.Split(ansi.Wrap(safe(strings.TrimSpace(item.Text)), max(1, rr.w-4), ""), "\n")
				// Each tile follows its own output tail; opening it shows the full text.
				height := max(0, rr.h-4)
				if len(text) > height {
					text = text[len(text)-height:]
				}
				heading := item.Title
				selected := m.focus == 1 && !m.sectionFocus && i == m.gridSelected(r)
				headingStyle := bold
				if selected {
					headingStyle = selectedStyle.Bold(true)
				}
				statusStyle := m.helpStyle()
				if strings.HasPrefix(status, "Stopped early") || status == "failed" {
					statusStyle = m.accent("#A84F39", "#DB937C")
				}
				content := []string{m.pulseLabel(item.ID, headingStyle.Render(line(heading, rr.w-4))), statusStyle.Render(line(status, rr.w-4))}
				for _, s := range text {
					content = append(content, line(s, rr.w-4))
				}
				for len(content) < rr.h-2 {
					content = append(content, line("", rr.w-4))
				}
				border := lipgloss.NormalBorder()
				if selected {
					border = lipgloss.DoubleBorder()
				}
				style := lipgloss.NewStyle().Border(border).Padding(0, 1)
				if color := m.pulseColor(item.ID); color != "" {
					style = style.BorderForeground(lipgloss.Color(color))
				}
				tile = style.Render(strings.Join(content, "\n"))
			} else {
				tile = lipgloss.NewStyle().Width(rr.w).Height(rr.h).Render("")
			}
			tiles = append(tiles, tile)
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, joinPanels(tiles)...))
		if y < rows-1 {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

// Selection is visible immediately on entering the grid, before the first arrow.
func (m *model) gridSelected(r rect) int {
	if m.gridPinned {
		return min(m.gridSelection, len(m.gridItems())-1)
	}
	_, _, size := m.gridGeometry(r)
	return m.gridPage(r) * size
}
func (m *model) gridSummary() string {
	for _, p := range m.layout().panels {
		if p.kind == 1 {
			_, _, size := m.gridGeometry(p.box)
			n := len(m.gridItems())
			heading := "Loom"
			if m.section == 3 && m.simulation != nil {
				r := m.selectionRunSummary(m.simulation.ID)
				if r.Loop > 0 {
					attempt := m.selectionAttempt(r.PolicyRun)
					heading = fmt.Sprintf("Loop %d/%d", r.Loop, attempt.Loops)
					if attempt.Status == "failed" {
						heading += " · selection blocked"
					}
				}
			}
			if m.gridFollow {
				heading += " · following live"
			}
			return fmt.Sprintf("%s · %d outputs · page %d/%d", heading, n, m.gridPage(p.box)+1, (n+size-1)/size)
		}
	}
	return ""
}

func (m *model) openGridTile(index int) tea.Cmd {
	items := m.gridItems()
	if index < 0 || index >= len(items) {
		return nil
	}
	if m.section == 1 && items[index].ID == "" {
		m.status = "This branch is queued"
		return nil
	}
	if m.section == 1 && m.pending {
		m.status = "Wait for the current preview"
		return nil
	}
	m.gridFollow = false
	m.gridPinned = true
	m.loomGrid = false
	m.conversationOpen = m.section == 3
	m.focus = 1
	m.gridSelection = index
	if m.section == 1 {
		m.notesOpen = false
		m.selectDocument(items[index].ID)
		return m.send("node.open", map[string]any{"node": items[index].ID})
	}
	if target, ok := gridConversation(items[index].ID); ok {
		m.gridSelection = target.Conversation
		m.selectLoomConversation(map[string]any{"run": target.Run, "conversation": target.Conversation})
		if cached := m.simulationViews[target.Run]; cached != nil {
			m.simulation = cached
		} else if m.simulation == nil || m.simulation.ID != target.Run {
			return m.send("simulator.open", map[string]any{"run": target.Run, "conversation": target.Conversation})
		}
	}
	m.selectConversationRow()
	m.reflow()
	m.document.GotoTop()
	return nil
}
func (m *model) gridKey(key string) (tea.Cmd, bool) {
	if !m.gridVisible() || m.focus != 1 {
		return nil, false
	}
	var r rect
	for _, p := range m.layout().panels {
		if p.kind == 1 {
			r = p.box
		}
	}
	cols, _, size := m.gridGeometry(r)
	if !m.gridPinned {
		m.gridSelection = m.gridPage(r) * size
	}
	step := 0
	switch key {
	case "nav.left":
		step = -1
	case "nav.right":
		step = 1
	case "nav.up":
		step = -cols
	case "nav.down":
		step = cols
	case "nav.enter":
		return m.openGridTile(m.gridSelection), true
	default:
		return nil, false
	}
	m.gridPinned = true
	m.gridFollow = false
	m.gridSelection = max(0, min(len(m.gridItems())-1, m.gridSelection+step))
	return nil, true
}

// Compact persisted metadata; full results and errors belong in Inspect.
func monitorSummary(result monitorResult) string {
	label := ""
	switch result.Status {
	case "":
		return ""
	case "unavailable", "failed", "error":
		label = "monitoring error"
	case "partial":
		label = "monitoring incomplete"
	default:
		label = "monitoring " + result.Status
	}
	warn, stop := 0, 0
	for _, d := range result.Detections {
		switch d.Action {
		case "warn":
			warn++
		case "stop":
			stop++
		}
	}
	if warn > 0 {
		label += fmt.Sprintf(" · %d warn", warn)
	}
	if stop > 0 {
		label += fmt.Sprintf(" · %d stop", stop)
	}
	return label
}

func conversationMonitorStatus(c simulationConversation) string {
	status := monitorResult{}
	rank := 0
	for _, turn := range c.Turns {
		for _, check := range append([]monitorResult{turn.Monitor}, turn.MonitorChecks...) {
			n := 1
			switch check.Status {
			case "":
				n = 0
			case "partial":
				n = 2
			case "checking", "queued", "running":
				n = 3
			case "unavailable", "failed", "error":
				n = 4
			}
			if n >= rank && n > 0 {
				status, rank = check, n
			}
		}
	}
	// Behavior flags are summarized separately from status in the title.
	status.Detections = nil
	return monitorSummary(status)
}

func gridConversation(id string) (conversationParent, bool) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return conversationParent{}, false
	}
	n, err := strconv.Atoi(parts[1])
	return conversationParent{Run: parts[0], Conversation: n}, err == nil
}

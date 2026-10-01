package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m *model) adaptiveBranches() bool {
	return m.section == 1 && !m.notesOpen && m.editing == "" && !m.gridVisible()
}

// Measure all expanded rows, not the hovered row: navigation cannot resize the
// panes. Small cell increments preserve as much document space as possible.
func (m *model) branchPaneWidth(available int) int {
	needed := 4
	for _, r := range m.rows() {
		needed = max(needed, r.depth+ansi.StringWidth(safe(r.label))+4)
	}
	// Four-cell increments avoid wasting a quarter screen on a short name.
	return min(available, max(24, (needed+3)/4*4))
}

func (m *model) branchHorizontalLimit(r rect) int {
	longest := 0
	for _, item := range m.rows() {
		longest = max(longest, item.depth+ansi.StringWidth(safe(item.label)))
	}
	return max(0, longest-max(1, r.w-4))
}

func (m *model) branchHorizontalOffset(r rect) int {
	if !m.adaptiveBranches() {
		return 0
	}
	offset := min(m.branchScroll, m.branchHorizontalLimit(r))
	rows := m.rows()
	if len(rows) > 0 {
		item := rows[min(m.selected, len(rows)-1)]
		if item.id != m.branchScrollRow {
			// On a new row show its newest operation, leaving a little ancestor indent.
			offset = min(m.branchHorizontalLimit(r), max(0, item.depth-max(2, (r.w-4)/4)))
		}
	}
	return offset
}

func (m *model) scrollBranches(delta int) {
	for _, p := range m.layout().panels {
		if p.kind != 0 {
			continue
		}
		m.branchScroll = max(0, min(m.branchHorizontalOffset(p.box)+delta, m.branchHorizontalLimit(p.box)))
		rows := m.rows()
		if len(rows) > 0 {
			m.branchScrollRow = rows[min(m.selected, len(rows)-1)].id
		}
	}
	// Deliberate horizontal navigation takes precedence until focus changes.
	m.labelScroll.manualKey = m.focusedLabelTarget().key
}

func (m *model) branchRowText(item row, r rect) string {
	text := strings.Repeat(" ", item.depth) + safe(item.label)
	if !m.adaptiveBranches() {
		return text
	}
	offset := m.branchHorizontalOffset(r)
	return ansi.Cut(text, offset, offset+max(1, r.w-4))
}

func (m *model) treeRowX(item row, r rect) int {
	return r.x + 2 + item.depth - m.branchHorizontalOffset(r)
}

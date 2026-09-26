package main

import "strings"

// Walk parent links, rather than insertion order: new siblings may arrive after
// their cousins. Collapse is UI state and never deletes a saved continuation.
func (m *model) branchRows() []row {
	children := map[string][]node{}
	known := map[string]bool{}
	for _, n := range m.data.Nodes {
		known[n.ID] = true
	}
	for _, n := range m.data.Nodes {
		parent := n.Parent
		if !known[parent] {
			parent = ""
		}
		children[parent] = append(children[parent], n)
	}
	var rows []row
	var walk func(string, int)
	walk = func(parent string, depth int) {
		for _, n := range children[parent] {
			label := documentLabel(n)
			// The parent tree supplies source context; detached anthology rows
			// and document headings retain the complete identifier.
			if depth > 0 && m.section != 2 && n.Label != "" && label == n.Label {
				parts := strings.Split(label, "-")
				if len(parts) >= 3 {
					label = strings.Join(parts[len(parts)-2:], "-")
				}
			}
			if n.Kept {
				label = "★ " + label
			}
			if n.Status == "empty" {
				label += " · no new text"
			} else if n.Status != "complete" {
				label += " · " + n.Status
			}
			check := m.selectionMark(n.ID)
			if m.section == 2 {
				if n.Kept {
					rows = append(rows, row{id: n.ID, label: check + label, kind: "node", preview: n.Preview})
				}
			} else {
				marker := "  "
				if len(children[n.ID]) > 0 {
					marker = "▾ "
					if m.collapsed[n.ID] {
						marker = "▸ "
					}
				}
				rows = append(rows, row{id: n.ID, label: marker + check + label, kind: "node", preview: n.Preview, depth: min(depth, 3)})
			}
			if m.section == 2 || m.filter != "" || !m.collapsed[n.ID] {
				walk(n.ID, depth+1)
			}
		}
	}
	walk("", 0)
	return rows
}
func (m *model) hasChildren(id string) bool {
	for _, n := range m.data.Nodes {
		if n.Parent == id {
			return true
		}
	}
	return false
}
func (m *model) branchArrow(key string) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	m.selected = min(m.selected, len(rows)-1)
	id := rows[m.selected].id
	if key == "right" {
		if m.hasChildren(id) {
			if m.collapsed[id] {
				m.collapsed[id] = false
			} else {
				m.selected = min(m.selected+1, len(rows)-1)
			}
		}
		return
	}
	if m.hasChildren(id) && !m.collapsed[id] {
		m.collapsed[id] = true
		return
	}
	for _, n := range m.data.Nodes {
		if n.ID == id {
			for i, r := range rows {
				if r.id == n.Parent {
					m.selected = i
					return
				}
			}
		}
	}
}

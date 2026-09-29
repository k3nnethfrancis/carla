package main

import (
	"fmt"
	"strings"
)

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
	grouped := map[string]bool{}
	if m.section == 1 {
		for _, set := range m.visibleDocumentSets() {
			for _, id := range set.Members {
				grouped[id] = true
			}
		}
	}
	var walk func(string, int)
	walk = func(parent string, depth int) {
		for _, n := range children[parent] {
			if grouped[n.ID] {
				continue
			}
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
	if m.section == 1 {
		sets := m.visibleDocumentSets()
		logical := []string{}
		for _, set := range sets {
			id := set.SetID
			if id == "" {
				id = set.ID
			}
			found := false
			for _, old := range logical {
				found = found || old == id
			}
			if !found {
				logical = append(logical, id)
			}
		}
		var addSet func(documentSet, int, string)
		addSet = func(set documentSet, depth int, label string) {
			arrow := "▾ "
			if m.collapsed[set.ID] {
				arrow = "▸ "
			}
			mark := "  "
			all := len(set.Members) > 0
			for _, id := range set.Members {
				all = all && m.branchSelection[id]
			}
			if all {
				mark = "✓ "
			}
			rows = append(rows, row{id: set.ID, kind: "document-set", depth: depth, label: mark + arrow + label + fmt.Sprintf(" · %d documents", len(set.Members))})
			if m.collapsed[set.ID] {
				return
			}
			for _, id := range set.Members {
				for _, n := range m.data.Nodes {
					if n.ID == id {
						rows = append(rows, row{id: id, kind: "node", label: m.selectionMark(id) + documentLabel(n), depth: depth + 1, preview: n.Preview})
						walk(id, depth+2)
					}
				}
			}
		}
		for i, id := range logical {
			var versions []documentSet
			for _, set := range sets {
				if set.SetID == id || set.SetID == "" && set.ID == id {
					versions = append(versions, set)
				}
			}
			head := versions[len(versions)-1]
			for _, set := range versions {
				if set.ID == m.data.DocumentSetHeads[id] {
					head = set
				}
			}
			addSet(head, 0, fmt.Sprintf("Set %d · %s", i+1, head.Action))
			if !m.collapsed[head.ID] {
				for _, old := range versions {
					if old.ID != head.ID {
						addSet(old, 1, "Previous version")
					}
				}
			}
		}
	}
	walk("", 0)
	return rows
}
func (m *model) hasChildren(id string) bool {
	if len(m.documentSetMembers(id)) > 0 {
		return true
	}
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

func (m *model) documentSetMembers(id string) []string {
	for _, s := range m.data.DocumentSets {
		if s.ID == id {
			return s.Members
		}
	}
	return nil
}

// Provenance retains removed members. Only complete saved groups are actionable;
// surviving documents from incomplete groups remain reachable in their ancestry tree.
func (m *model) visibleDocumentSets() []documentSet {
	known := map[string]bool{}
	for _, n := range m.data.Nodes {
		known[n.ID] = true
	}
	var out []documentSet
	for _, s := range m.data.DocumentSets {
		complete := len(s.Members) > 0
		for _, id := range s.Members {
			complete = complete && known[id]
		}
		if complete {
			out = append(out, s)
		}
	}
	return out
}

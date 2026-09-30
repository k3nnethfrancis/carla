package main

import (
	"fmt"
	"strconv"
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
			for _, id := range m.documentSetMembers(set.ID) {
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
			label = documentStatusLabel(n, label)
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
			for _, id := range m.documentSetMembers(set.ID) {
				all = all && m.branchSelection[id]
			}
			if all {
				mark = "✓ "
			}
			rows = append(rows, row{id: set.ID, kind: "document-set", depth: depth, label: mark + arrow + m.documentSetActivity(set.ID) + label + fmt.Sprintf(" · %d documents", len(set.Members))})
			if m.collapsed[set.ID] {
				return
			}
			var addScope func(actionScope, string, int)
			addScope = func(scope actionScope, path string, level int) {
				if scope.Kind == "set" {
					arrow := "▾ "
					if m.collapsed[path] {
						arrow = "▸ "
					}
					mark := "  "
					if m.branchSelection["set:"+path] {
						mark = "✓ "
					}
					rows = append(rows, row{id: path, kind: "document-set", depth: level, label: mark + arrow + m.documentSetActivity(path) + "Set"})
					if m.collapsed[path] {
						return
					}
					for i, c := range scope.Children {
						addScope(c, fmt.Sprintf("%s/%d", path, i), level+1)
					}
					return
				}
				for _, n := range m.data.Nodes {
					if n.ID == scope.Node {
						rows = append(rows, row{id: n.ID, kind: "node", label: m.selectionMark(n.ID) + documentStatusLabel(n, documentLabel(n)), depth: level, preview: n.Preview})
						walk(n.ID, level+1)
						break
					}
				}
			}
			scope := m.documentScope(set.ID)
			for i, c := range scope.Children {
				addScope(c, fmt.Sprintf("%s/scope/%d", set.ID, i), depth+1)
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
	if strings.Contains(id, "/scope/") {
		var ids []string
		var visit func(actionScope)
		visit = func(s actionScope) {
			if s.Kind == "document" {
				ids = append(ids, s.Node)
			}
			for _, c := range s.Children {
				visit(c)
			}
		}
		visit(m.documentScope(id))
		return ids
	}
	for _, s := range m.data.DocumentSets {
		if s.ID == id {
			members := append([]string(nil), s.Members...)
			if head := m.data.DocumentSetHeads[s.SetID]; head == "" || head == s.ID {
				for i, id := range members {
					for _, n := range m.data.Nodes {
						if n.ID == id {
							if next := m.data.DocumentHeads[n.DocumentID]; next != "" {
								members[i] = next
							}
							break
						}
					}
				}
			}
			return members
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
		// Singleton operation records are provenance, not another collection level.
		// Keep their document revisions in the ordinary ancestry tree.
		complete := len(s.Members) > 1
		for _, id := range s.Members {
			complete = complete && known[id]
		}
		if complete {
			out = append(out, s)
		}
	}
	return out
}

// documentScope preserves saved nested membership while resolving current heads.
// Set IDs describe provenance; exact leaf IDs remain the execution authority.
func (m *model) documentScope(id string) actionScope {
	if base, path, ok := strings.Cut(id, "/scope/"); ok {
		scope := m.documentScope(base)
		for _, part := range strings.Split(path, "/") {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(scope.Children) {
				return actionScope{Kind: "set"}
			}
			scope = scope.Children[index]
		}
		scope.ID = ""
		return scope
	}
	for _, set := range m.data.DocumentSets {
		if set.ID != id {
			continue
		}
		members := m.documentSetMembers(id)
		replacements := map[string]string{}
		for i, old := range set.Members {
			replacements[old] = members[i]
		}
		var resolve func(actionScope) actionScope
		resolve = func(s actionScope) actionScope {
			if s.Kind == "document" {
				if next := replacements[s.Node]; next != "" {
					s.Node = next
				}
				return s
			}
			s.Children = append([]actionScope(nil), s.Children...)
			for i, c := range s.Children {
				s.Children[i] = resolve(c)
			}
			// Nested historical IDs are provenance, not current containing-set IDs.
			s.ID = ""
			return s
		}
		result := actionScope{Kind: "set", ID: id}
		if set.Scope != nil {
			result = resolve(*set.Scope)
			result.ID = id
			result.Kind = "set"
			if len(result.Children) == 0 {
				result.Children = []actionScope{{Kind: "document", Node: members[0]}}
				result.Node = ""
			}
		} else {
			for _, member := range members {
				result.Children = append(result.Children, actionScope{Kind: "document", Node: member})
			}
		}
		return result
	}
	return actionScope{Kind: "set"}
}

func documentStatusLabel(n node, label string) string {
	switch n.Status {
	case "generating", "running":
		label = "▶ generating · " + label
	case "queued":
		label = "◷ queued · " + label
	case "", "complete":
	case "empty":
		label = "no new text · " + label
	default:
		label = strings.ReplaceAll(n.Status, "_", " ") + " · " + label
	}
	if len(n.Monitor.Detections) > 0 {
		label = "! " + label
	}
	if n.Monitor.Status == "checking" {
		label = "checking · " + label
	}
	return label
}
func (m *model) documentSetActivity(id string) string {
	active := 0
	for _, member := range m.documentSetMembers(id) {
		for _, n := range m.data.Nodes {
			if n.ID == member && (n.Status == "generating" || n.Status == "running" || n.Status == "queued") {
				active++
			}
		}
	}
	if active > 0 {
		return fmt.Sprintf("▶ %d active · ", active)
	}
	return ""
}

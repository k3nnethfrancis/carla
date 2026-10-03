package main

func (m *model) hasDocumentOperations() bool {
	for _, set := range m.data.DocumentSets {
		if set.OperationID != "" {
			return true
		}
	}
	return false
}

// Operation wrappers are a view over saved provenance, never replacement document
// IDs. Leaves remain actionable versions; Loom parents preserve alternative sets.
func (m *model) operationBranchRows() []row {
	nodes := map[string]node{}
	parents := map[string]string{}
	labels := map[string]string{}
	kinds := map[string]string{}
	order := []string{}
	for _, n := range m.data.Nodes {
		nodes[n.ID] = n
		parents[n.ID], labels[n.ID], kinds[n.ID] = n.Parent, documentLabel(n), "node"
		order = append(order, n.ID)
	}
	seen := map[string]bool{}
	for _, set := range m.data.DocumentSets {
		if set.OperationID == "" {
			continue
		}
		complete := len(set.Members) > 0
		for _, member := range set.Members {
			_, ok := nodes[member]
			complete = complete && ok
		}
		if !complete {
			continue
		}
		if set.Action != "loom" {
			// A one-document operation is already represented by its new revision.
			// Multi-document operations retain their selectable collection scope.
			if len(set.Members) > 1 {
				parents[set.ID], labels[set.ID], kinds[set.ID] = "", set.Label, "document-set"
				order = append(order, set.ID)
				for _, member := range set.Members {
					parents[member] = set.ID
				}
			}
			continue
		}
		op := "loom:" + set.OperationID
		if !seen[op] {
			seen[op] = true
			parent := ""
			if set.SourceScope != nil && set.SourceScope.Kind == "document" {
				parent = set.SourceScope.Node
			}
			// Single-source legacy scopes can still be attached by exact node ancestry.
			if parent == "" {
				parent = nodes[set.Members[0]].Parent
				for _, member := range set.Members {
					if nodes[member].Parent != parent {
						parent = ""
						break
					}
				}
			}
			parents[op], labels[op], kinds[op] = parent, set.OperationLabel, "document-set"
			order = append(order, op)
		}
		if len(set.Members) == 1 {
			parents[set.Members[0]] = op
		} else {
			parents[set.ID], labels[set.ID], kinds[set.ID] = op, set.Label, "document-set"
			order = append(order, set.ID)
			for _, member := range set.Members {
				parents[member] = set.ID
			}
		}
	}
	children := map[string][]string{}
	for _, id := range order {
		parent := parents[id]
		if _, ok := kinds[parent]; !ok {
			parent = ""
		}
		children[parent] = append(children[parent], id)
	}
	var rows []row
	visited := map[string]bool{}
	var walk func(string, int)
	walk = func(parent string, depth int) {
		for _, id := range children[parent] {
			if visited[id] {
				continue
			}
			visited[id] = true
			label := labels[id]
			marker := "  "
			if len(children[id]) > 0 {
				marker = "▾ "
				if m.collapsed[id] {
					marker = "▸ "
				}
			}
			check := m.selectionMark(id)
			preview := ""
			if n, ok := nodes[id]; ok {
				if n.Kept {
					label = "★ " + label
				}
				label, preview = documentStatusLabel(n, label), n.Preview
			} else {
				members := m.documentSetMembers(id)
				all := len(members) > 0
				for _, member := range members {
					all = all && m.branchSelection[member]
				}
				check = "  "
				if all {
					check = "✓ "
				}
				label = m.documentSetActivity(id) + label
			}
			rows = append(rows, row{id: id, kind: kinds[id], label: marker + check + label, preview: preview, depth: depth})
			if !m.collapsed[id] || m.filter != "" {
				walk(id, depth+1)
			}
		}
	}
	walk("", 0)
	return rows
}

// Tree depth is the authority for navigating synthetic parents as well as nodes.
func parentBranchRow(rows []row, index int) int {
	if index < 0 || index >= len(rows) {
		return index
	}
	for i := index - 1; i >= 0; i-- {
		if rows[i].depth < rows[index].depth {
			return i
		}
	}
	return index
}

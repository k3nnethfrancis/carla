package main

// Simulator containment has one authoritative tree. Enumerating leaves is only
// for dispatch/checkmarks; it must never replace the stored hierarchy.
func simulationScopeLeaves(scope actionScope) []conversationParent {
	if scope.Kind == "conversation" {
		return []conversationParent{{Run: scope.Run, Conversation: scope.Conversation}}
	}
	var out []conversationParent
	for _, child := range scope.Children {
		out = append(out, simulationScopeLeaves(child)...)
	}
	return out
}
func findSimulationScope(scope *actionScope, id string) *actionScope {
	if scope == nil {
		return nil
	}
	if scope.ID == id {
		return scope
	}
	for i := range scope.Children {
		if found := findSimulationScope(&scope.Children[i], id); found != nil {
			return found
		}
	}
	return nil
}
func (m *model) simulationGroupScope(id string) *actionScope {
	runs := m.simulationSummaries()
	for _, r := range runs {
		if s := findSimulationScope(r.AlternativeScope, id); s != nil {
			return s
		}
	}
	scope := actionScope{Kind: "set", ID: id}
	seen := map[string]bool{}
	for _, r := range runs {
		if r.AlternativeGroup != id && alternativeKey(r) != id {
			continue
		}
		key := alternativeKey(r)
		if r.AlternativeScope != nil {
			if !seen[key] {
				scope.Children = append(scope.Children, *r.AlternativeScope)
				seen[key] = true
			}
		} else {
			child := actionScope{Kind: "set", ID: r.ID}
			for _, c := range r.Conversations {
				child.Children = append(child.Children, actionScope{Kind: "conversation", Run: r.ID, Conversation: c.Index})
			}
			scope.Children = append(scope.Children, child)
		}
	}
	if len(scope.Children) == 0 {
		return nil
	}
	return &scope
}

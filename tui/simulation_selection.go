package main

import (
	"fmt"
	"strings"
)

// Selection is an operation target, not the highlighted preview. A parent covers
// all its children; selecting a child replaces that scope with an explicit set.
// Resolve it once for commands, evaluation, checkmarks and the action preview.
type simulationSelection struct {
	Group         string
	Members       []conversationParent
	Run           string
	All           bool
	Conversations map[int]bool
}

func (m *model) selectSimulation(run string) {
	m.simSelection = &simulationSelection{Run: run, All: true}
}
func (m *model) selectLoomConversation(target map[string]any) {
	m.simSelection = &simulationSelection{Run: target["run"].(string), Conversations: map[int]bool{target["conversation"].(int): true}}
}
func (m *model) toggleConversation(run string, index int) {
	if m.simSelection == nil || m.simSelection.Run != run || m.simSelection.All {
		m.simSelection = &simulationSelection{Run: run, Conversations: map[int]bool{index: true}}
		return
	}
	if m.simSelection.Conversations[index] {
		delete(m.simSelection.Conversations, index)
	} else {
		m.simSelection.Conversations[index] = true
	}
	if len(m.simSelection.Conversations) == 0 {
		m.simSelection = nil
	}
}
func (m *model) conversationSelected(run string, index int) bool {
	s := m.simSelection
	if s != nil && (s.Group != "" || len(s.Members) > 0) {
		for _, t := range m.selectedConversations() {
			if t.Run == run && t.Conversation == index {
				return true
			}
		}
		return false
	}
	return s != nil && s.Run == run && (s.All || s.Conversations[index])
}
func (m *model) selectedConversations() []conversationParent {
	if m.simSelection == nil {
		return nil
	}
	if m.simSelection.Group != "" {
		if scope := m.simulationGroupScope(m.simSelection.Group); scope != nil {
			return simulationScopeLeaves(*scope)
		}
		var out []conversationParent
		for _, run := range m.simulationSummaries() {
			if alternativeKey(run) == m.simSelection.Group || run.AlternativeGroup == m.simSelection.Group {
				for _, c := range run.Conversations {
					out = append(out, conversationParent{Run: run.ID, Conversation: c.Index})
				}
			}
		}
		return out
	}
	if len(m.simSelection.Members) > 0 {
		return m.simSelection.Members
	}
	var conversations []simulationConversation
	if m.simulation != nil && m.simulation.ID == m.simSelection.Run {
		conversations = m.simulation.Conversations
	} else {
		for _, run := range m.data.SimulationRuns {
			if run.ID == m.simSelection.Run {
				conversations = run.Conversations
				break
			}
		}
	}
	var targets []conversationParent
	for _, c := range conversations {
		if m.conversationSelected(m.simSelection.Run, c.Index) {
			targets = append(targets, conversationParent{Run: m.simSelection.Run, Conversation: c.Index})
		}
	}
	return targets
}
func (m *model) loomConversationTarget() (map[string]any, bool) {
	if m.section != 3 || m.simSelection == nil {
		return nil, false
	}
	if m.simSelection.Group != "" {
		if scope := m.simulationGroupScope(m.simSelection.Group); scope != nil {
			return map[string]any{"scope": *scope}, true
		}
	}
	targets := m.selectedConversations()
	if len(targets) == 0 {
		return nil, false
	}
	children := []actionScope{}
	for _, t := range targets {
		children = append(children, actionScope{Kind: "conversation", Run: t.Run, Conversation: t.Conversation})
	}
	if len(children) == 1 && !m.simSelection.All {
		return map[string]any{"scope": children[0]}, true
	}
	return map[string]any{"scope": actionScope{Kind: "set", ID: m.simSelection.Run, Children: children}}, true
}

// Plan is shared by the command preview and execution: the same target, defaults
// and validation must describe what will actually run.
func (m *model) simulationLoomPlan(options generationOptions) (map[string]any, string, error) {
	targets := m.selectedConversations()
	if m.simSelection != nil && len(targets) == 0 {
		return nil, "", fmt.Errorf("selected conversations are no longer available; /clear starts fresh")
	}
	if options.Action == "" {
		options.Action = "loom"
	}
	if options.Action == "continue" && len(targets) == 0 {
		return nil, "", fmt.Errorf("select a conversation or set to continue; /loom starts fresh")
	}
	if options.Action == "continue" && (options.Count > 0) {
		return nil, "", fmt.Errorf("/continue advances the selected set; use /loom for alternatives")
	}
	if options.Action == "loom" && options.Count == 0 {
		options.Count = 1
	}
	if options.Turns == 0 && m.simSelection == nil {
		options.Turns = 1
	}
	turns := options.Turns
	if turns == 0 {
		turns = 1
		if n, ok := m.data.SimulatorConfig["turns"].(float64); ok && n > 0 {
			turns = int(n)
		}
	}
	unit := "turns"
	if turns == 1 {
		unit = "turn"
	}
	noun := func(n int, singular string) string {
		if n == 1 {
			return singular
		}
		return singular + "s"
	}
	label := fmt.Sprintf("Start %d %s · %d %s each", max(1, options.Count), noun(max(1, options.Count), "conversation"), turns, unit)
	if len(targets) > 0 {
		label = fmt.Sprintf("Continue %d selected %s · %d %s each", len(targets), noun(len(targets), "conversation"), turns, unit)
		if options.Action == "loom" && options.Count > 1 {
			label = fmt.Sprintf("Create %d alternative %s of %d %s · %d %s each", options.Count, noun(options.Count, "set"), len(targets), noun(len(targets), "conversation"), turns, unit)
		}
	}
	if options.Message != "" {
		label += " · send visitor message to each"
	}
	if options.Loops > 1 {
		selection := m.data.SelectionEnabled
		if options.Selection != nil {
			selection = *options.Selection
		}
		if selection {
			label += fmt.Sprintf(" · %d loops: choose a winner, split again", options.Loops)
		} else {
			label += fmt.Sprintf(" · %d loops: split once, continue all", options.Loops)
		}
	}
	args := map[string]any{}
	options.apply(args)
	if target, ok := m.loomConversationTarget(); ok {
		for k, v := range target {
			args[k] = v
		}
	}
	return args, label, nil
}
func (m *model) simulationActionLabel() string {
	options := generationOptions{}
	if fields := strings.Fields(m.command.Value()); len(fields) > 0 {
		for _, alias := range append(m.commandAliases("loom"), m.commandAliases("continue")...) {
			if fields[0] == "/"+alias {
				id := "loom"
				for _, name := range m.commandAliases("continue") {
					if fields[0] == "/"+name {
						id = "continue"
					}
				}
				if parsed, err := parseGenerationOptions(m.command.Value(), id); err == nil {
					parsed.Action = id
					options = parsed
				}
				break
			}
		}
	}
	_, label, err := m.simulationLoomPlan(options)
	if err != nil {
		return err.Error()
	}
	return label
}

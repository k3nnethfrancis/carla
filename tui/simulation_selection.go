package main

import (
	"fmt"
	"strings"
)

// Selection is an operation target, not the highlighted preview. A parent covers
// all its children; selecting a child replaces that scope with an explicit set.
// Resolve it once for commands, evaluation, checkmarks and the action preview.
type simulationSelection struct {
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
	return s != nil && s.Run == run && (s.All || s.Conversations[index])
}
func (m *model) selectedConversations() []conversationParent {
	if m.simSelection == nil {
		return nil
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
	targets := m.selectedConversations()
	if len(targets) == 1 {
		return map[string]any{"run": targets[0].Run, "conversation": targets[0].Conversation}, true
	}
	indices := []int{}
	for _, target := range targets {
		indices = append(indices, target.Conversation)
	}
	return map[string]any{"run": m.simSelection.Run, "conversations": indices}, true
}

// Plan is shared by the command preview and execution: the same target, defaults
// and validation must describe what will actually run.
func (m *model) simulationLoomPlan(options generationOptions) (map[string]any, string, error) {
	targets := m.selectedConversations()
	if m.simSelection != nil && len(targets) == 0 {
		return nil, "", fmt.Errorf("selected conversations are no longer available; /clear starts fresh")
	}
	if options.Message != "" && m.simSelection != nil {
		return nil, "", fmt.Errorf("clear the conversation selection with /clear before using --message for a fresh opening")
	}
	if len(targets) > 1 && options.Count > 0 && options.Count != len(targets) {
		return nil, "", fmt.Errorf("a set continues each selected conversation once; omit the count or select one conversation for alternatives")
	}
	if options.Count == 0 && len(targets) < 2 {
		options.Count = 1
	}
	if options.Loops == 0 {
		options.Loops = 1
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
	label := fmt.Sprintf("Start %d conversations · %d %s each", options.Count, turns, unit)
	if len(targets) > 0 {
		noun := "conversations"
		if len(targets) == 1 {
			noun = "conversation"
		}
		label = fmt.Sprintf("Continue %d selected %s · %d %s each", len(targets), noun, turns, unit)
		if len(targets) == 1 && options.Count > 1 {
			label = fmt.Sprintf("Generate %d alternatives of selected conversation · %d %s each", options.Count, turns, unit)
		}
	}
	if options.Loops > 1 {
		label += fmt.Sprintf(" · %d selection loops", options.Loops)
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
		for _, alias := range m.commandAliases("loom") {
			if fields[0] == "/"+alias {
				if parsed, err := parseGenerationOptions(m.command.Value(), "loom"); err == nil {
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

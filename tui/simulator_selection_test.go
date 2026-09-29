package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/charmbracelet/x/ansi"
	"net"
	"strings"
	"testing"
)

func TestSimulatorExplicitTargetLifecycle(t *testing.T) {
	m := simulatorFixture()
	m.focus, m.selected = 0, 4 // Highlight the second conversation.
	if _, ok := m.loomConversationTarget(); ok {
		t.Fatal("highlight selected a Loom parent")
	}
	space := tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	m.data.Busy = true // UI-only selection also works while generations run.
	m.Update(space)
	if m.simSelection == nil || !m.simSelection.Conversations[1] {
		t.Fatal("Space did not select")
	}
	if !strings.Contains(m.rows()[4].label, "✓") {
		t.Fatal("missing checkmark")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if !m.simSelection.Conversations[1] {
		t.Fatal("arrows retargeted Loom")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(space)
	if m.simSelection != nil {
		t.Fatal("Space did not clear")
	}
	m.openGridTile(1)
	if m.simSelection == nil || !m.simSelection.Conversations[1] {
		t.Fatal("opening did not select")
	}
	m.simulatorBack()
	m.gridSelection = 0
	if !m.simSelection.Conversations[1] {
		t.Fatal("browsing grid changed selected parent")
	}
	m.perform("clear")
	if m.simSelection != nil || !strings.Contains(m.targetLabel(), "new conversation") {
		t.Fatal("clear did not reset target")
	}
}

func TestSimulatorEnterOpensAndSelectsListConversation(t *testing.T) {
	m := simulatorFixture()
	m.focus, m.selected = 0, 4
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m.client = &client{conn: left}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter did not open")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var request struct {
		Command string
		Args    map[string]any
	}
	json.NewDecoder(right).Decode(&request)
	<-done
	if request.Command != "simulator.open" || request.Args["conversation"] != float64(1) || m.simSelection == nil || !m.simSelection.Conversations[1] {
		t.Fatal(request, m.simSelection)
	}
}

func TestLoomUsesCheckedParentNotHighlightedConversation(t *testing.T) {
	m := simulatorFixture()
	m.focus, m.selected = 0, 4
	m.selectLoomConversation(map[string]any{"run": "batch", "conversation": 0})
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m.client = &client{conn: left}
	m.focusCommand(true)
	m.command.SetValue("/loom 4 --turns 2")
	cmd := m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(m.status)
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var request struct {
		Command string
		Args    map[string]any
	}
	json.NewDecoder(right).Decode(&request)
	<-done
	if request.Command != "simulator.run" || request.Args["scope"].(map[string]any)["conversation"] != float64(0) || request.Args["count"] != float64(4) {
		t.Fatal(request)
	}
	if m.simSelection == nil {
		t.Fatal("pending request lost its selection before acceptance")
	}
	m.apply(event{Type: "simulation", Data: json.RawMessage(`{"id":"new","status":"complete","conversations":[{"index":0,"status":"complete","turns":[]}]}`)})
	if m.simSelection == nil || !m.simSelection.All || m.simSelection.Run != "new" {
		t.Fatal("new run must select its output group")
	}
}

func TestVisitorMessageArguments(t *testing.T) {
	for _, input := range []string{`/loom 2 --msg "What's a path?" --turns 2`, `/loom --message="What's a path?" 2 --turns=2`} {
		options, err := parseGenerationOptions(input, "loom")
		if err != nil || options.Message != "What's a path?" || options.Count != 2 || options.Turns != 2 {
			t.Fatal(input, options, err)
		}
	}
	options, err := parseGenerationOptions(`/loom --msg "say \"hello\"; $(leave this alone)"`, "loom")
	if err != nil || options.Message != `say "hello"; $(leave this alone)` {
		t.Fatal(options, err)
	}
	for _, input := range []string{`/loom --msg ""`, `/loom --msg "unfinished`, `/loom --msg one --message two`, `/loom --msg`} {
		if _, err := parseGenerationOptions(input, "loom"); err == nil {
			t.Fatal("accepted", input)
		}
	}
	for _, section := range []int{1} {
		m := fixture()
		m.section = section
		if m.loom(generationOptions{Message: "Hi"}) != nil || !strings.Contains(m.status, "Simulator") {
			t.Fatal("message allowed in documents")
		}
	}
	m := simulatorFixture()
	m.selectLoomConversation(map[string]any{"run": "batch", "conversation": 0})
	args, _, err := m.simulationLoomPlan(generationOptions{Message: "Hi"})
	if err != nil || args["visitor"] != "Hi" {
		t.Fatal(args, err)
	}
}

func TestCompletedPolicyFlagsLeaveSidebarButRemainInHeader(t *testing.T) {
	m := simulatorFixture()
	c := &m.simulation.Conversations[1]
	c.Turns[1].MonitorChecks = []monitorResult{{Detections: []policyDetection{{Name: "looping"}}}}
	c.Status = "running"
	if !strings.Contains(m.rows()[4].label, "!") {
		t.Fatal("running warning missing")
	}
	c.Status = "complete"
	if strings.Contains(m.rows()[4].label, "!") {
		t.Fatal("completed sidebar kept warning")
	}
	m.openGridTile(1)
	for _, width := range []int{45, 80} {
		heading := ansi.Strip(m.conversationHeading(width))
		if !strings.HasPrefix(heading, "Conversation 2 · complete") || !strings.HasSuffix(heading, "! looping") || ansi.StringWidth(heading) != width {
			t.Fatal(heading)
		}
	}
	c.Status = "failed"
	if !strings.Contains(ansi.Strip(m.conversationHeading(80)), "Conversation 2 · failed") {
		t.Fatal("missing status in title")
	}
	if strings.Contains(ansi.Strip(m.conversationDocument(80)), "failed") {
		t.Fatal("status repeated in transcript")
	}
	m.reflow()
	m.document.GotoBottom()
	if !strings.Contains(ansi.Strip(m.View().Content), "! looping") {
		t.Fatal("header warning lost on scroll")
	}
}

func TestDelayedOpenDoesNotRestoreClearedLoomSelection(t *testing.T) {
	m := simulatorFixture()
	m.selectLoomConversation(map[string]any{"run": "batch", "conversation": 1})
	m.perform("clear")
	data := json.RawMessage(`{"id":"batch","status":"complete","opened":true,"open_conversation":1,"conversations":[{"index":0,"turns":[]},{"index":1,"turns":[]}]}`)
	m.apply(event{Type: "simulation", Data: data})
	if m.simSelection != nil {
		t.Fatal("late preview restored cleared selection")
	}
	data = json.RawMessage(`{"id":"fork","status":"draft","opened":true,"forked":true,"open_conversation":0,"conversations":[{"index":0,"turns":[]}]}`)
	m.apply(event{Type: "simulation", Data: data})
	if m.simSelection == nil || m.simSelection.Run != "fork" {
		t.Fatal("explicit fork must become next Loom parent")
	}
}

func TestLoomParentAndSubsetUseOneSelectionResolver(t *testing.T) {
	m := simulatorFixture()
	m.data.SimulatorConfig = map[string]any{"turns": float64(2)}
	m.selectSimulation("batch")
	if len(m.selectedConversations()) != 2 || !m.conversationSelected("batch", 1) {
		t.Fatal("parent must cover children")
	}
	m.toggleConversation("batch", 0)
	if len(m.selectedConversations()) != 1 || m.conversationSelected("batch", 1) {
		t.Fatal("child must replace parent scope")
	}
	m.toggleConversation("batch", 1)
	targets := m.evaluationTargets()
	args, ok := m.loomConversationTarget()
	if !ok || len(targets) != 2 || len(simulationScopeLeaves(args["scope"].(actionScope))) != 2 {
		t.Fatal(targets, args)
	}
	if !strings.Contains(m.targetLabel(), "2 selected conversations · 2 turns each") {
		t.Fatal(m.targetLabel())
	}
	m.toggleConversation("batch", 0)
	args, _ = m.loomConversationTarget()
	if args["scope"].(actionScope).Conversation != 1 {
		t.Fatal(args)
	}
	m.perform("clear")
	if len(m.selectedConversations()) != 0 {
		t.Fatal("clear retained targets")
	}
}

func TestContinueSelectedLoomDispatchesBatchWithConfiguredTurns(t *testing.T) {
	for _, command := range []string{"/loom", "/continue"} {
		m := simulatorFixture()
		m.selectSimulation("batch")
		m.data.SimulatorConfig = map[string]any{"turns": float64(2)}
		left, right := net.Pipe()
		m.client = &client{conn: left}
		m.focusCommand(true)
		m.command.SetValue(command)
		cmd := m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal(m.status)
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		var request struct {
			Command string
			Args    map[string]any
		}
		if err := json.NewDecoder(right).Decode(&request); err != nil {
			t.Fatal(err)
		}
		<-done
		left.Close()
		right.Close()
		if request.Command != "simulator.run" || request.Args["scope"].(map[string]any)["id"] != "batch" || len(request.Args["scope"].(map[string]any)["children"].([]any)) != 2 || request.Args["turns"] != nil {
			t.Fatal(request)
		}
	}
}

func TestSimulationPlanMatchesPreviewAndRejectsAmbiguousCount(t *testing.T) {
	m := simulatorFixture()
	m.selectSimulation("batch")
	m.command.SetValue("/continue --turns 3")
	args, label, err := m.simulationLoomPlan(generationOptions{Action: "continue", Turns: 3})
	if err != nil || args["turns"] != 3 || label != m.simulationActionLabel() {
		t.Fatal(args, label, err)
	}
	args, _, err = m.simulationLoomPlan(generationOptions{Count: 4})
	if err != nil || args["count"] != 4 {
		t.Fatal(args, err)
	}
	if _, _, err = m.simulationLoomPlan(generationOptions{Action: "continue", Count: 4}); err == nil {
		t.Fatal("continue accepted alternatives")
	}
}

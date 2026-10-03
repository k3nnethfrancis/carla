package main

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestMonitoringFailureIsMetadataNotConversationText(t *testing.T) {
	m := simulatorFixture()
	c := &m.simulation.Conversations[0]
	c.Turns[1].Monitor = monitorResult{Status: "unavailable", Error: "private runtime path cannot start"}
	items := m.gridItems()
	if strings.Contains(items[0].Text, "runtime") || !strings.Contains(items[0].Status, "complete · monitoring error") {
		t.Fatal(items[0])
	}
	m.openGridTile(0)
	if strings.Contains(ansi.Strip(m.conversationDocument(80)), "monitor") {
		t.Fatal(m.conversationDocument(80))
	}
	if !strings.Contains(ansi.Strip(m.conversationHeading(120)), "complete · monitoring error") {
		t.Fatal(m.conversationHeading(120))
	}
	c.Turns[1].MonitorChecks = []monitorResult{{Status: "complete"}}
	if conversationMonitorStatus(*c) != "monitoring error" {
		t.Fatal(conversationMonitorStatus(*c))
	}
}

func TestDocumentGridKeepsMonitorErrorsOutOfText(t *testing.T) {
	m := fixture()
	m.section = 1
	m.loomTiles = []loomTile{{ID: "monitored", Title: "continue-1", Text: "actual document", Status: "complete"}}
	m.data.Nodes = []node{{ID: "monitored", Status: "complete", Monitor: monitorResult{Status: "unavailable", Error: "judge startup details"}}}
	tile := m.gridItems()[0]
	if tile.Text != "actual document" || tile.Status != "complete · monitoring error" {
		t.Fatal(tile)
	}
}

func TestMonitoringWarningsAndStopsRemainCompact(t *testing.T) {
	got := monitorSummary(monitorResult{Status: "complete", Detections: []policyDetection{{Name: "Looping", Action: "warn"}, {Name: "Harm", Action: "stop"}}})
	if got != "monitoring complete · 1 warn · 1 stop" {
		t.Fatal(got)
	}
}

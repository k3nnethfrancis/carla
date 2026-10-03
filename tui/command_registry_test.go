package main

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// Set CARLA_UPDATE_COMMAND_DOCS=1 to regenerate the checked-in reference.
// Normal tests fail on drift; they never write documentation.
func TestCommandReferenceMatchesRegistry(t *testing.T) {
	ids := []string{}
	for id := range commandDescriptions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out strings.Builder
	out.WriteString("# Command reference\n\nGenerated from `tui/command_registry.go`, the metadata used by help and completion.\nFor a first experiment, start with the [README walkthrough](../README.md#from-a-seed-to-evaluated-conversations).\nAvailability depends on the current stage and target. Compatibility commands remain searchable.\n\n")
	for _, id := range ids {
		m := &model{section: 3}
		out.WriteString("## /" + commandName(action{id: id}) + "\n\n")
		out.WriteString(strings.ReplaceAll(m.helpText(id), "\n", "  \n") + "\n\n")
		m.section = 4
		other := m.commandAliases(id)
		m.section = 3
		if strings.Join(other, ",") != strings.Join(m.commandAliases(id), ",") {
			out.WriteString("In Evaluate, aliases: /" + strings.Join(other, " · /") + ".\n\n")
		}
	}
	path := "../docs/command-reference.md"
	if os.Getenv("CARLA_UPDATE_COMMAND_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(out.String()), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != out.String() {
		t.Fatal("Command reference is stale; run: cd tui && CARLA_UPDATE_COMMAND_DOCS=1 go test -run TestCommandReferenceMatchesRegistry")
	}
	for _, a := range allActions {
		m := &model{}
		if commandDescriptions[m.canonicalCommand(a.id)] == "" {
			t.Errorf("missing description for %s", a.id)
		}
	}
}

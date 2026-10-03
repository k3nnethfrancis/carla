package main

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

func TestGenerationPolicyOverridesPreserveAbsentOnAndOff(t *testing.T) {
	for _, action := range []string{"loom", "continue", "generate"} {
		opts, err := parseGenerationOptions("/"+action, action)
		if err != nil || opts.Selection != nil || opts.Monitoring != nil {
			t.Fatal(opts, err)
		}
		args := map[string]any{}
		opts.apply(args)
		if _, ok := args["selection"]; ok {
			t.Fatal(args)
		}
		if _, ok := args["monitoring"]; ok {
			t.Fatal(args)
		}
		opts, err = parseGenerationOptions("/"+action+" --selection=off --monitoring on", action)
		if err != nil || opts.Selection == nil || *opts.Selection || opts.Monitoring == nil || !*opts.Monitoring {
			t.Fatal(opts, err)
		}
		opts.apply(args)
		if args["selection"] != false || args["monitoring"] != true {
			t.Fatal(args)
		}
	}
	for _, input := range []string{
		"/loom --selection true", "/loom --monitoring false", "/loom --selection ON", "/loom --monitoring 0",
		"/loom --selection", "/loom --monitoring=", "/loom --selection on --selection off",
		"/continue --monitoring on --monitoring=on",
	} {
		if _, err := parseGenerationOptions(input, "loom"); err == nil {
			t.Fatal("accepted", input)
		}
	}
}

func TestPolicyOverridesDispatchWithoutPersistingConfig(t *testing.T) {
	for _, section := range []int{1, 3} {
		m := fixture()
		m.section = section
		enabled, disabled := true, false
		opts := generationOptions{Count: 3, Loops: 1, Selection: &enabled, Monitoring: &disabled}
		req := captureCommand(t, m, func() tea.Cmd { return m.loom(opts) })
		if string(req.Args["selection"]) != "true" || string(req.Args["monitoring"]) != "false" || string(req.Args["loops"]) != "1" {
			t.Fatal(req)
		}
		if m.data.SimulatorConfig["selection"] != nil || m.data.SimulatorConfig["monitoring"] != nil {
			t.Fatal("saved per-run overrides")
		}
	}
}

func TestOmittedLoopsStayAbsentFromSimulatorPlan(t *testing.T) {
	m := fixture()
	m.section = 3
	enabled := true
	args, _, err := m.simulationLoomPlan(generationOptions{Count: 3, Selection: &enabled})
	if err != nil || args["loops"] != nil || args["selection"] != true {
		t.Fatal(args, err)
	}
	// Backend owns context validation; the frontend must not fabricate --loops 1.
}

func TestGenerationHelpExplainsPolicyOverrides(t *testing.T) {
	m := fixture()
	for _, command := range []string{"loom", "continue"} {
		text := m.commandHelp(command)
		for _, want := range []string{"--selection on|off", "--monitoring on|off", "--loops 1", "never save"} {
			if !strings.Contains(text, want) {
				t.Fatal(command, want, text)
			}
		}
	}
}

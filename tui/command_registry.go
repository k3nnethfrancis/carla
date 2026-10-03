package main

import (
	"fmt"
	"strings"
)

// Command vocabulary and explanatory metadata. Context-sensitive availability and
// execution remain in their owners; help, completion and the generated reference reuse this file.

type action struct{ id, label, key string }

var allActions = []action{
	{"continue", "Continue", "ctrl+r"}, {"generate", "Generate (alias for continue)", ""}, {"branch", "Branch current version", "ctrl+b"}, {"loom", "Generate alternatives", ""},
	{"add", "Add to seeds", ""}, {"remove", "Remove from collection", ""},
	{"delete", "Delete selected branches", ""}, {"keep", "Keep branch", "k"}, {"grow", "Grow", "g"},
	{"settings", "Settings", "ctrl+t"}, {"models", "Model", "m"},
	{"workspaces", "Workspace", "ctrl+w"}, {"edit", "Edit document", "e"},
	{"inspect", "Inspect generation and policies", "ctrl+e"}, {"review", "Review document", "ctrl+u"},
	{"spec", "Selection spec", "s"}, {"prompt", "Selector prompt", "p"},
	{"snapshot", "Export saved material", "ctrl+s"}, {"clear", "Clear seed selection", "x"},
	{"library", "Library", "1"}, {"branches", "Branches", "2"},
	{"kept", "Anthology", "3"}, {"notes", "Notes", "4"}, {"simulator", "Simulator", "5"}, {"sim-config", "Configure simulator", ""}, {"simulate", "Run conversations", ""}, {"grow-config", "Configure Grow", ""}, {"grow-policy", "Grow selection criteria", ""},
	{"run", "Run Simulator conversations", ""}, {"configure", "Configure this page", ""},
	{"find", "Filter this list", "ctrl+f"}, {"active", "View active generation", ""}, {"rename", "Rename document or conversation", ""},
	{"visitor", "Write a visitor message in a conversation fork", ""}, {"grid", "Show Loom grid", ""}, {"loom-policy", "Configure conversation warnings and stop rules", ""},
	{"character-sampling", "Character temperature, top-p and output tokens", ""}, {"visitor-sampling", "Visitor temperature, top-p and output tokens", ""},
	{"import", "Import a document into Library", ""},
	{"behaviors", "Reusable behavior specs", ""}, {"policy", "Monitoring, selection and evaluation criteria", ""}, {"eval", "Evaluate selected material", ""}, {"evaluations", "Evaluation datasets", "6"},
}

// Help documents commands independently of availability (busy/edit states).
var commandDescriptions = map[string]string{
	"behaviors":   "Create reusable behavior specs. Import a copy into a policy; detection rules stay with its behaviors and responses with the policy.",
	"policy":      "Configure Monitoring, Selection and Evals policies. Each policy has one judge and behavior specs. Monitoring checks during generation; Selection chooses alternatives; Evals assesses saved content.",
	"eval":        "Run /eval [policy] on selected document versions or checked conversations. In Evaluate, open New run setup with the focused dataset or selected items. Uses the active policy when no name is given. --train-on-pass true marks passing results for training; the policy default applies when omitted.",
	"evaluations": "Organize datasets in Data, open Evals policies in Policies, inspect Runs, and configure an evaluation with + New run. /export saves selected items or the opened collection with metadata.",
	"import":      "Add a local UTF-8 .txt or .md document to the shared Library. Title defaults to filename; author and source URL are optional.",
	"visitor":     "Write a visitor message in a new conversation fork. Open a conversation first.",
	"grid":        "Show concurrent Loom outputs. Arrows select a tile; Enter opens it. /grid returns.",
	"find":        "Filter documents or runs on the current page; type in the focused filter.",
	"active":      "Jump to the active generation while leaving background work running.",
	"rename":      "Name a document, conversation or Loom; clear the name to restore its automatic ancestry label.",
	"configure":   "Configure generation models, prompts, temperature, top-p and sampling; in Evaluate, configure the opened dataset. /policy owns monitoring, selection and Evals judges and behaviors. Loom flags override generation settings for one run.",
	"branch":      "Copy selected documents, conversations or sets without generation, preserving ancestry. Anthology opens the copies in Branches. Unavailable in Library and Evaluate. Save edits first.",
	"continue":    "Continue selected documents or conversations. Documents save a child revision; conversations advance in place with prior evidence retained. Anthology creates a new continuation in Branches without keeping it. Unavailable in Library and Evaluate. --loops repeats continuation; --tokens caps each completion. Simulator accepts --turns and --visitor. --model overrides this operation; --eval judges outputs. --monitoring on|off controls monitoring for this run; --selection off suppresses selection. Aliases: /generate, /run. Document judging is off unless --monitoring on or --eval is supplied; Simulator uses saved policy switches.",
	"loom":        "With a current target, /loom or /loom 1 continues it. A count of two or more forks the selected structure into alternatives. A set of four conversations with /loom 2 produces two sets of four. Anthology starts fresh Simulator conversations from selected kept documents. With selection Off, --loops splits once then continues every output. With selection On, each loop judges alternatives, chooses a winner and splits from that winner for the next loop. Library and Evaluate do not generate. --selection on|off and --monitoring on|off override policies for this run; selection needs two or more alternatives and explicit --loops N. Document judging is off unless --monitoring on, --selection on or --eval is supplied; Simulator uses saved policy switches.",
	"add":         "Library: import a source file. Branches: add selected versions to Anthology. Anthology: choose existing versions. Evaluate: add saved traces without judging.",
	"remove":      "Library: deselect sources. Anthology: unkeep versions. Branches: confirm deletion of versions and descendants. Evaluate: remove selected items, or confirm removal of the open dataset when no item is targeted; sources, policies and Runs remain. Alias: /delete.",
	"keep":        "Keep checked branches (or the highlighted branch) in Anthology. In Evaluate, mark selected items for training.",
	"models":      "Choose the Loom base model; on Simulator choose Character or Visitor. /model visitor jumps directly to that picker.",
	"workspaces":  "Open a saved workspace or create a new one.",
	"edit":        "Edit existing document text or a conversation message; saving preserves the original as a new version.",
	"inspect":     "Browse saved generation and policy evidence: Overview, Generation, Monitoring, Selection and Evaluations. Read summaries first; open individual requests, responses or Raw data for exact JSON and token events. Missing historical evidence is labeled as not recorded. Retry selection asks for confirmation, uses frozen candidates and policy, saves a new attempt, and generates no text.",
	"review":      "Attach a verdict and note to this document or a text range.",
	"snapshot":    "Export selected saved items, or the current collection when nothing is selected. Saves content, provenance and judgment metadata to workspace files. Does not judge or train.",
	"clear":       "Clear source/branch selection. In Simulator clear the conversation target so the next Loom starts fresh. Does not delete saved content.",
	"library":     "Browse seed documents and select passages.",
	"branches":    "Browse continuations; LEFT collapses, RIGHT expands, ENTER opens.",
	"kept":        "Browse the anthology of kept document versions.",
	"notes":       "Open this document’s notes. + adds a note; Enter reads and jumps to its anchor.",
	"simulator":   "Configure and inspect raw-document conversation runs.",
	"help":        "Show all commands and keyboard navigation.",
	"keys":        "Assign keys to actions and navigation; bindings apply across workspaces.",
	"cancel":      "Stop the active generation; preserve its partial output.",
	"save":        "Save the active document or policy edit (editing only).",
	"discard":     "Cancel the draft and return to document navigation; also ESC.",
	"restart":     "Restart Carla in this workspace; stop active generation and preserve partials.",
	"exit":        "Exit Carla; preserves unsaved drafts in recovery files.",
}

func commandName(a action) string {
	switch a.id {
	case "snapshot":
		return "export"
	case "evaluations":
		return "evaluate"
	case "configure":
		return "config"
	case "kept":
		return "anthology"
	case "models":
		return "model"
	case "workspaces":
		return "workspace"
	case "cancel":
		return "stop"
	case "discard":
		return "cancel"
	}
	return a.id
}
func (m *model) canonicalCommand(id string) string {
	switch id {
	case "generate", "run", "simulate":
		return "continue"
	case "grow":
		return "loom"
	case "export":
		return "snapshot"
	case "import":
		return "add"
	case "keep":
		if m.section == 4 {
			return "keep"
		}
		return "add"
	case "delete":
		return "remove"
	case "fork":
		return "branch"
	case "quit":
		return "exit"
	case "grow-config", "grow-policy", "spec", "prompt", "loom-policy":
		return "policy"
	case "settings", "config", "sim-config", "character-sampling", "visitor-sampling":
		return "configure"
	}
	return id
}
func (m *model) commandAliases(id string) []string {
	names := []string{commandName(action{id: id})}
	switch id {
	case "loom":
		names = append(names, "grow")
	case "continue":
		names = append(names, "generate", "run", "simulate")
	case "add":
		names = append(names, "import")
		if m.section != 4 {
			names = append(names, "keep")
		}
	case "snapshot":
		names = append(names, "snapshot")
	case "evaluations":
		names = append(names, "evaluations")
	case "branch":
		names = append(names, "fork")
	case "remove":
		names = append(names, "delete")
	case "configure":
		names = append(names, "configure", "settings", "sim-config", "character-sampling", "visitor-sampling")
	case "policy":
		names = append(names, "grow-config", "grow-policy", "spec", "prompt", "loom-policy", "loom-control-policy")
	case "exit":
		names = append(names, "quit")
	case "kept":
		names = append(names, "kept")
	}
	return names
}

// Optional direct entry opens the same control as keyboard navigation.
func (m *model) commandArguments(id string) []string {
	switch id {
	case "loom", "continue":
		args := []string{}
		if id == "loom" {
			noun := "[alternatives]"
			if (m.section == 3 && m.simSelection == nil) || m.section == 2 {
				noun = "[conversations]"
			}
			if m.section == 3 && len(m.selectedConversations()) > 1 {
				noun = "[alternative sets]"
			}
			args = append(args, noun)
		}
		args = append(args, "--tokens N|Max", "--model alias")
		if m.section == 3 || (m.section == 2 && id == "loom") {
			args = append(args, "--turns N", "--visitor \"text\"", "--visitor-model alias")
		}
		args = append(args, "--eval \"name\"")
		selection := "--selection off"
		if id == "loom" {
			selection = "--selection on|off"
		}
		args = append(args, selection, "--monitoring on|off", "--loops N")
		return args
	case "eval":
		return []string{"[\"policy name\"]", "--train-on-pass true|false"}
	case "models":
		if m.section == 3 {
			return []string{"[character|visitor]"}
		}
	case "configure":
		if m.section == 3 {
			return []string{"[setting]", "e.g. /config turns"}
		}
	}
	return nil
}

func (m *model) helpText(id string) string {
	if id == "navigation" {
		return fmt.Sprintf("FOCUS & SELECTION\nArrows browse and preview. %s checks an item; checked items define command scope. Selecting a set includes its members.\n\n%s opens a document or selection actions. %s unwinds one level. %s / %s move between panes.\n\nSIMULATOR\nSelect a Loom parent to act on all its conversations. Select children for a subset. /clear removes the target so the next /loom starts fresh.\n\nEDITING\n%s saves; %s cancels the draft. /config keys changes bindings.\n\nGeneration actions are available in Branches, Anthology and Simulator; Library and Evaluate do not generate.", m.keyLabel("select"), m.keyLabel("nav.enter"), m.keyLabel("nav.back"), m.keyLabel("nav.next"), m.keyLabel("nav.prev"), m.keyLabel("save"), m.keyLabel("nav.back"))
	}
	var text string
	if id == "loom" || id == "continue" {
		text = m.generationHelp(id)
	} else {
		text = commandDescriptions[id]
		if args := m.commandArguments(id); len(args) > 0 {
			text += "\n\nSYNTAX\n/" + commandName(action{id: id}) + " " + strings.Join(args, " ")
		}
		if id == "eval" {
			text += "\n\nEXAMPLES\n/eval\n/eval \"Voice\" --train-on-pass true\n\nTARGET\nBranches/Simulator: saved selected items. Evaluate: opens New run setup. No implicit default dataset.\n\n--train-on-pass true|false\nOverride the policy's training-mark default for this run. Every enabled behavior must complete and pass."
		}
		if id == "configure" {
			text += "\n\nDEFAULTS\nBranches: Tokens, model, sampling and context. Simulator: Turns, Character tokens, Visitor tokens, speaker models and prompts. Flags override one run; /config saves defaults.\n\n/config keys\n/config model\n/config workspace"
		}
	}
	names := m.commandAliases(id)
	if len(names) > 1 {
		text += "\n\nALIASES\n/" + strings.Join(names[1:], " · /")
	}
	return text
}
func (m *model) generationHelp(id string) string {
	text := "Advance selected items. Documents save a revision; conversations advance their saved histories.\n\n/continue [flags]"
	if id == "loom" {
		text = "Continue once, or split into alternative futures.\n\n/loom [N] [flags]\nWith a target: N = 1 continues; N = 2+ splits.\nNo Simulator target: start N fresh conversations."
	}
	text += "\n\nGENERATION FLAGS\n--tokens N|Max\n  Cap new tokens per completion; not context size.\n--loops N\n  Repeat the operation. Omitted: one pass.\n--model alias\n  Override the document or character model.\n\nSIMULATOR FLAGS\n--turns N\n  Additional character replies per loop.\n--visitor \"text\"\n  Supply the next visitor message once, to each target.\n--visitor-model alias\n  Override the visitor model.\n\nPOLICY FLAGS\n--monitoring on|off\n  Enable or bypass the configured monitor this run.\n--eval \"policy name\"\n  Assess completed outputs with a named Evals policy.\n"
	if id == "loom" {
		text += "--selection on|off\n  Choose among alternatives. On requires N >= 2 and an explicit --loops N. --loops 1 selects once.\n"
	} else {
		text += "--selection off\n  Bypass selection. Continue cannot enable it.\n"
	}
	text += "\nDEFAULTS & SCOPE\n/config saves tokens, turns and model defaults. Flags affect only this run. Documents default to judging Off; Simulator inherits saved policy switches. --tokens controls output; context includes input plus output.\n\nBranches: continue or split selected documents. Anthology: /continue opens a new Branches continuation; /loom starts Simulator from kept documents. New versions are not automatically kept.\n\nSimulator: select a parent for the whole set, or children for a subset. Arrows only preview. /clear makes the next Loom fresh. /continue requires a target. Library and Evaluate do not generate.\n\nEXAMPLES\n/continue --tokens 512\n/loom 4 --tokens 512\n/loom 2 --tokens 512 --turns 3 --loops 2\n/continue --visitor \"Why?\" --turns 1\n/loom 4 --selection on --loops 1\n/loom 2 --eval \"Voice\" --monitoring off\n\nThe --turns and --visitor examples require Simulator (or Anthology Loom). Here --turns 3 --loops 2 adds six character replies per conversation. With selection Off, loops advance every output; with it On, later loops split again from the winning alternative.\n\nSPELLINGS\nFlags accept --flag=value and any order; examples put --loops last. --msg and --message mean --visitor."
	if id == "loom" {
		text += " --count N and -n N mean the positional count.\n\nSET EXAMPLE\nFour selected conversations + /loom 2 creates two alternative sets of four. /continue advances the original four. One output continues; two or more split alternatives."
	}
	return text
}

// Full command help is a read-only child view; Enter never executes a command.

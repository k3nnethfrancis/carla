package main

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"sort"
	"strings"
)

// Commands reuse the action handlers; typing never invokes single-key shortcuts.
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
func (m *model) commandChoices() []action {
	value := strings.ToLower(strings.TrimSpace(m.command.Value()))
	if fields := strings.Fields(value); len(fields) > 1 {
		// Exact commands own their arguments; free-text intent searches keep
		// every word (for example /character sampling).
		for _, a := range allActions {
			for _, alias := range m.commandAliases(m.canonicalCommand(a.id)) {
				if fields[0] == "/"+alias {
					value = fields[0]
				}
			}
		}
	}
	if m.focus != 3 || !strings.HasPrefix(value, "/") {
		return nil
	}
	actions := m.contextualActions()
	actions = append(actions, action{id: "add", label: "Add to this collection"})
	for _, a := range allActions {
		if a.id == "eval" && len(m.evaluationTargets()) == 0 && !(m.section == 4 && len(m.collectionItems()) > 0) {
			continue
		}
		if m.section == 4 {
			canonical := m.canonicalCommand(a.id)
			switch canonical {
			case "loom", "branch", "models", "rename", "review", "clear", "grid", "import":
				continue
			}
			if a.id == "inspect" || a.id == "notes" || a.id == "keep" || a.id == "remove" {
				continue
			}
		}
		if (a.id == "run" || a.id == "character-sampling" || a.id == "visitor-sampling") && m.section != 3 {
			continue
		}
		if a.id == "delete" && m.targetRow().kind != "node" && !m.selectionVisible() {
			continue
		}
		if a.id == "active" && !m.data.Busy {
			continue
		}
		if a.id == "clear" && m.section == 3 {
			a.label = "Clear conversation selection · next Loom starts fresh"
		}
		if a.id == "clear" && m.selectionVisible() {
			a.label = "Clear branch selection"
		}
		_, conversation := m.conversationTarget()
		if m.section == 3 && a.id == "branch" {
			conversation = len(m.selectedConversations()) > 0
		}
		if a.id == "visitor" && m.section != 3 {
			continue
		}
		if documentAction(a.id) && !(a.id == "branch" && len(m.actionNodeIDs()) > 0) && !(conversation && (a.id == "branch" || a.id == "edit" || a.id == "inspect")) && m.targetRow().kind != "node" && !(a.id == "inspect" && m.section == 3 && m.simulation != nil) && !(a.id == "rename" && m.section == 3 && m.targetRow().kind != "sim-config" && m.targetRow().kind != "sim-run") {
			continue
		}
		if a.id != "generate" && a.id != "keep" && a.id != "add" && a.id != "remove" {
			actions = append(actions, a)
		}
	}
	if m.section == 4 && len(m.evaluationIDs()) > 0 {
		actions = append(actions, action{id: "keep", label: "Mark selected items for training"}, action{id: "remove", label: "Remove selected items from this evaluation"}, action{id: "notes", label: "Edit evaluation note"}, action{id: "inspect", label: "Exact evaluated input and judge result"})
	}
	actions = append(actions, action{id: "help", label: "All commands and navigation"}, action{id: "keys", label: "Edit keybindings"}, action{id: "quit", label: "Quit Carla"}, action{id: "exit", label: "Exit Carla"}, action{id: "restart", label: "Restart Carla in this workspace"})
	if m.data.Busy {
		available := []action{{id: "cancel", label: "Stop active operation"}}
		for _, a := range actions {
			if m.readOnlyAction(a.id) {
				available = append(available, a)
			}
		}
		actions = available
	}
	if m.editing != "" {
		actions = []action{{id: "notes", label: "Document notes"}, {id: "save", label: "Save edit"}, {id: "discard", label: "Discard edit"}, {id: "help", label: "All commands and navigation"}, {id: "keys", label: "Edit keybindings"}}
	}

	// Recovery actions must remain available even while editing or waiting.
	for _, recovery := range []action{{id: "restart", label: "Restart Carla"}, {id: "quit", label: "Quit Carla"}, {id: "exit", label: "Exit Carla"}} {
		found := false
		for _, a := range actions {
			if a.id == recovery.id {
				found = true
			}
		}
		if !found {
			actions = append(actions, recovery)
		}
	}
	// Collapse aliases before matching so one operation occupies one row.
	unique := []action{}
	seen := map[string]bool{}
	for _, a := range actions {
		a.id = m.canonicalCommand(a.id)
		if !m.paletteAvailable(a.id, value) {
			continue
		}
		if seen[a.id] {
			continue
		}
		seen[a.id] = true
		switch a.id {
		case "add":
			a.label = m.addDescription()
		case "continue":
			a.label = "Continue selected items · repeat with --loops"
			if m.section == 2 {
				a.label = "Continue a new branch from kept versions"
			}
		case "policy":
			a.label = "Monitoring, selection and evaluation judges"
		case "eval":
			a.label = "Run active or named evaluation on selected items"
		case "evaluations":
			a.label = "Named collections, judgments and training items"
		case "snapshot":
			a.label = "Export selected items or this collection with provenance"
		}
		if a.id == "models" && m.section == 3 {
			a.label = "Choose character or visitor model"
		}
		if a.id == "loom" {
			if m.section == 3 {
				a.label = "Start conversations · number = conversations"
				if m.simSelection != nil {
					a.label = m.simulationActionLabel()
				}
			} else if m.section == 1 {
				a.label = "Continue documents · 2+ splits alternatives"
			} else {
				a.label = "Start Simulator conversations from kept documents"
			}
		}
		if m.section == 3 && a.id == "branch" {
			a.label = "Fork selected conversation without generation"
		}
		if m.section == 3 && a.id == "edit" {
			a.label = "Edit a message in a new conversation fork"
		}
		if a.id == "configure" {
			a.label = "Generation models, prompts and sampling"
			if m.section == 4 {
				a.label = "Configure the opened evaluation and its judges"
			}
		}
		if a.id == "remove" && m.section == 1 {
			a.label = fmt.Sprintf("Remove %d versions and descendants…", m.collectionCount())
		}
		unique = append(unique, a)
	}
	actions = unique
	if !m.data.Busy && m.editing == "" {
		m.prioritizePageCommands(actions)
	}
	query := strings.TrimPrefix(value, "/")
	// An exact unavailable command must never execute a longer prefix match.
	for _, known := range allActions {
		canonical := m.canonicalCommand(known.id)
		for _, alias := range m.commandAliases(canonical) {
			if alias == query {
				available := false
				for _, a := range actions {
					available = available || a.id == canonical
				}
				if !available {
					return nil
				}
			}
		}
	}

	var matches []action
	score := func(a action) int {
		best := 99
		for _, name := range m.commandAliases(a.id) {
			if name == query {
				return 0
			}
			if strings.HasPrefix(name, query) {
				best = 1
			}
		}
		hay := strings.ToLower(commandName(a) + " " + a.label + " " + commandDescriptions[a.id])
		found := true
		for _, word := range strings.Fields(query) {
			if !strings.Contains(hay, word) {
				found = false
			}
		}
		if found && best > 2 {
			best = 2
		}
		return best
	}
	for _, a := range actions {
		if score(a) < 99 {
			matches = append(matches, a)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return score(matches[i]) < score(matches[j]) })
	if len(matches) > 0 && score(matches[0]) <= 1 {
		end := 0
		for end < len(matches) && score(matches[end]) <= 1 {
			end++
		}
		if strings.Contains(strings.TrimSpace(m.command.Value()), " ") && score(matches[0]) == 0 {
			return matches[:1]
		}
		return matches[:end]
	}
	// A known but unavailable command must not accidentally execute a keyword match.
	for _, a := range allActions {
		for _, name := range m.commandAliases(m.canonicalCommand(a.id)) {
			if name == query {
				return nil
			}
		}
	}
	if len(query) < 3 {
		return nil
	}
	return matches
}
func (m *model) suggestionCount() int {
	return min(4, max(1, m.height-14-len(m.commandHints())), len(m.commandChoices()))
}
func (m *model) focusCommand(slash bool) tea.Cmd {
	if m.focus == 1 && m.gridVisible() {
		for _, p := range m.layout().panels {
			if p.kind == 1 {
				m.gridSelection = m.gridSelected(p.box)
				m.gridPinned = true
			}
		}
	}
	if m.focus != 3 || m.dialog != nil {
		m.commandOrigin = &commandOrigin{m.focus, m.sectionFocus, m.searching, m.dialog}
	}
	m.dialog = nil
	m.searching = false
	m.rememberCommandDocument()
	m.focus = 3
	m.sectionFocus = false
	if slash {
		m.historyPosition = 0
		m.command.SetValue("/")
		m.command.CursorEnd()
		m.commandIndex = 0
	}
	m.reflow()
	return m.command.Focus()
}
func (m *model) commandKey(msg tea.KeyPressMsg) tea.Cmd {
	choices := m.commandChoices()
	switch m.navigationKey(msg.String()) {
	case "nav.next", "nav.prev":
		step := 1
		if m.navigationKey(msg.String()) == "nav.prev" {
			step = -1
		}
		return m.cycleFocus(step)
	case "nav.back":
		return m.dismissCommands()
	case "nav.up", "nav.down":
		if m.historyPosition > 0 || strings.TrimSpace(m.command.Value()) == "" || (len(strings.Fields(m.command.Value())) > 1 && len(m.commandHints()) > 0) {
			step := 1
			if m.navigationKey(msg.String()) == "nav.down" {
				step = -1
			}
			m.browseCommandHistory(step)
			return nil
		}
		if len(choices) > 0 {
			delta := 1
			if m.navigationKey(msg.String()) == "nav.up" {
				delta = -1
			}
			m.commandIndex = (m.commandIndex + delta + len(choices)) % len(choices)
			m.reflow()
		}
		return nil
	case "nav.enter":
		if len(choices) == 0 {
			m.status = "No matching action · type / to browse"
			if m.data.Busy {
				m.status = "This action is unavailable during generation · /active · /stop"
			}
			if m.editing != "" {
				m.status = "Save or cancel the edit before changing documents or running generation"
			}
			return nil
		}
		a := choices[min(m.commandIndex, len(choices)-1)]
		if a.id == "restart" || a.id == "quit" || a.id == "exit" {
			m.recordCommand(a)
			return m.exitSession(a.id == "restart")
		}
		if m.pending {
			m.status = "Waiting for the current request…"
			return nil
		}

		rawInput := m.command.Value()
		var options generationOptions
		var err error
		if a.id == "continue" || a.id == "loom" || a.id == "branch" {
			options, err = parseGenerationOptions(rawInput, a.id)
		}
		if err != nil {
			m.status = "Error: " + err.Error()
			return nil
		}
		m.recordCommand(a)
		m.command.SetValue("")
		m.commandIndex = 0
		m.reflow()
		switch a.id {
		case "eval":
			return m.openEval(rawInput)
		case "branch":
			return m.forkDocument()
		case "loom", "continue":
			options.Action = a.id
			return m.loom(options)
		case "save":
			return m.saveEditor()
		case "discard":
			return m.cancelEdit()
		}
		if cmd, handled := m.directConfig(a.id, rawInput); handled {
			return cmd
		}
		return m.perform(a.id)
	}
	m.historyPosition = 0
	var cmd tea.Cmd
	m.command, cmd = m.command.Update(msg)
	m.commandIndex = 0
	m.reflow()
	return cmd
}
func (m *model) commandView() string {
	border := lipgloss.NormalBorder()
	if m.focus == 3 {
		border = lipgloss.ThickBorder()
	}
	input := lipgloss.NewStyle().Border(border).Padding(0, 1).Render(line(m.command.View(), m.width-6))
	lines := []string{lipgloss.NewStyle().PaddingLeft(1).Render(input)}
	choices := m.commandChoices()
	start := max(0, m.commandIndex-m.suggestionCount()+1)
	for i := start; i < min(len(choices), start+m.suggestionCount()); i++ {
		a := choices[i]
		name := "/" + commandName(a)
		marker := "  "
		nameStyle, descriptionStyle := dim, dim
		if i == m.commandIndex {
			marker = "› "
			nameStyle, descriptionStyle = bold, m.helpStyle()
		}
		// Keep the name and description distinct without a full-width inverse bar.
		nameWidth := min(lipgloss.Width(name), max(1, m.width-8))
		label := marker + nameStyle.Render(line(name, nameWidth)) + "  " +
			descriptionStyle.Render(line(a.label, max(0, m.width-8-nameWidth)))
		lines = append(lines, " "+label)
	}
	for _, hint := range m.commandHints() {
		lines = append(lines, "  "+dim.Render(line(hint, m.width-4)))
	}
	return strings.Join(lines, "\n")
}

// Tab and Shift+Tab traverse the same inner ring in opposite directions.
func (m *model) cycleFocus(step int) tea.Cmd {
	m.rememberCommandDocument()
	m.command.Blur()
	m.sectionFocus = false
	if m.editing != "" {
		m.focus = 1
		m.reflow()
		return m.editor.Focus()
	}
	previousFocus := m.focus
	m.focus = (m.focus + step + 4) % 4
	if m.adaptiveBranches() && len(m.layout().panels) == 1 {
		for m.focus == 1 || m.focus == 2 {
			m.focus = (m.focus + step + 4) % 4
		}
	}
	if m.focus == 2 && !m.showInspector {
		m.focus = (m.focus + step + 4) % 4
	}
	m.reflow()
	if m.cursorActive() {
		m.revealCursor()
	}
	if m.focus == 3 {
		m.commandOrigin = &commandOrigin{focus: previousFocus}
		return m.command.Focus()
	}
	return nil
}
func (m *model) outerFocus() {
	m.command.Blur()
	m.focus = 0
	m.sectionFocus = true
	m.reflow()
}

func (m *model) rememberCommandDocument() {
	if m.focus == 3 {
		return
	}
	m.commandDocument = ""
	if m.focus == 1 && (m.section == 1 || m.section == 2) && m.data.Current != nil {
		m.commandDocument = m.currentID()
	}
}

// Reorder available actions rather than maintaining a second command registry.
// Global commands stay searchable; editing and busy states keep their own menus.
func (m *model) prioritizePageCommands(actions []action) {
	var preferred []string
	switch m.section {
	case 0:
		preferred = []string{"add", "remove", "clear", "configure", "branches"}
	case 1:
		preferred = []string{"continue", "loom", "configure", "branch", "add", "remove", "clear", "edit", "notes", "inspect", "review", "models", "settings"}
	case 2:
		preferred = []string{"snapshot", "remove", "inspect", "notes", "edit", "simulator", "continue", "loom", "branch"}
	case 4:
		preferred = []string{"eval", "configure", "keep", "remove", "notes", "inspect", "snapshot", "policy", "find"}
	case 3:
		preferred = []string{"continue", "loom", "configure", "clear", "branch", "edit", "visitor", "inspect", "anthology"}
	}
	if m.notesOpen {
		preferred = append([]string{"notes", "edit", "inspect"}, preferred...)
	}
	rank := func(id string) int {
		if id == "kept" {
			id = "anthology"
		}
		for i, wanted := range preferred {
			if id == wanted {
				return i
			}
		}
		return len(preferred)
	}
	sort.SliceStable(actions, func(i, j int) bool { return rank(actions[i].id) < rank(actions[j].id) })
}

// Internal action IDs remain stable for saved keyboard bindings. Canonical names
// and aliases share a single palette row and dispatch path.
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
func (m *model) directConfig(id, input string) (tea.Cmd, bool) {
	fields := strings.Fields(input)
	if len(fields) == 2 && id == "configure" {
		for _, target := range []string{"keys", "workspaces", "workspace", "models", "model"} {
			if fields[1] == target {
				if target == "workspace" {
					target = "workspaces"
				}
				if target == "model" {
					target = "models"
				}
				m.openConfig()
				parent := m.dialog
				cmd := m.perform(target)
				if m.dialog != nil {
					m.dialog.parent = parent
				}
				return cmd, true
			}
		}
	}
	if len(fields) != 2 || m.section != 3 {
		return nil, false
	}
	arg := strings.ToLower(fields[1])
	if id == "models" && (arg == "character" || arg == "visitor") {
		m.speakerPicker()
		d := m.dialog
		m.dialog = nil
		return m.configureChoice(d, row{id: arg + "_alias", label: arg + " model"}), true
	}
	if id == "configure" {
		m.openSimulatorConfig()
		d := m.dialog
		for i, r := range d.rows {
			if r.id == arg {
				d.index = i
				m.dialog = nil
				return m.configureChoice(d, r), true
			}
		}
		m.status = "Choose a setting from configuration"
		return nil, true
	}
	return nil, false
}

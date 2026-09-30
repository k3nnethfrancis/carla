package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"
	"time"
)

type action struct{ id, label, key string }

var allActions = []action{
	{"continue", "Continue", "ctrl+r"}, {"generate", "Generate (alias for continue)", ""}, {"branch", "Branch current version", "ctrl+b"}, {"loom", "Generate alternatives", ""},
	{"add", "Add to seeds", ""}, {"remove", "Remove from collection", ""},
	{"delete", "Delete selected branches", ""}, {"keep", "Keep branch", "k"}, {"grow", "Grow", "g"},
	{"settings", "Settings", "ctrl+t"}, {"models", "Model", "m"},
	{"workspaces", "Workspace", "ctrl+w"}, {"edit", "Edit document", "e"},
	{"inspect", "Exact input", "ctrl+e"}, {"review", "Review document", "ctrl+u"},
	{"spec", "Selection spec", "s"}, {"prompt", "Selector prompt", "p"},
	{"snapshot", "Export kept documents", "ctrl+s"}, {"clear", "Clear seed selection", "x"},
	{"library", "Library", "1"}, {"branches", "Branches", "2"},
	{"kept", "Anthology", "3"}, {"notes", "Notes", "4"}, {"simulator", "Simulator", "5"}, {"sim-config", "Configure simulator", ""}, {"simulate", "Run conversations", ""}, {"grow-config", "Configure Grow", ""}, {"grow-policy", "Grow selection criteria", ""},
	{"run", "Run Simulator conversations", ""}, {"configure", "Configure this page", ""},
	{"find", "Filter this list", "ctrl+f"}, {"active", "View active generation", ""}, {"rename", "Rename document", ""},
	{"visitor", "Write a visitor message in a conversation fork", ""}, {"grid", "Show Loom grid", ""}, {"loom-policy", "Configure conversation warnings and stop rules", ""},
	{"character-sampling", "Character temperature, top-p and output tokens", ""}, {"visitor-sampling", "Visitor temperature, top-p and output tokens", ""},
	{"import", "Import a document into Library", ""},
	{"policy", "Monitoring, selection and evaluation criteria", ""}, {"eval", "Evaluate selected material", ""}, {"evaluations", "Evaluation datasets", "6"},
}

func (m *model) perform(id string) tea.Cmd {
	if id == "remove.alt" {
		id = "remove"
	}
	if (m.section == 0 || m.section == 4) && (id == "branch" || id == "continue" || id == "loom" || id == "generate" || id == "grow") {
		m.status = "Open material in Branches or Simulator to generate"
		return nil
	}
	if m.inNotesContext() {
		switch id {
		case "remove", "delete", "branch", "keep", "eval", "snapshot", "continue", "loom", "grow", "generate":
			m.status = "This action applies to documents; focus the document or return to Branches"
			return nil
		}
	}
	if id == "grow" {
		return m.loom(generationOptions{Action: "loom"})
	}
	if id == "add" {
		return m.addItem()
	}
	if id == "snapshot" {
		return m.exportItems()
	}
	if id == "policy" {
		return m.openPolicy()
	}
	if id == "eval" {
		return m.openEval("/eval")
	}
	if m.section == 4 && (id == "keep" || id == "remove" || id == "delete" || id == "notes" || id == "inspect" || id == "snapshot") {
		if id == "delete" {
			id = "remove"
		}
		return m.evalAction(id)
	}
	// Legacy keybindings enter the same generation operation as the command bar.
	if id == "continue" || id == "generate" || id == "run" || id == "simulate" || id == "grow" {
		return m.loom(generationOptions{Action: "continue"})
	}
	if id == "remove" && m.section == 1 {
		id = "delete"
	}
	if id == "configure" {
		return m.openConfig()
	}

	if m.section == 3 && id == "edit" {
		return m.editConversation(false)
	}
	if id == "visitor" {
		return m.editConversation(true)
	}
	if id == "loom-policy" {
		return m.openLoomPolicy()
	}
	if id == "grid" {
		m.conversationOpen = false
		m.loomGrid = true
		m.gridPinned = false
		m.focus = 1
		if !m.gridVisible() {
			m.status = "Run a Loom with multiple outputs to show the grid"
		}
		return nil
	}
	if id == "restart" {
		return m.exitSession(true)
	}

	if id == "generate" {
		id = "continue"
	}
	if id == "commands.alt" {
		id = "commands"
	}
	if id == "keep.alt" {
		id = "keep"
	}
	if id == "help" {
		return m.openHelp()
	}
	if id == "keys" {
		return m.openKeys()
	}
	if id == "cancel" {
		return m.send("cancel", nil)
	}
	if m.data.Busy && !m.readOnlyAction(id) {
		m.status = "Operation running · /stop cancels it"
		return nil
	}
	if id == "inspect" && m.section == 3 {
		if m.simulation == nil {
			m.status = "Open a simulation run first"
			return nil
		}
		return m.send("simulator.inspect", map[string]any{"run": m.simulation.ID})
	}
	if documentAction(id) && !(id == "branch" && len(m.actionNodeIDs()) > 0) && !(m.section == 3 && id == "branch") && (m.targetRow().kind != "node" || m.targetRow().id != m.currentID() || m.pending) {
		m.status = "Select a branch and wait for its preview before /" + id
		return m.previewTarget()
	}
	switch id {
	case "commands":
		return m.focusCommand(true)
	case "find":
		m.notesOpen = false
		m.focus = 0
		m.searching = true
		m.reflow()
		return m.search.Focus()
	case "delete":
		if m.selectionVisible() || m.targetRow().kind == "node" {
			return m.selectionAction("delete")
		}
		m.status = "Select branches first"
		return nil
	case "select":
		return m.toggleTarget()
	case "open":
		return m.activate()
	case "keep", "add", "remove":
		return m.contextualAction(id)
	case "grow-config":
		return m.openGrowConfig()
	case "grow-policy":
		return m.beginEdit("policy_spec")
	case "sim-config":
		return m.openSimulatorConfig()
	case "branch":
		return m.forkDocument()
	case "loom":
		return m.loom(generationOptions{})
	case "clear":
		if m.section == 3 {
			m.simSelection = nil
			m.reflow()
			return nil
		}
		if m.selectionVisible() {
			return m.selectionAction("clear")
		}
		return m.send("seed.clear", nil)
	case "snapshot":
		return m.send("snapshot", nil)
	case "inspect":
		return m.send("inspect", nil)
	case "edit":
		return m.beginEdit("document")
	case "spec":
		return m.beginEdit("policy_spec")
	case "prompt":
		return m.beginEdit("policy_prompt")
	case "import":
		return m.openDialog("import")
	case "models":
		if m.section == 3 {
			return m.speakerPicker()
		}
		return m.openDialog("models")
	case "settings":
		if m.section == 3 {
			return m.openSimulatorConfig()
		}
		return m.openDialog("settings")
	case "active":
		return m.openActive()
	case "rename":
		d := &dialog{kind: "rename", title: "Document title", args: map[string]any{"node": m.currentID()}}
		d.add("Title", documentLabel(*m.data.Current))
		m.dialog = d
		return d.fields[0].input.Focus()
	case "character-sampling", "visitor-sampling":
		return m.openSampling(strings.TrimSuffix(id, "-sampling") + "_settings")
	case "workspaces", "review":
		return m.openDialog(id)
	case "notes":
		return m.openNotes(true)
	case "library", "branches", "kept", "simulator", "evaluations":
		modes := map[string]int{"library": 0, "branches": 1, "kept": 2, "simulator": 3, "evaluations": 4}
		return m.switchSection(modes[id])
	}
	return nil
}

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	before, workspace := m.workspaceView(), m.data.Workspace.Path
	defer func() {
		if workspace == m.data.Workspace.Path && before != m.workspaceView() {
			m.saveWorkspaceView()
		}
	}()
	switch msg := message.(type) {
	case policyTick:
		return m, m.advancePolicyPulse(time.Time(msg))
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.reflow()
		return m, nil
	case event:
		cmd := m.apply(msg)
		if msg.Type == "bye" {
			return m, cmd
		}
		reset := m.flushSeedReset()
		return m, tea.Batch(cmd, reset, m.previewTarget(), m.client.read())
	case failure:
		m.restoringView = false
		m.editRequest = ""
		m.pending = false
		m.disconnected = true
		if !strings.HasPrefix(m.status, "Error:") {
			m.status = "Disconnected: " + msg.err.Error()
		}
		m.status += " · /restart or CTRL+Q"
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.reflow()
		return m, nil
	case tea.KeyPressMsg:
		raw := msg.String()
		if m.editRequest != "" && raw != "ctrl+c" && m.boundAction(raw, "global") != "quit" {
			m.status = "Saving edit…"
			return m, nil
		}
		key := m.navigationKey(raw)
		// File/URL setup forms need literal slashes.
		literalSlash := m.dialog != nil && (strings.HasPrefix(m.dialog.kind, "setup-") || m.dialog.kind == "import")
		if raw == "/" && !literalSlash && (m.focus != 3 || m.dialog != nil || m.searching || m.sectionFocus) && !m.keyCapture {
			m.editor.Blur()
			m.search.Blur()
			return m, m.focusCommand(true)
		}
		if raw == "ctrl+c" || (!m.keyCapture && m.boundAction(raw, "global") == "quit") {
			return m, m.exitSession(false)
		}
		if m.dialog != nil && m.dialog.kind == "keys" {
			return m, m.keysKey(msg)
		}

		if m.width < 60 || m.height < 18 {
			if key == "nav.back" && m.editing != "" {
				m.editing = ""
				m.editor.Blur()
			}
			return m, nil
		}
		if m.dialog != nil {
			return m, m.dialogKey(msg)
		}
		if key == "nav.back" && m.section == 4 && m.evalCollection != "" && m.focus == 0 && !m.sectionFocus {
			m.enterCollection("")
			return m, nil
		}
		if m.sectionFocus {
			if m.boundAction(raw, "panels") == "commands" {
				return m, m.focusCommand(true)
			}
			switch key {
			case "nav.left", "nav.right":
				step := 1
				if key == "nav.left" {
					step = -1
				}
				return m, m.switchSection((m.section + step + len(sectionNames)) % len(sectionNames))
			case "nav.enter", "nav.down", "nav.next":
				m.sectionFocus = false
				m.focus = 0
			case "nav.prev":
				m.sectionFocus = false
				m.focus = 0
				return m, m.cycleFocus(-1)
			case "nav.back":
				return m, nil

			}
			m.reflow()
			return m, m.previewTarget()
		}

		if m.editing != "" && m.saveKey(msg) {
			return m, m.saveEditor()
		}
		if m.focus == 3 {
			if m.editing == "" && strings.Contains(raw, "+") {
				if action := m.boundAction(raw, "panels"); action != "" {
					return m, m.perform(action)
				}
			}
			return m, m.commandKey(msg)
		}
		if m.notesOpen && m.focus == 0 {
			return m, m.notesKey(msg)
		}
		if cmd, handled := m.gridKey(key); handled {
			return m, cmd
		}
		if m.editing == "" && !m.searching && (m.boundAction(raw, "panels") == "remove" || m.boundAction(raw, "panels") == "remove.alt") {
			return m, m.perform("remove")
		}
		if m.cursorActive() && documentInput(msg) {
			if msg.Code == tea.KeyEnter {
				return m, m.documentActions()
			}
			m.status = "Press ENTER and choose Edit here, or use /edit"
			return m, nil
		}

		if m.editing != "" {
			if action := m.boundAction(raw, "editor"); action != "" {
				key = action
			}
			switch key {
			case "nav.next", "nav.prev":
				m.editor.Blur()
				return m, m.focusCommand(false)
			case "nav.back", "discard":
				return m, m.cancelEdit()
			case "save":
				return m, m.saveEditor()
			case "edit.review":
				m.status = "Save or cancel this edit before reviewing"
				return m, nil
			case "edit.branch":
				if m.editing == "document" {
					m.status = "Save or cancel this edit before branching"
					return m, nil
				}
			}
			var cmd tea.Cmd
			m.editor, cmd = m.editor.Update(msg)
			return m, cmd
		}
		if m.searching {
			if key == "nav.back" || key == "nav.enter" {
				m.searching = false
				m.search.Blur()
				return m, m.previewTarget()
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.filter = m.search.Value()
			m.selected = 0
			m.reflow()
			return m, cmd
		}
		if action := m.boundAction(raw, "panels"); action != "" {
			return m, m.perform(action)
		}
		switch key {
		case "nav.back":
			if m.simulatorBack() {
				return m, nil
			}
			m.outerFocus()
			return m, nil
		case "nav.next", "nav.prev":
			step := 1
			if key == "nav.prev" {
				step = -1
			}
			return m, m.cycleFocus(step)
		case "nav.enter":
			if m.focus == 0 {
				return m, m.activate()
			}
		case "nav.left", "nav.right":
			if m.focus == 0 && m.section == 3 {
				m.conversationArrow(strings.TrimPrefix(key, "nav."))
				return m, nil
			}
			if m.focus == 0 && m.section == 1 {
				m.branchArrow(strings.TrimPrefix(key, "nav."))
				m.reflow()
				return m, m.previewTarget()
			}
			if m.focus == 0 && m.section == 0 {
				rows := m.rows()
				if len(rows) > 0 {
					r := rows[min(m.selected, len(rows)-1)]
					if r.kind == "source" {
						if key == "nav.right" && m.expanded[r.id] {
							m.selected = min(m.selected+1, len(rows)-1)
						} else {
							m.expanded[r.id] = key == "nav.right"
						}
					} else if key == "nav.left" {
						parent := strings.SplitN(r.id, ":", 2)[0]
						for i, row := range rows {
							if row.id == parent {
								m.selected = i
								break
							}
						}
					}
					m.reflow()
				}
				return m, nil
			}
		case "nav.up", "nav.down":
			if m.focus == 0 {
				step := 1
				if key == "nav.up" {
					step = -1
				}
				m.selected = max(0, min(m.selected+step, len(m.rows())-1))
				m.reflow()
				return m, m.previewTarget()
			}
		}
		if m.cursorActive() {
			return m, m.moveCursor(key, msg)
		}
		var cmd tea.Cmd
		if m.focus == 1 {
			m.document, cmd = m.document.Update(navigationMessage(key, msg))
		} else if m.focus == 2 {
			m.inspector, cmd = m.inspector.Update(navigationMessage(key, msg))
		}
		return m, cmd
	case tea.PasteMsg:
		if m.editRequest != "" {
			return m, nil
		}
		if m.focus == 3 {
			m.historyPosition = 0
		}
		if m.dialog == nil && m.cursorActive() {
			m.status = "Use /edit before pasting"
			return m, nil
		}
	case tea.MouseClickMsg:
		if m.editRequest != "" {
			return m, nil
		}
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		if m.dialog != nil {
			return m, m.dialogClick(msg.X, msg.Y)
		}
		if m.gridVisible() {
			for _, p := range m.layout().panels {
				if p.kind == 1 {
					_, _, size := m.gridGeometry(p.box)
					for i, r := range m.gridRects(p.box) {
						if r.contains(msg.X, msg.Y) {
							m.focus = 1
							return m, m.openGridTile(m.gridPage(p.box)*size + i)
						}
					}
				}
			}
		}
		choices := m.commandChoices()
		first := m.layout().actionY + 3
		if msg.Y >= first && msg.Y < first+m.suggestionCount() && msg.X >= 1 && msg.X < m.width-1 {
			m.commandIndex = max(0, m.commandIndex-m.suggestionCount()+1) + msg.Y - first
			if m.commandIndex < len(choices) {
				return m, m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			}
		}
		if msg.Y >= m.layout().actionY && msg.Y < m.layout().actionY+3 {
			return m, m.focusCommand(false)
		}
		if m.notesOpen {
			for _, panel := range m.layout().panels {
				if panel.kind == 0 && panel.box.contains(msg.X, msg.Y) {
					return m, m.notesClick(msg.Y-panel.box.y-3, panel.box)
				}
				if panel.kind == 1 && panel.box.contains(msg.X, msg.Y) {
					m.focus = 1
					m.reflow()
					if m.editing == "document" {
						return m, m.editor.Focus()
					}
					return m, nil
				}
			}
		}
		if m.editing != "" {
			return m, nil
		}
		if msg.Y == 2 {
			for i, r := range m.sectionRects() {
				if r.contains(msg.X, msg.Y) {
					m.sectionFocus = false
					return m, m.switchSection(i)
				}
			}
		}
		if msg.Y == 1 {
			if msg.X >= m.width/2 {
				return m, m.openDialog("models")
			}
			return m, m.openDialog("workspaces")
		}
		layout := m.layout()
		for _, panel := range layout.panels {
			if panel.box.contains(msg.X, msg.Y) {
				m.focus = panel.kind
				if panel.kind == 0 {
					if m.section == 0 && msg.Y == panel.box.y+panel.box.h-3 {
						return m, m.perform("clear")
					}
					index := msg.Y - panel.box.y - 3 + m.navStart(m.navigationRows(panel.box))
					if index >= 0 && index < len(m.rows()) {
						m.selected = index
						r := m.rows()[index]
						if m.section == 1 || m.section == 2 {
							checkX := panel.box.x + 2 + len([]rune(m.treeIndent(r.depth, panel.box)))
							if m.section == 1 {
								checkX += 2
							}
							if msg.X >= checkX && msg.X < checkX+2 {
								return m, m.toggleTarget()
							}
						}
						if m.section == 3 && (strings.HasPrefix(r.label, "▾") || strings.HasPrefix(r.label, "▸")) && msg.X == panel.box.x+2+len([]rune(m.treeIndent(r.depth, panel.box))) {
							m.collapsed[r.id] = !m.collapsed[r.id]
							return m, nil
						}
						if m.section == 1 && m.hasChildren(r.id) && msg.X == panel.box.x+2+len([]rune(m.treeIndent(r.depth, panel.box))) {
							m.collapsed[r.id] = !m.collapsed[r.id]
							m.reflow()
							return m, nil
						}
						m.reflow()
						return m, m.previewTarget()
					}
				}
				m.reflow()
				return m, nil
			}
		}
	case tea.MouseWheelMsg:
		if m.dialog != nil {
			delta := 3
			if msg.Button == tea.MouseWheelUp {
				delta = -3
			}
			if m.dialog.kind == "help-detail" {
				m.scrollHelp(delta)
			}
			if m.dialog.kind == "help" {
				m.dialog.index = max(0, min(m.dialog.index+delta, len(m.dialog.rows)-1))
			}
			// Modal input must never move the underlying document or selection.
			return m, nil
		}
		if m.editing != "" && m.focus == 1 && m.dialog == nil && m.editRequest == "" {
			code := tea.KeyDown
			if msg.Button == tea.MouseWheelUp {
				code = tea.KeyUp
			}
			var cmds []tea.Cmd
			for i := 0; i < 3; i++ {
				var cmd tea.Cmd
				m.editor, cmd = m.editor.Update(tea.KeyPressMsg{Code: code})
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if m.editRequest != "" {
			return m, nil
		}
		var cmd tea.Cmd
		if m.focus == 1 {
			m.document, cmd = m.document.Update(msg)
		} else if m.focus == 2 {
			m.inspector, cmd = m.inspector.Update(msg)
		} else if m.focus == 0 {
			delta := 3
			if msg.Button == tea.MouseWheelUp {
				delta = -3
			}
			m.selected = max(0, min(m.selected+delta, len(m.rows())-1))
		}
		return m, cmd
	}
	if m.focus == 3 && m.dialog == nil {
		var cmd tea.Cmd
		m.command, cmd = m.command.Update(message)
		m.commandIndex = 0
		m.reflow()
		return m, cmd
	}
	if m.editing != "" && m.dialog == nil {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(message)
		return m, cmd
	}
	if m.dialog != nil && len(m.dialog.fields) > 0 {
		d := m.dialog
		var cmd tea.Cmd
		d.fields[d.field].input, cmd = d.fields[d.field].input.Update(message)
		return m, cmd
	}
	return m, nil
}

func (m *model) dialogClick(x, y int) tea.Cmd {
	d := m.dialog
	box := m.dialogRect()
	if len(d.rows) > 0 {
		index := y - box.y - 3 + m.dialogStart(m.dialogVisibleRows())
		if x > box.x && x < box.x+box.w-1 && index >= 0 && index < len(d.rows) {
			d.index = index
			if d.kind == "sim-documents" {
				return m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace})
			}
			if d.kind == "keys" {
				m.keyCapture = true
				m.refreshKeys()
				return nil
			}
			return m.submitDialog()
		}
	} else {
		visible := max(1, (box.h-6)/3)
		start := max(0, d.field-visible+1)
		index := (y-box.y-3)/3 + start
		if y >= box.y+3 && index >= 0 && index < len(d.fields) {
			d.fields[d.field].input.Blur()
			d.field = index
			return d.fields[index].input.Focus()
		}
	}
	return nil
}

func (m *model) nodeTitle() string {
	if m.editing == "evaluation-new-spec" {
		return "Evaluation criteria"
	}
	if strings.HasPrefix(m.editing, "evaluation-") {
		return strings.ReplaceAll(m.editing, "-", " ")
	}
	if m.section == 4 {
		return "Evaluate"
	}
	switch m.editing {
	case "conversation":
		if m.conversationEdit < 0 {
			return "New visitor message · fork"
		}
		return fmt.Sprintf("Edit message %d · fork", m.conversationEdit+1)
	case "character_template":
		return "Character prompt"
	case "visitor_template":
		return "Visitor prompt"
	case "opening_prompt":
		return "Opening generation prompt"
	case "visitor_brief":
		return "Visitor brief"
	}
	if m.editing == "monitor_spec" {
		if name := m.dimension(m.behaviorEditID).Name; name != "" {
			return "Behavior spec · " + name
		}
		return "Behavior spec"
	}
	if m.editing == "policy_spec" {
		return "Selection spec"
	}
	if m.editing == "policy_prompt" {
		return "Selector prompt"
	}
	if m.section == 0 && m.editing == "" {
		return "Source preview"
	}
	if m.data.Current == nil {
		return "Document"
	}
	n := m.data.Current
	title := n.Title
	if title == "" {
		title = fmt.Sprintf("%s · %s", n.Kind, n.ID[:min(6, len(n.ID))])
	}
	if n.Kept {
		title = "★ " + title
	}
	title = documentStatusLabel(*n, title)
	if n.Model != "" {
		title += " · " + n.Model
	}
	return title
}

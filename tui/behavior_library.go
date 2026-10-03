package main

import (
	tea "charm.land/bubbletea/v2"
	"strings"
)

// Library definitions are reusable criteria. Policies import a versioned copy,
// keeping model, enabled state, thresholds and actions local to their use.
type libraryBehavior struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Spec     string `json:"spec"`
	Revision int    `json:"revision"`
}

func (m *model) libraryBehavior(id string) *libraryBehavior {
	for i := range m.data.BehaviorLibrary {
		if m.data.BehaviorLibrary[i].ID == id {
			return &m.data.BehaviorLibrary[i]
		}
	}
	return nil
}
func (m *model) openBehaviorLibrary() tea.Cmd {
	d := &dialog{kind: "behavior-library", title: "Behaviors"}
	for _, b := range m.data.BehaviorLibrary {
		d.rows = append(d.rows, row{id: b.ID, label: b.Name, preview: b.Spec})
	}
	d.rows = append(d.rows, row{id: "new", label: "+ New behavior", preview: "Write a reusable spec. Add copies to policies in Monitoring, Selection or Evals."})
	m.dialog = d
	return nil
}
func (m *model) openBehaviorLibraryPicker(parent *dialog, kind string, args map[string]any) tea.Cmd {
	d := &dialog{kind: kind, title: "From library", parent: parent, args: args}
	for _, b := range m.data.BehaviorLibrary {
		d.rows = append(d.rows, row{id: b.ID, label: b.Name, preview: libraryImportHelp(kind) + b.Spec})
	}
	if len(d.rows) == 0 {
		m.status = "No saved specs. Create one with /behaviors first"
		return nil
	}
	m.dialog = d
	return nil
}
func (m *model) openLibraryBehavior(id string) tea.Cmd {
	b := m.libraryBehavior(id)
	if b == nil {
		return nil
	}
	m.dialog = &dialog{kind: "behavior-library-item", title: b.Name, args: map[string]any{"id": id}, rows: []row{{id: "name", label: "Name · " + b.Name, preview: "Short name used when choosing this reusable spec."}, {id: "spec", label: "Behavior spec", preview: "Editing updates future imports. Existing policy copies retain their saved spec. " + b.Spec}, {id: "delete", label: "Remove from library…", preview: "Remove this reusable definition. Existing policy copies and results remain."}}}
	return nil
}
func (m *model) submitBehaviorLibrary(d *dialog) tea.Cmd {
	if strings.HasPrefix(d.kind, "behavior-library-sync") {
		return m.submitLibrarySync(d)
	}
	id, _ := d.args["id"].(string)
	b := m.libraryBehavior(id)
	if d.kind == "behavior-library-new" || d.kind == "behavior-library-name" {
		name := strings.TrimSpace(d.fields[0].input.Value())
		if name == "" {
			m.status = "Name the behavior"
			return nil
		}
		if b != nil {
			return m.saveDialog(d, "behavior.save", map[string]any{"id": b.ID, "name": name, "spec": b.Spec})
		}
		m.editing = "library-spec"
		m.editReturn = d
		m.dialog = nil
		m.editor.SetValue("")
		m.focus = 1
		m.reflow()
		return m.editor.Focus()
	}
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	if d.kind == "behavior-library" {
		if r.id == "new" {
			m.dialog = &dialog{kind: "behavior-library-new", title: "New behavior", parent: d}
			m.dialog.add("Name", "")
			return m.dialog.fields[0].input.Focus()
		}
		cmd := m.openLibraryBehavior(r.id)
		m.dialog.parent = d
		return cmd
	}
	if b == nil {
		return nil
	}
	if d.kind == "behavior-library-delete" {
		if r.id == "cancel" {
			m.dialog = d.parent
			return nil
		}
		m.dialog = d.parent.parent
		return m.send("behavior.delete", map[string]any{"id": id})
	}
	switch r.id {
	case "name":
		m.dialog = &dialog{kind: "behavior-library-name", title: "Behavior name", parent: d, args: d.args}
		m.dialog.add("Name", b.Name)
		return m.dialog.fields[0].input.Focus()
	case "spec":
		m.editing = "library-spec"
		m.editReturn = d
		m.dialog = nil
		m.editor.SetValue(b.Spec)
		m.focus = 1
		m.reflow()
		return m.editor.Focus()
	case "delete":
		m.dialog = &dialog{kind: "behavior-library-delete", title: "Remove library spec? Policy copies remain", parent: d, args: d.args, rows: []row{{id: "cancel", label: "Cancel"}, {id: "confirm", label: "Remove"}}}
	}
	return nil
}
func (m *model) saveLibraryEditor(text string) tea.Cmd {
	if strings.TrimSpace(text) == "" {
		m.status = "Write the behavior spec before saving"
		return nil
	}
	d := m.editReturn
	id, _ := d.args["id"].(string)
	b := m.libraryBehavior(id)
	args := map[string]any{"spec": text}
	if b == nil {
		args["name"] = d.fields[0].input.Value()
	} else {
		args["id"] = b.ID
		args["name"] = b.Name
	}
	return m.submitEditor("behavior.save", args)
}

func libraryImportHelp(kind string) string {
	if kind == "eval-policy-behavior-library" || kind == "operational-policy-library-selection" {
		return "Imports Off. Review complete-trace wording and Pass when, then enable.\n"
	}
	return ""
}

// Library state compares the definition, not policy-specific detection settings.
func (m *model) behaviorLibraryRow(b evaluationBehavior) row {
	label := "Save to library"
	source := m.libraryBehavior(b.SourceID)
	if source != nil {
		switch {
		case source.Name == b.Name && source.Spec == b.Spec:
			label = "Saved in library ✓"
		case source.Revision != b.SourceRevision:
			label = "Library update available…"
		default:
			label = "Modified · Update library…"
		}
	} else {
		for _, candidate := range m.data.BehaviorLibrary {
			if candidate.Name == b.Name && candidate.Spec == b.Spec {
				label = "Saved in library ✓"
				break
			}
		}
		if b.SourceID != "" && label == "Save to library" {
			label = "Removed from library · Save again"
		}
	}
	return row{id: "library-save", label: label, preview: "Library stores name and spec. Policy settings stay here. Open to review; updates never change other policy copies or past results."}
}

func (m *model) openPolicyBehaviorLibrary(parent *dialog, policy string, b evaluationBehavior) tea.Cmd {
	source := m.libraryBehavior(b.SourceID)
	args := map[string]any{"policy": policy, "behavior": b.ID, "action": "save"}
	if purpose, _ := m.operationalContext(); purpose == "selection" {
		args["purpose"] = purpose
	}
	if source == nil {
		return m.saveDialog(&dialog{parent: parent}, "behavior.publish", args)
	}
	rows := []row{{id: "cancel", label: "Back", preview: "Keep the policy and library unchanged."}}
	if source.Name != b.Name || source.Spec != b.Spec {
		rows = append(rows, row{id: "update", label: "Update library from this policy…", preview: "Replace the library name and spec. Other policy copies remain unchanged."}, row{id: "refresh", label: "Use library version in this policy…", preview: source.Name + "\n" + source.Spec + "\nKeeps this policy’s enabled state, Pass when and threshold."})
	}
	rows = append(rows, row{id: "copy", label: "Save as separate spec…", preview: "Create a separate library identity and link this policy to it."})
	args["revision"] = source.Revision
	m.dialog = &dialog{kind: "behavior-library-sync", title: m.behaviorLibraryRow(b).label, parent: parent, args: args, rows: rows}
	return nil
}

func (m *model) submitLibrarySync(d *dialog) tea.Cmd {
	r := d.rows[d.index]
	if r.id == "cancel" {
		m.dialog = d.parent
		return nil
	}
	if d.kind == "behavior-library-sync" {
		args := map[string]any{}
		for k, v := range d.args {
			args[k] = v
		}
		args["action"] = r.id
		m.dialog = &dialog{kind: "behavior-library-sync-confirm", title: r.label, parent: d, args: args, rows: []row{{id: "cancel", label: "Cancel"}, {id: "confirm", label: "Confirm", preview: r.preview}}}
		return nil
	}
	return m.saveDialog(&dialog{parent: d.parent.parent}, "behavior.publish", d.args)
}

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
	d.rows = append(d.rows, row{id: "new", label: "+ New behavior", preview: "Write a reusable spec. Add copies to judges in Monitoring, Selection or Evals."})
	m.dialog = d
	return nil
}
func (m *model) openBehaviorLibraryPicker(parent *dialog, kind string, args map[string]any) tea.Cmd {
	d := &dialog{kind: kind, title: "From library", parent: parent, args: args}
	for _, b := range m.data.BehaviorLibrary {
		d.rows = append(d.rows, row{id: b.ID, label: b.Name, preview: b.Spec})
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

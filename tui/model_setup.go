package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type setupPlan struct {
	Name, Repo, Revision, License string
	Files                         []string
	Size                          *int64
}

// Setup shares the standard dialogs, keyboard navigation and theme. Parent links
// retain typed values and selection when Escape moves back through the flow.
func (m *model) setupEvent(raw json.RawMessage) tea.Cmd {
	var e struct {
		Stage, Message, Name, File string
		Presets                    []string
		Plans                      []setupPlan
		Downloaded, Total          int64
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		m.status = err.Error()
		return nil
	}
	m.pending = false
	switch e.Stage {
	case "home":
		d := &dialog{kind: "setup-home", title: "Set up a model", parent: m.setupReturn, args: map[string]any{}}
		for i, name := range e.Presets {
			d.rows = append(d.rows, row{id: strconv.Itoa(i), label: name, preview: "Local base model · review size and license before downloading."})
		}
		d.rows = append(d.rows, row{id: "hub", label: "From Hugging Face", preview: "Choose a GGUF from a repository or paste a file URL."}, row{id: "local", label: "Use a local GGUF", preview: "Use an existing file in place, without copying it."}, row{id: "skip", label: "Skip for now", preview: "Browse documents offline. Add a model later from /model."})
		m.dialog = d
	case "plans":
		if m.dialog == nil || m.dialog.kind != "setup-busy" {
			return nil
		}
		d := &dialog{kind: "setup-files", title: "Choose a GGUF", parent: m.dialog.parent, args: m.dialog.args}
		for i, plan := range e.Plans {
			size := "Size unavailable"
			if plan.Size != nil {
				size = fmt.Sprintf("%.2f GiB", float64(*plan.Size)/(1<<30))
			}
			preview := fmt.Sprintf("%s · %d file(s) · %s\n%s\nRevision %s", size, len(plan.Files), plan.License, plan.Repo, plan.Revision)
			d.rows = append(d.rows, row{id: strconv.Itoa(i), label: plan.Name, preview: preview})
		}
		m.dialog = d
		if len(d.rows) == 1 {
			return m.submitSetup(d)
		}
	case "progress":
		if m.dialog != nil && m.dialog.kind == "setup-busy" {
			if e.File != "" {
				m.dialog.title = "Downloading · " + e.File
			}
			if e.Total > 0 {
				m.dialog.rows[0].preview = fmt.Sprintf("%.0f%% · %.1f / %.1f MiB\nESC cancels; cached progress is retained.", float64(e.Downloaded)*100/float64(e.Total), float64(e.Downloaded)/(1<<20), float64(e.Total)/(1<<20))
			}
		}
	case "complete":
		m.dialog = &dialog{kind: "setup-done", title: "Model ready", rows: []row{{id: "done", label: "Continue to Carla", preview: e.Name + "\nSelected and saved. Generation requires llama-server on PATH."}}}
	case "error":
		var parent *dialog
		if m.dialog != nil {
			parent = m.dialog.parent
		}
		m.dialog = &dialog{kind: "setup-error", title: "Model setup failed", parent: parent, rows: []row{{id: "back", label: "Back", preview: e.Message}}}
	case "cancelled":
		// Escape already restored the preceding dialog.
	}
	m.reflow()
	return nil
}

func (m *model) setupBusy(parent *dialog, args map[string]any, title string) {
	m.dialog = &dialog{kind: "setup-busy", title: title, parent: parent, args: args, rows: []row{{id: "cancel", label: "Cancel", preview: "Working… ESC cancels this operation."}}}
}

func (m *model) submitSetup(d *dialog) tea.Cmd {
	if len(d.rows) == 0 && len(d.fields) == 0 {
		return nil
	}
	switch d.kind {
	case "setup-home":
		id := d.rows[d.index].id
		if id == "skip" {
			m.dialog = nil
			return nil
		}
		if id == "hub" || id == "local" {
			next := &dialog{kind: "setup-input", title: "Hugging Face GGUF", parent: d, args: map[string]any{"origin": id}}
			label := "Repository or GGUF URL"
			if id == "local" {
				next.title = "Local GGUF"
				label = "File path"
			}
			next.add(label, "")
			m.dialog = next
			m.reflow()
			return next.fields[0].input.Focus()
		}
		index, _ := strconv.Atoi(id)
		args := map[string]any{"preset": index, "kind": "base"}
		m.setupBusy(d, args, "Checking model details")
		return m.send("setup.plan", args)
	case "setup-input":
		value := strings.TrimSpace(d.fields[0].input.Value())
		if value == "" {
			return nil
		}
		key := "source"
		if d.args["origin"] == "local" {
			key = "path"
		}
		m.dialog = &dialog{kind: "setup-role", title: "Model type", parent: d, args: map[string]any{key: value}, rows: []row{{id: "base", label: "Base · Loom and Simulator", preview: "Choose the model's actual training type; GGUF does not identify it."}, {id: "instruct", label: "Instruct · Grow selector", preview: "A separate selection model. Existing policy configuration is preserved."}}}
	case "setup-role":
		args := d.args
		args["kind"] = d.rows[d.index].id
		command := "setup.plan"
		title := "Checking model details"
		if _, ok := args["path"]; ok {
			command = "setup.local"
			title = "Adding local model"
		}
		m.setupBusy(d, args, title)
		return m.send(command, args)
	case "setup-files":
		selected := d.rows[d.index]
		index, _ := strconv.Atoi(selected.id)
		m.dialog = &dialog{kind: "setup-confirm", title: "Download model?", parent: d, args: map[string]any{"index": index, "kind": d.args["kind"]}, rows: []row{{id: "download", label: "Download", preview: selected.preview}, {id: "back", label: "Back", preview: "Choose another file or source."}}}
	case "setup-confirm":
		if d.rows[d.index].id == "back" {
			return m.closeDialog()
		}
		m.setupBusy(d, d.args, "Starting download")
		return m.send("setup.download", d.args)
	case "setup-busy":
		return m.closeDialog()
	case "setup-error":
		return m.closeDialog()
	case "setup-done":
		m.dialog = nil
	}
	m.reflow()
	return nil
}

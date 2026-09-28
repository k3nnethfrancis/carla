package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"os"
	"path/filepath"
)

// Exit is local, never queued behind an inference/backend request. Closing the
// connection in runSession cancels the backend job and releases its workspace.
// Save editor-only text separately: recovery must not mutate the project graph.
func (m *model) exitSession(restart bool) tea.Cmd {
	if m.editing != "" {
		dir := filepath.Join(m.data.Workspace.Path, "recovered-drafts")
		data, err := json.MarshalIndent(map[string]any{
			"kind": m.editing, "node": m.editNode, "evaluation": m.evalEditingID, "text": m.editor.Value(),
			"offset": textOffset(m.editor.Value(), m.editor.Line(), m.editor.Column()),
		}, "", "  ")
		if err == nil {
			err = os.MkdirAll(dir, 0700)
		}
		if err == nil {
			var f *os.File
			f, err = os.CreateTemp(dir, "draft-*.json")
			if err == nil {
				_, err = f.Write(data)
				if err == nil {
					err = f.Sync()
				}
				closeErr := f.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
		if err != nil {
			m.status = "Error: could not preserve draft: " + err.Error()
			return nil
		}
	}
	m.restarting = restart
	return tea.Quit
}

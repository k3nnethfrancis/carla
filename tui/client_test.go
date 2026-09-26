package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Exercise the real Go→Python boundary with an isolated on-disk workspace.
// Inference itself is separately tested through the Python runtime contract.
func TestPythonTransport(t *testing.T) {
	python, err := filepath.Abs("../.venv/bin/python")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, "-m", "character_lab.backend", "--workspace", "test")
	cmd.Env = append(os.Environ(), "CARLA_DATA_DIR="+t.TempDir())
	c, err := startClient(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { c.conn.Close(); cmd.Process.Kill(); cmd.Wait() }()
	c.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	read := func(kind string) event {
		t.Helper()
		msg := c.read()()
		e, ok := msg.(event)
		if !ok {
			t.Fatalf("expected event: %#v", msg)
		}
		if e.Type != kind {
			t.Fatalf("expected %s, got %s: %s", kind, e.Type, e.Data)
		}
		return e
	}
	read("library")
	read("state")
	read("setup")
	if _, ok := c.send("seed.toggle", map[string]any{"ref": "gunkel:table"})().(sent); !ok {
		t.Fatal("send failed")
	}
	e := read("state")
	var s state
	json.Unmarshal(e.Data, &s)
	if s.Current == nil || len(s.Selected) != 1 {
		t.Fatal("selection not persisted")
	}
	c.send("node.edit", map[string]any{"node": s.Current.ID, "text": "🙂 a new path"})()
	read("state")
	var empty map[string]any
	c.send("seed.clear", empty)()
	e = read("state")
	json.Unmarshal(e.Data, &s)
	if s.Current == nil || len(s.Selected) != 0 || len(s.Nodes) != 2 {
		t.Fatal("clear lost history")
	}
	c.send("quit", nil)()
	read("bye")
}

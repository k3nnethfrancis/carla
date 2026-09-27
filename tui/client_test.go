package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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

// A broken TUI connection must cancel its in-flight generation, retain its
// partial trace, release the workspace lock, and permit the next launch.
func TestPythonTransportDisconnectRecoversPartialAndLock(t *testing.T) {
	data := t.TempDir()
	const alias = "synthetic-base"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			fmt.Fprint(w, "ok")
		case "/v1/models":
			fmt.Fprintf(w, `{"data":[{"id":%q}]}`, alias)
		case "/tokenize":
			fmt.Fprint(w, `{"tokens":[1,2,3]}`)
		case "/props":
			fmt.Fprint(w, `{"default_generation_settings":{"n_ctx":2048},"total_slots":1}`)
		case "/completion":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"content\":\" visible partial\",\"stop\":false,\"tokens_predicted\":1}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer func() { server.CloseClientConnections(); server.Close() }()
	port := server.Listener.Addr().(*net.TCPAddr).Port
	modelPath := filepath.Join(data, "synthetic.gguf")
	if err := os.WriteFile(modelPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	modelsPath := filepath.Join(data, "models.json")
	models, _ := json.Marshal(map[string]any{"alias": alias, "kind": "base", "name": "Synthetic", "path": modelPath, "port": port, "context": 2048})
	if err := os.WriteFile(modelsPath, models, 0600); err != nil {
		t.Fatal(err)
	}
	python, err := filepath.Abs("../.venv/bin/python")
	if err != nil {
		t.Fatal(err)
	}
	launch := func() (*client, *exec.Cmd) {
		cmd := exec.Command(python, "-m", "character_lab.backend", "--workspace", "test", "--models", modelsPath)
		cmd.Env = append(os.Environ(), "CARLA_DATA_DIR="+data)
		c, err := startClient(cmd)
		if err != nil {
			t.Fatal(err)
		}
		c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		return c, cmd
	}
	read := func(c *client, kind string) event {
		t.Helper()
		msg := c.read()()
		e, ok := msg.(event)
		if !ok || e.Type != kind {
			t.Fatalf("expected %s, got %#v", kind, msg)
		}
		return e
	}
	wait := func(cmd *exec.Cmd) {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("backend exit: %v", err)
			}
		case <-time.After(8 * time.Second):
			cmd.Process.Kill()
			<-done
			t.Fatal("backend retained a disconnected session")
		}
	}
	c, cmd := launch()
	defer cmd.Process.Kill()
	read(c, "library")
	read(c, "state")
	if _, ok := c.send("seed.toggle", map[string]any{"ref": "gunkel:table"})().(sent); !ok {
		t.Fatal("seed selection failed")
	}
	read(c, "state")
	if _, ok := c.send("continue", map[string]any{"n_predict": 16})().(sent); !ok {
		t.Fatal("continuation did not start")
	}
	seen := false
	for i := 0; i < 30; i++ {
		msg := c.read()()
		e, ok := msg.(event)
		if !ok {
			t.Fatalf("stream failed before partial output: %#v", msg)
		}
		if e.Type == "token" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatal("no partial token arrived")
	}
	c.conn.Close()
	wait(cmd)

	project, err := os.ReadFile(filepath.Join(data, "test", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Nodes []struct {
			Text, Status string
			Trace        map[string]any
		}
	}
	if err := json.Unmarshal(project, &saved); err != nil {
		t.Fatal(err)
	}
	last := saved.Nodes[len(saved.Nodes)-1]
	if last.Status == "complete" || last.Text == "" || last.Trace["request"] == nil {
		t.Fatalf("partial generation lost or marked complete: %+v", last)
	}

	reopened, second := launch()
	defer second.Process.Kill()
	read(reopened, "library")
	read(reopened, "state")
	reopened.send("quit", nil)()
	read(reopened, "bye")
	reopened.conn.Close()
	wait(second)
}

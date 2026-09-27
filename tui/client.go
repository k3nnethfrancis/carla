package main

import (
	"bufio"
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"
)

type event struct {
	V    int             `json:"v"`
	Type string          `json:"type"`
	Seq  int             `json:"seq"`
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}
type failure struct{ err error }
type sent struct{}
type client struct {
	conn    net.Conn
	decoder *json.Decoder
	mu      sync.Mutex
	serial  int
}

func startClient(cmd *exec.Cmd) (*client, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	var hello struct {
		V, Port int
		Token   string
	}
	ready := make(chan error, 1)
	go func() { ready <- json.NewDecoder(bufio.NewReader(stdout)).Decode(&hello) }()
	select {
	case err = <-ready:
	case <-time.After(30 * time.Second):
		err = fmt.Errorf("backend startup timed out")
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("backend: %w: %s", err, diagnostics.String())
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", hello.Port), 5*time.Second)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	c := &client{conn: conn, decoder: json.NewDecoder(conn)}
	err = json.NewEncoder(conn).Encode(map[string]any{"v": 1, "token": hello.Token})
	if err != nil {
		conn.Close()
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	return c, nil
}

// Exactly one pending read preserves stream ordering. Bubble Tea schedules the
// next read after applying each event; TCP provides bounded backpressure.
func (c *client) read() tea.Cmd {
	return func() tea.Msg {
		var e event
		if err := c.decoder.Decode(&e); err != nil {
			return failure{err}
		}
		if e.V != 1 {
			return failure{fmt.Errorf("unsupported backend protocol %d", e.V)}
		}
		return e
	}
}
func (c *client) send(command string, args map[string]any) tea.Cmd {
	_, cmd := c.request(command, args)
	return cmd
}

// Reserve the ID before scheduling I/O so the editor can match its save reply.
func (c *client) request(command string, args map[string]any) (string, tea.Cmd) {
	c.mu.Lock()
	c.serial++
	id := fmt.Sprint(c.serial)
	c.mu.Unlock()
	return id, func() tea.Msg {
		c.mu.Lock()
		defer c.mu.Unlock()
		if args == nil {
			args = map[string]any{}
		}
		c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		err := json.NewEncoder(c.conn).Encode(map[string]any{"v": 1, "id": id, "command": command, "args": args})
		if err != nil {
			return failure{err}
		}
		return sent{}
	}
}

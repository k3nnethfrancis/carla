// Carla's terminal process owns presentation only. Python owns the workspace.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--help" || arg == "-h" {
			fmt.Println("carla [--workspace NAME | --project PATH] [--models JSON] [--policy-model JSON]\nPython launcher: carla --setup-model to add or download a model.\n\nLocal document Loom. Resumes your last workspace.\nCTRL+W workspaces · CTRL+K actions · /keys bindings · CTRL+C quit\nRequires a UTF-8 terminal of at least 60 × 18 cells.")
			return
		}
	}
	if !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stderr, "Carla needs an interactive terminal. Use --help for options.")
		os.Exit(1)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	workspace, err := runSession()
	if err != nil || workspace == "" {
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(binary, append([]string{binary}, restartArgs(os.Args[1:], workspace)...), os.Environ())
}

// Preserve model overrides, but reopen the workspace selected in the UI rather
// than reapplying the original launch workspace.
func restartArgs(args []string, workspace string) []string {
	result := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--workspace", "--project":
			i++
		default:
			if strings.HasPrefix(args[i], "--workspace=") || strings.HasPrefix(args[i], "--project=") {
				continue
			}
			result = append(result, args[i])
		}
	}
	return append(result, "--project", workspace)
}

func runSession() (string, error) {
	python := os.Getenv("CARLA_PYTHON")
	if python == "" {
		binary, err := os.Executable()
		if err != nil {
			return "", err
		}
		python = filepath.Join(filepath.Dir(binary), "..", ".venv", "bin", "python")
	}
	cmd := exec.Command(python, append([]string{"-m", "character_lab.backend"}, os.Args[1:]...)...)
	client, err := startClient(cmd)
	if err != nil {
		return "", err
	}
	defer func() {
		client.conn.Close() // EOF makes Python cancel/save any in-flight operation.
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	}()
	final, err := tea.NewProgram(newModel(client)).Run()
	if err == nil && final.(*model).restarting {
		return final.(*model).data.Workspace.Path, nil
	}
	return "", err
}

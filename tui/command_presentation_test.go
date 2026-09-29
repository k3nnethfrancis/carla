package main

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestCommandDescriptionPrecedesParameters(t *testing.T) {
	for _, section := range []int{1, 3} {
		m := fixture()
		m.width, m.height, m.section = 100, 30, section
		m.focusCommand(true)
		m.command.SetValue("/loom")
		text := ansi.Strip(m.commandView())
		description := "Continue documents"
		if section == 3 {
			description = "Start conversations"
		}
		if strings.Index(text, description) < 0 || strings.Index(text, description) > strings.Index(text, "--tokens") {
			t.Fatal(text)
		}
		if !strings.Contains(text, "› /loom") {
			t.Fatal("selected command lacks marker", text)
		}
	}
}

func TestFooterSaveAlignment(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		m := fixture()
		m.width = width
		text := ansi.Strip(m.footerView("TAB next · SHIFT+TAB back · ESC out · /keys", "Saved locally"))
		if ansi.StringWidth(text) != width-1 || !strings.HasSuffix(text, "Saved locally") {
			t.Fatal(width, text)
		}
	}
}

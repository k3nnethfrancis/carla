package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestLabelScrollLifecycle(t *testing.T) {
	s := labelScrollState{}
	target := labelTarget{"row-1", "✓ a long title", 8}
	if s.sync(target) == nil || s.offset != 0 {
		t.Fatal("overflow must schedule the initial pause without moving")
	}
	token := labelScrollTick(s.generation)
	if s.sync(target) != nil {
		t.Fatal("unrelated updates must not schedule duplicate timers")
	}
	for expected := 1; expected <= s.limit; expected++ {
		if s.advance(token) == nil || s.offset != expected {
			t.Fatalf("step %d: %+v", expected, s)
		}
	}
	if s.advance(token) == nil || s.offset != 0 {
		t.Fatal("after end pause return to the beginning")
	}
	target.width++
	if s.sync(target) == nil || s.offset != 0 {
		t.Fatal("resize must restart the initial pause")
	}
	if s.advance(token) != nil || s.offset != 0 {
		t.Fatal("stale pre-resize tick moved the row")
	}
	token = labelScrollTick(s.generation)
	s.sync(labelTarget{})
	if s.advance(token) != nil || s.target.key != "" {
		t.Fatal("focus loss must cancel the timer chain")
	}
	if s.sync(labelTarget{"short", "Fits", 12}) != nil {
		t.Fatal("fitting text must stay idle")
	}
}

func TestLabelScrollIdentityAndManualNavigation(t *testing.T) {
	s := labelScrollState{}
	target := labelTarget{"row-1", "a very long title", 8}
	s.sync(target)
	s.advance(labelScrollTick(s.generation))
	for _, changed := range []labelTarget{{"row-2", target.text, target.width}, {"row-2", "a changed long title", target.width}} {
		token := labelScrollTick(s.generation)
		if s.sync(changed) == nil || s.offset != 0 || s.advance(token) != nil {
			t.Fatal("identity/content changes must invalidate pending animation")
		}
	}
	s.manualKey = "row-2"
	if s.sync(s.target) != nil || s.target.key != "" {
		t.Fatal("manual branch scrolling must suspend animation")
	}
	if s.sync(target) == nil || s.manualKey != "" {
		t.Fatal("new row should restore automatic reveal")
	}
}

func TestLabelScrollUnicodeAndStablePrefix(t *testing.T) {
	m := fixture()
	text := safe("  ▾ ✓ \x1b[31m日本語 👩‍💻 é long title\x1b[0m")
	target := labelTarget{"unicode", text, 17}
	m.labelScroll.sync(target)
	token := labelScrollTick(m.labelScroll.generation)
	for step := 0; step <= m.labelScroll.limit; step++ {
		got := m.scrollingLabel(text, target.width)
		if !strings.HasPrefix(got, "  ▾ ✓ ") || !utf8.ValidString(got) || ansi.StringWidth(got) != target.width || strings.Contains(got, "\x1b") {
			t.Fatalf("invalid cell window at %d: %q", step, got)
		}
		if step < m.labelScroll.limit {
			m.labelScroll.advance(token)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(m.scrollingLabel(text, target.width)), "title") {
		t.Fatal("last window should expose full title ending")
	}
}

func TestFocusedLabelScope(t *testing.T) {
	t.Setenv("REDUCE_MOTION", "")
	m := fixture()
	m.width, m.height, m.focus = 100, 32, 0
	m.sources[0].Title = strings.Repeat("long source title ", 5)
	if m.syncLabelScroll() == nil {
		t.Fatal("focused library overflow should animate")
	}
	for _, focus := range []int{1, 2, 3} {
		m.focus = focus
		if m.focusedLabelTarget().key != "" {
			t.Fatalf("focus %d must not animate sidebar", focus)
		}
	}
	m.focus, m.sectionFocus = 0, true
	if m.focusedLabelTarget().key != "" {
		t.Fatal("tab-header focus must not animate a list")
	}
	m.sectionFocus = false
	m.dialog = &dialog{kind: "picker", title: "Models", rows: []row{{id: "long", label: strings.Repeat("model ", 30)}}}
	if m.syncLabelScroll() == nil {
		t.Fatal("modal focused overflow should animate")
	}
	token := labelScrollTick(m.labelScroll.generation)
	old := *m.dialog
	m.dialog = &old
	if m.syncLabelScroll() != nil || labelScrollTick(m.labelScroll.generation) != token {
		t.Fatal("rebuilding an unchanged dialog must not reset its animation")
	}
	m.dialog = nil
	m.focus = 3
	m.syncLabelScroll()
	if _, cmd := m.Update(token); cmd != nil {
		t.Fatal("a stale modal tick should not cause backend effects or reschedule")
	}
	t.Setenv("REDUCE_MOTION", "1")
	m.focus = 0
	if m.focusedLabelTarget().key != "" {
		t.Fatal("reduced motion must suppress animation")
	}
}

func TestLabelScrollResizeThroughUpdate(t *testing.T) {
	t.Setenv("REDUCE_MOTION", "")
	m := fixture()
	m.width, m.height = 100, 32
	m.dialog = &dialog{kind: "picker", title: "Models", rows: []row{{id: "long", label: strings.Repeat("model ", 30)}}}
	m.syncLabelScroll()
	old := labelScrollTick(m.labelScroll.generation)
	m.labelScroll.advance(old)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	if m.labelScroll.offset != 0 || m.labelScroll.generation == uint64(old) {
		t.Fatal("resize did not restart focused title")
	}
	_, cmd := m.Update(old)
	if cmd != nil || m.labelScroll.offset != 0 {
		t.Fatal("old timer survived resize")
	}
}

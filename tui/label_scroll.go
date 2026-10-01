package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	labelScrollDelay    = time.Second
	labelScrollStep     = 125 * time.Millisecond
	labelScrollEndPause = 1500 * time.Millisecond
)

type labelTarget struct {
	key, text string
	width     int
}

type labelScrollTick uint64

// A generation token invalidates pending timers after navigation, resize or a
// content change. Only an overflowing, focused row owns a timer.
type labelScrollState struct {
	target        labelTarget
	generation    uint64
	offset, limit int
	manualKey     string
}

func (s *labelScrollState) timer(delay time.Duration) tea.Cmd {
	generation := s.generation
	return tea.Tick(delay, func(time.Time) tea.Msg { return labelScrollTick(generation) })
}

// Keep tree indentation and state marks still while moving only the title.
func labelParts(text string) (string, string) {
	body := strings.TrimLeft(text, " \t✓✔★☆▸▾▶▼▷▽›←+!│├└─")
	return text[:len(text)-len(body)], body
}

func (s *labelScrollState) sync(target labelTarget) tea.Cmd {
	if s.manualKey != "" && s.manualKey != target.key {
		s.manualKey = ""
	}
	prefix, body := labelParts(target.text)
	space := target.width - ansi.StringWidth(prefix)
	if space < 2 || ansi.StringWidth(body) <= space || target.key == s.manualKey {
		target = labelTarget{}
	}
	if target == s.target {
		return nil
	}
	s.generation++
	s.target, s.offset, s.limit = target, 0, 0
	if target.key == "" {
		return nil
	}
	s.limit = ansi.StringWidth(body) - space
	return s.timer(labelScrollDelay)
}

func (s *labelScrollState) advance(tick labelScrollTick) tea.Cmd {
	if uint64(tick) != s.generation || s.target.key == "" {
		return nil
	}
	if s.offset == s.limit {
		s.offset = 0
		return s.timer(labelScrollDelay)
	}
	s.offset++
	if s.offset == s.limit {
		return s.timer(labelScrollEndPause)
	}
	return s.timer(labelScrollStep)
}

// Unlike branchRowText, retain the hidden tail for the focused-row marquee.
func (m *model) navigationLabel(item row, r rect) string {
	text := strings.Repeat(" ", item.depth) + safe(item.label)
	if m.adaptiveBranches() {
		text = ansi.Cut(text, m.branchHorizontalOffset(r), ansi.StringWidth(text))
	}
	return text
}

func (m *model) focusedLabelTarget() labelTarget {
	if m.width < 60 || m.height < 18 || os.Getenv("REDUCE_MOTION") != "" {
		return labelTarget{}
	}
	if d := m.dialog; d != nil {
		if d.kind == "help-detail" || len(d.fields) > 0 || d.index < 0 || d.index >= len(d.rows) {
			return labelTarget{}
		}
		r := d.rows[d.index]
		return labelTarget{fmt.Sprintf("dialog:%s:%s:%d:%s", d.kind, d.title, d.index, r.id), safe(r.label), m.dialogRect().w - 4}
	}
	if m.focus != 0 || m.sectionFocus || m.searching {
		return labelTarget{}
	}
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return labelTarget{}
	}
	for _, p := range m.layout().panels {
		if p.kind == 0 {
			r := rows[m.selected]
			return labelTarget{fmt.Sprintf("nav:%d:%t:%d:%s:%s", m.section, m.notesOpen, m.selected, r.kind, r.id), m.navigationLabel(r, p.box), p.box.w - 4}
		}
	}
	return labelTarget{}
}

func (m *model) syncLabelScroll() tea.Cmd {
	return m.labelScroll.sync(m.focusedLabelTarget())
}

func (m *model) scrollingLabel(text string, width int) string {
	s := m.labelScroll
	if s.offset == 0 || s.target.text != text || s.target.width != width {
		return line(text, width)
	}
	prefix, body := labelParts(text)
	space := width - ansi.StringWidth(prefix)
	// Cut measures display cells and preserves grapheme clusters (including CJK
	// and emoji). A partially visible wide glyph is padded instead of split.
	return line(prefix+ansi.Cut(body, s.offset, s.offset+space), width)
}

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestGridBoundsNamesAndTail(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}, {120, 36}, {180, 48}} {
		m := fixture()
		m.width, m.height, m.section, m.focus = size[0], size[1], 1, 1
		m.loomGrid = true
		for i := 0; i < 7; i++ {
			m.loomTiles = append(m.loomTiles, loomTile{Title: fmt.Sprintf("Branch %d", i+1), Text: strings.Repeat("🙂 live text\n", 30) + "TAIL", Status: "generating"})
		}
		for _, p := range m.layout().panels {
			if p.kind == 1 {
				text := ansi.Strip(m.renderGrid(p.box))
				if lipgloss.Width(text) > p.box.w || lipgloss.Height(text) > p.box.h {
					t.Fatal(size, "grid overflow", text)
				}
				if !strings.Contains(text, "Branch 1") || !strings.Contains(text, "TAIL") {
					t.Fatal(size, text)
				}
				for n := 0; n < 5; n++ {
					m.gridKey("nav.right")
				}
				text = ansi.Strip(m.renderGrid(p.box))
				if !strings.Contains(text, "Branch 6") {
					t.Fatal("paging lost tile", text)
				}
			}
		}
	}
}
func TestGridReceivesInterleavedBranchChunks(t *testing.T) {
	m := fixture()
	m.section = 1
	m.apply(event{Type: "loom.start", Data: json.RawMessage(`{"count":2}`)})
	for i, id := range []string{"a", "b"} {
		data, _ := json.Marshal(map[string]any{"index": i, "id": id, "title": id, "status": "generating"})
		m.apply(event{Type: "loom.branch", Data: data})
	}
	m.apply(event{Type: "token", Data: json.RawMessage(`{"Node":"b","Text":"second"}`)})
	m.apply(event{Type: "token", Data: json.RawMessage(`{"Node":"a","Text":"first"}`)})
	if m.loomTiles[0].Text != "first" || m.loomTiles[1].Text != "second" {
		t.Fatal(m.loomTiles)
	}
	m.apply(event{Type: "loom.end", Data: json.RawMessage(`{"status":"stopped"}`)})
}
func TestReviewReasonIsExplicit(t *testing.T) {
	c := simulationConversation{Status: "needs_review", Turns: []simulationTurn{{Flags: []string{"token_limit"}}}}
	if conversationStatus(c) != "Stopped early · token cap reached" {
		t.Fatal(conversationStatus(c))
	}
}

func TestGridEdgesSelectionAndChrome(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 36}, {151, 45}} {
		m := fixture()
		m.width, m.height, m.section, m.focus = size[0], size[1], 1, 1
		m.loomGrid = true
		for i := 0; i < 4; i++ {
			m.loomTiles = append(m.loomTiles, loomTile{Title: fmt.Sprintf("Branch %d", i+1), Text: "A path continues.", Status: "complete"})
		}
		for _, p := range m.layout().panels {
			if p.kind != 1 {
				continue
			}
			rects := m.gridRects(p.box)
			first, last := rects[0], rects[len(rects)-1]
			if first.x != p.box.x || first.y != p.box.y || last.x+last.w != p.box.x+p.box.w || last.y+last.h != p.box.y+p.box.h {
				t.Fatal("grid not flush with panel", size, rects)
			}
			text := ansi.Strip(m.renderGrid(p.box))
			if lipgloss.Height(text) != p.box.h || lipgloss.Width(text) != p.box.w {
				t.Fatal("grid dimensions", size)
			}
			if !strings.HasPrefix(text, "╔") || strings.Contains(text, "Arrows select") || strings.Contains(text, "outputs") {
				t.Fatal("selection/chrome", text)
			}
			m.gridKey("nav.right")
			if m.gridSelected(p.box) != 1 {
				t.Fatal("selection failed")
			}
		}
		if size[0] >= 120 && !strings.Contains(ansi.Strip(m.sectionBar()), "4 outputs") {
			t.Fatal("missing inline summary")
		}
	}
}

func TestGridUsesAllSpaceForTwoAndThree(t *testing.T) {
	for _, n := range []int{2, 3, 5, 6, 7} {
		m := fixture()
		m.section = 1
		m.loomGrid = true
		m.gridPinned = true
		for i := 0; i < n; i++ {
			m.loomTiles = append(m.loomTiles, loomTile{Title: fmt.Sprintf("Branch %d", i), Text: "text"})
		}
		r := rect{1, 3, 100, 31}
		if n > 4 {
			m.gridSelection = 4
		}
		rects := m.gridRects(r)
		count := n
		if n > 4 {
			count = n - 4
		}
		if len(rects) != count {
			t.Fatal("empty slots", rects)
		}
		if count <= 2 && rects[0].h != r.h {
			t.Fatal("short tiles", rects)
		}
		last := rects[len(rects)-1]
		if count == 3 && (last.w != r.w || last.x != r.x) {
			t.Fatal("third tile not full width", rects)
		}
		if last.y+last.h != r.y+r.h {
			t.Fatal("unused bottom", rects)
		}
		if lipgloss.Height(m.renderGrid(r)) != r.h || lipgloss.Width(m.renderGrid(r)) != r.w {
			t.Fatal("render/geometry mismatch")
		}
	}
}

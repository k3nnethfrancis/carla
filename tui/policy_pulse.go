package main

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type policyDetection struct {
	Run, ID, Name, Action, Color string
	Conversation, Turn, Check    int
	Probability                  float64
}
type policyPulse struct {
	Detection policyDetection
	Started   time.Time
}
type policyTick time.Time

const pulseDuration = time.Second

func conversationKey(run string, index int) string { return fmt.Sprintf("%s:%d", run, index) }
func pulseTick() tea.Cmd {
	return tea.Tick(40*time.Millisecond, func(t time.Time) tea.Msg { return policyTick(t) })
}
func (m *model) detectPolicy(data json.RawMessage) tea.Cmd {
	var d policyDetection
	if json.Unmarshal(data, &d) != nil {
		return nil
	}
	if m.policySeen == nil {
		m.policySeen = map[string]bool{}
	}
	id := fmt.Sprintf("%s:%d:%d:%s:%d", d.Run, d.Conversation, d.Turn, d.ID, d.Check)
	if m.policySeen[id] {
		return nil
	}
	m.policySeen[id] = true
	if m.policyPulses == nil {
		m.policyPulses = map[string]policyPulse{}
	}
	idle := len(m.policyPulses) == 0
	m.policyPulses[conversationKey(d.Run, d.Conversation)] = policyPulse{d, time.Now()}
	if idle {
		return pulseTick()
	}
	return nil
}
func (m *model) advancePolicyPulse(now time.Time) tea.Cmd {
	for k, p := range m.policyPulses {
		if now.Sub(p.Started) >= 31*time.Second {
			delete(m.policyPulses, k)
		}
	}
	if len(m.policyPulses) > 0 {
		return pulseTick()
	}
	return nil
}

// Five fades with exponentially widening gaps. Fresh detections restart the train;
// historical snapshots never trigger animation.
func (m *model) pulseColor(key string) string {
	p, ok := m.policyPulses[key]
	if !ok {
		return ""
	}
	if _, off := os.LookupEnv("NO_COLOR"); off {
		return ""
	}
	if os.Getenv("TERM") == "dumb" {
		return ""
	}
	age, active := pulsePhase(time.Since(p.Started))
	if os.Getenv("REDUCE_MOTION") != "" {
		age = 0
		active = time.Since(p.Started) < 31*time.Second
	}
	if !active {
		return ""
	}
	colors := map[string][2]string{"amber": {"#947039", "#CFB179"}, "coral": {"#A84F39", "#DB937C"}, "blue": {"#526F91", "#A4BBD3"}, "violet": {"#79628C", "#B8A0CB"}}
	pair, ok := colors[p.Detection.Color]
	if !ok {
		pair = colors["amber"]
	}
	start, end := pair[0], "#777777"
	if m.dark {
		start, end = pair[1], "#AAAAAA"
	}
	fraction := float64(age) / float64(pulseDuration)
	if os.Getenv("REDUCE_MOTION") != "" {
		fraction = 0
	}
	var a, b uint32
	fmt.Sscanf(start, "#%x", &a)
	fmt.Sscanf(end, "#%x", &b)
	mix := func(shift uint) uint32 {
		return uint32(float64((a>>shift)&255)*(1-fraction) + float64((b>>shift)&255)*fraction)
	}
	return fmt.Sprintf("#%02X%02X%02X", mix(16), mix(8), mix(0))
}
func (m *model) pulseLabel(key, text string) string {
	if color := m.pulseColor(key); color != "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(text)
	}
	return text
}

func pulsePhase(age time.Duration) (time.Duration, bool) {
	for _, offset := range []time.Duration{0, 2 * time.Second, 6 * time.Second, 14 * time.Second, 30 * time.Second} {
		if age >= offset && age < offset+pulseDuration {
			return age - offset, true
		}
	}
	return 0, false
}

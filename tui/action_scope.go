package main

// actionScope preserves collection boundaries while leaves identify the exact
// saved targets. Ancestry is provenance, not implicit generation membership.
type actionScope struct {
	Label        string        `json:"label,omitempty"`
	ShortLabel   string        `json:"short_label,omitempty"`
	Title        string        `json:"title,omitempty"`
	Kind         string        `json:"kind"`
	ID           string        `json:"id,omitempty"`
	Node         string        `json:"node,omitempty"`
	Run          string        `json:"run,omitempty"`
	Conversation int           `json:"conversation"`
	Children     []actionScope `json:"children,omitempty"`
}

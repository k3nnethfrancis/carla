package main

import (
	"strings"
	"testing"
)

func TestOperationTreeRetainsNewestFirstAncestry(t *testing.T) {
	m := fixture()
	m.section = 1
	m.data.Nodes = []node{
		{ID: "root", Label: "doc-1"},
		{ID: "a", Parent: "root", Label: "branch-1-loom-1-doc-1"},
		{ID: "b", Parent: "root", Label: "branch-2-loom-1-doc-1"},
		{ID: "c", Parent: "a", Label: "continue-1-branch-1-loom-1-doc-1"},
	}
	m.data.DocumentSets = []documentSet{
		{ID: "one", Action: "loom", OperationID: "op", OperationLabel: "loom-1-doc-1", Label: "branch-1-loom-1-doc-1", Members: []string{"a"}},
		{ID: "two", Action: "loom", OperationID: "op", OperationLabel: "loom-1-doc-1", Label: "branch-2-loom-1-doc-1", Members: []string{"b"}},
	}
	rows := m.branchRows()
	want := []string{"root", "loom:op", "a", "c", "b"}
	for i, id := range want {
		if len(rows) != len(want) || rows[i].id != id {
			t.Fatalf("rows: %#v", rows)
		}
	}
	if !strings.Contains(rows[3].label, "continue-1-branch-1-loom-1-doc-1") {
		t.Fatal(rows[3])
	}
	scope := m.documentScope("loom:op")
	if scope.Kind != "set" || len(scope.Children) != 2 || scope.Children[0].Children[0].Node != "a" {
		t.Fatal(scope)
	}
	if len(m.documentSetMembers("loom:op")) != 2 {
		t.Fatal("Loom selection lost members")
	}
	m.collapsed["loom:op"] = true
	if len(m.branchRows()) != 2 {
		t.Fatal(m.branchRows())
	}
	m.selectDocument("c")
	if m.collapsed["loom:op"] || m.rows()[m.selected].id != "c" {
		t.Fatal("returning to a document must expand its Loom ancestor")
	}
	m.selected = 4
	m.branchArrow("left")
	if m.selected != 1 {
		t.Fatalf("left should return to Loom parent: %d", m.selected)
	}
}

func TestOperationTreeKeepsCustomNames(t *testing.T) {
	m := fixture()
	m.section = 1
	m.data.Nodes = []node{{ID: "root", Label: "doc-1", Title: "My source"}, {ID: "next", Parent: "root", Label: "continue-1-doc-1", Title: "My continuation"}}
	m.data.DocumentSets = []documentSet{{ID: "one", Action: "continue", OperationID: "op", Members: []string{"next"}}}
	rows := m.branchRows()
	if len(rows) != 2 || !strings.Contains(rows[0].label, "My source") || !strings.Contains(rows[1].label, "My continuation") {
		t.Fatal(rows)
	}
}

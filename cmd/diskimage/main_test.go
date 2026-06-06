package main

import (
	"testing"
)

func TestRootHasCommands(t *testing.T) {
	cmd := rootCmd()
	foundCreate := false
	foundExec := false
	for _, c := range cmd.Commands() {
		switch c.Name() {
		case "create":
			foundCreate = true
		case "exec":
			foundExec = true
		}
	}
	if !foundCreate || !foundExec {
		t.Fatalf("expected create and exec commands registered; create=%v exec=%v", foundCreate, foundExec)
	}
}

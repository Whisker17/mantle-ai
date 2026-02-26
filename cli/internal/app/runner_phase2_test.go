package app

import (
	"bytes"
	"encoding/json"
	"testing"
)

type schemaNode struct {
	Path        string       `json:"path"`
	Use         string       `json:"use"`
	Flags       []schemaFlag `json:"flags"`
	Subcommands []schemaNode `json:"subcommands"`
}

type schemaFlag struct {
	Name string `json:"name"`
}

func TestRunnerSchemaIncludesPhase2CommandRoots(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	r := NewRunnerWithWriters(&stdout, &stderr)

	code := r.Run([]string{"schema", "--results-only"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, stderr.String())
	}

	var root schemaNode
	if err := json.Unmarshal(stdout.Bytes(), &root); err != nil {
		t.Fatalf("failed to parse schema output: %v output=%s", err, stdout.String())
	}

	for _, cmd := range []string{"swap", "lend", "stake", "yield", "bridge"} {
		if !schemaHasSubcommand(root, cmd) {
			t.Fatalf("expected root schema to include %q subcommand", cmd)
		}
	}
}

func TestRunnerSchemaSwapQuoteFlags(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	r := NewRunnerWithWriters(&stdout, &stderr)

	code := r.Run([]string{"schema", "swap quote", "--results-only"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, stderr.String())
	}

	var node schemaNode
	if err := json.Unmarshal(stdout.Bytes(), &node); err != nil {
		t.Fatalf("failed to parse schema output: %v output=%s", err, stdout.String())
	}

	for _, flag := range []string{"from", "to", "amount", "fee-tier", "provider"} {
		if !schemaHasFlag(node, flag) {
			t.Fatalf("expected swap quote schema to include flag %q", flag)
		}
	}
}

func schemaHasSubcommand(node schemaNode, name string) bool {
	for _, sub := range node.Subcommands {
		if sub.Use == name || sub.Path == name {
			return true
		}
	}
	return false
}

func schemaHasFlag(node schemaNode, name string) bool {
	for _, flag := range node.Flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

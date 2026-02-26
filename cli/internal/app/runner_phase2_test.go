package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mantle/mantle-ai/cli/internal/config"
)

type schemaNode struct {
	Path        string       `json:"path"`
	Use         string       `json:"use"`
	Flags       []schemaFlag `json:"flags"`
	Subcommands []schemaNode `json:"subcommands"`
}

type schemaFlag struct {
	Name  string `json:"name"`
	Usage string `json:"usage"`
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

func TestRunnerSchemaLendRatesProtocolIncludesAaveV3(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	r := NewRunnerWithWriters(&stdout, &stderr)

	code := r.Run([]string{"schema", "lend rates", "--results-only"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, stderr.String())
	}

	var node schemaNode
	if err := json.Unmarshal(stdout.Bytes(), &node); err != nil {
		t.Fatalf("failed to parse schema output: %v output=%s", err, stdout.String())
	}

	usage := ""
	for _, flag := range node.Flags {
		if flag.Name == "protocol" {
			usage = flag.Usage
			break
		}
	}
	if usage == "" {
		t.Fatalf("expected protocol flag in lend rates schema, got %+v", node.Flags)
	}
	if !strings.Contains(usage, "aave_v3") {
		t.Fatalf("expected protocol usage to include aave_v3, got %q", usage)
	}
}

func TestLendingProviderEntriesAllIncludesAaveV3(t *testing.T) {
	state := &runtimeState{
		settings: config.Settings{
			Network: "mainnet",
			RPCURL:  "http://127.0.0.1:8545",
			Providers: map[string]config.ProviderSettings{
				"lendle":   {Enabled: true},
				"aurelius": {Enabled: true},
				"aave_v3":  {Enabled: true},
			},
		},
	}
	entries, err := state.lendingProviderEntries("all")
	if err != nil {
		t.Fatalf("lendingProviderEntries failed: %v", err)
	}
	names := map[string]bool{}
	for _, entry := range entries {
		names[entry.name] = true
	}
	for _, expected := range []string{"lendle", "aurelius", "aave_v3"} {
		if !names[expected] {
			t.Fatalf("missing %s provider in all entries: %v", expected, names)
		}
	}
	if len(entries) < 3 {
		t.Fatalf("expected at least 3 entries, got %d (%v)", len(entries), names)
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

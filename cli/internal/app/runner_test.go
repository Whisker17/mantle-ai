package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mantle/mantle-ai/cli/internal/version"
)

func TestRunnerVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	r := NewRunnerWithWriters(&stdout, &stderr)
	code := r.Run([]string{"version"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, stderr.String())
	}
	if got := stdout.String(); got != version.CLIVersion+"\n" {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestRunnerProvidersList(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	r := NewRunnerWithWriters(&stdout, &stderr)
	code := r.Run([]string{"providers", "list", "--results-only"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, stderr.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse output json: %v output=%s", err, stdout.String())
	}
	if len(out) == 0 {
		t.Fatal("expected providers output, got empty")
	}
}

func TestRunnerSchemaBypassesProviderInit(t *testing.T) {
	t.Setenv("MANTLE_RPC_URL", "http://127.0.0.1:1")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	r := NewRunnerWithWriters(&stdout, &stderr)
	code := r.Run([]string{"schema", "token info", "--results-only"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "token info") {
		t.Fatalf("expected schema output, got %s", stdout.String())
	}
}

func TestRunnerErrorEnvelopeIgnoresResultsOnly(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	r := NewRunnerWithWriters(&stdout, &stderr)
	code := r.Run([]string{"chain", "info", "--enable-commands", "token info", "--results-only"})
	if code != 16 {
		t.Fatalf("expected exit 16, got %d stderr=%s", code, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("failed to parse error envelope: %v output=%s", err, stderr.String())
	}
	if env["success"] != false {
		t.Fatalf("expected success=false, got %v", env["success"])
	}
}

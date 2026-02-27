package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mantle/mantle-ai/cli/internal/config"
	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/providers"
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

func TestRunYieldCollectorsFastSuccessNotBlockedBySlowFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	collectors := []yieldCollector{
		{
			name: "slow",
			run: func(ctx context.Context) ([]model.YieldOpportunity, error) {
				select {
				case <-time.After(400 * time.Millisecond):
					return []model.YieldOpportunity{{Protocol: "slow", Asset: "SLOW", APYTotal: 1}}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		},
		{
			name: "fast",
			run: func(ctx context.Context) ([]model.YieldOpportunity, error) {
				return []model.YieldOpportunity{{Protocol: "fast", Asset: "USDC", APYTotal: 2}}, nil
			},
		},
	}

	start := time.Now()
	results := runYieldCollectors(ctx, collectors)
	elapsed := time.Since(start)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].err == nil {
		t.Fatalf("expected slow collector to fail on timeout")
	}
	if results[1].err != nil {
		t.Fatalf("expected fast collector to succeed, got err: %v", results[1].err)
	}
	if len(results[1].items) != 1 || results[1].items[0].Protocol != "fast" {
		t.Fatalf("unexpected fast collector output: %+v", results[1].items)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("expected parallel execution under timeout budget, elapsed=%s", elapsed)
	}
}

func TestFetchBestSwapQuoteReturnsNetworkLevelUnsupportedOnSepolia(t *testing.T) {
	state := &runtimeState{
		settings: config.Settings{
			Network: "sepolia",
			RPCURL:  "https://rpc.sepolia.mantle.xyz",
			Providers: map[string]config.ProviderSettings{
				"agni":         {Enabled: false},
				"merchant_moe": {Enabled: false},
			},
		},
	}

	req := providers.SwapQuoteRequest{
		FromAsset: id.Asset{
			Address:  "0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9",
			Symbol:   "USDC",
			Decimals: 6,
		},
		ToAsset: id.Asset{
			Address:  "0xdEAddEaDdeadDEadDEADDEAddEADDEAddead1111",
			Symbol:   "WMNT",
			Decimals: 18,
		},
		AmountBaseUnits: "1000000",
		AmountDecimal:   "1",
		FeeTier:         3000,
	}

	data, statuses, warnings, partial, err := state.fetchBestSwapQuote(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error when sepolia swap providers are not configured")
	}
	if data != nil {
		t.Fatalf("expected nil data, got %+v", data)
	}
	if !partial {
		t.Fatalf("expected partial=true when providers are unavailable")
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 provider statuses, got %d", len(statuses))
	}
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %v", len(warnings), warnings)
	}
	typed, ok := clierr.As(err)
	if !ok {
		t.Fatalf("expected typed cli error, got %T", err)
	}
	if typed.Code != clierr.CodeUnsupported {
		t.Fatalf("expected unsupported code, got %d (%v)", typed.Code, typed)
	}
	if !strings.Contains(strings.ToLower(typed.Message), "no swap quote providers configured for network sepolia") {
		t.Fatalf("unexpected error message: %s", typed.Message)
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

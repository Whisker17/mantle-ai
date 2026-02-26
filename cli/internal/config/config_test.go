package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrecedenceFlagsOverEnvOverFile(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(configPath, []byte("output: plain\nnetwork: sepolia\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("MANTLE_OUTPUT", "json")
	settings, err := Load(GlobalFlags{ConfigPath: configPath, Plain: true, Network: "mainnet"})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.OutputMode != "plain" {
		t.Fatalf("expected plain output mode from flags, got %s", settings.OutputMode)
	}
	if settings.Network != "mainnet" {
		t.Fatalf("expected mainnet from flags, got %s", settings.Network)
	}
}

func TestLoadMutuallyExclusiveOutputFlags(t *testing.T) {
	_, err := Load(GlobalFlags{JSON: true, Plain: true})
	if err == nil {
		t.Fatal("expected error with --json and --plain")
	}
}

func TestLoadNetworkRPCSelection(t *testing.T) {
	settings, err := Load(GlobalFlags{Network: "sepolia"})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.Network != "sepolia" {
		t.Fatalf("expected sepolia, got %s", settings.Network)
	}
	if settings.RPCURL != sepoliaRPCDefault {
		t.Fatalf("expected default sepolia RPC, got %s", settings.RPCURL)
	}
}

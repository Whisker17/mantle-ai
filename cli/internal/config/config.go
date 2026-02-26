package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	mainnetRPCDefault = "https://rpc.mantle.xyz"
	sepoliaRPCDefault = "https://rpc.sepolia.mantle.xyz"
)

type GlobalFlags struct {
	ConfigPath     string
	JSON           bool
	Plain          bool
	Select         string
	ResultsOnly    bool
	EnableCommands string
	Strict         bool
	Timeout        string
	Retries        int
	MaxStale       string
	NoStale        bool
	NoCache        bool
	Network        string
	RPCURL         string
}

type Settings struct {
	OutputMode     string
	SelectFields   []string
	ResultsOnly    bool
	EnableCommands []string
	Strict         bool
	Timeout        time.Duration
	Retries        int
	MaxStale       time.Duration
	NoStale        bool
	CacheEnabled   bool
	CachePath      string
	CacheLockPath  string
	Network        string
	RPCURL         string
	RPCURLMainnet  string
	RPCURLSepolia  string
	AcrossAPIKey   string
	Providers      map[string]ProviderSettings
}

type ProviderSettings struct {
	Enabled    bool
	APIKeyEnv  string
	APIKey     string
	Configured bool
}

type fileConfig struct {
	Output        string `yaml:"output"`
	Strict        *bool  `yaml:"strict"`
	Timeout       string `yaml:"timeout"`
	Retries       *int   `yaml:"retries"`
	Network       string `yaml:"network"`
	RPCURL        string `yaml:"rpc_url"`
	RPCURLSepolia string `yaml:"rpc_url_sepolia"`
	Cache         struct {
		Enabled  *bool  `yaml:"enabled"`
		MaxStale string `yaml:"max_stale"`
		Path     string `yaml:"path"`
		LockPath string `yaml:"lock_path"`
	} `yaml:"cache"`
	Providers map[string]struct {
		Enabled   *bool  `yaml:"enabled"`
		APIKeyEnv string `yaml:"api_key_env"`
		APIKey    string `yaml:"api_key"`
	} `yaml:"providers"`
}

func Load(flags GlobalFlags) (Settings, error) {
	settings, err := defaultSettings()
	if err != nil {
		return Settings{}, err
	}

	cfgPath, err := resolveConfigPath(flags.ConfigPath)
	if err != nil {
		return Settings{}, err
	}
	if err := applyFileConfig(cfgPath, &settings); err != nil {
		return Settings{}, err
	}

	applyEnv(&settings)
	if err := applyFlags(flags, &settings); err != nil {
		return Settings{}, err
	}

	if settings.OutputMode == "" {
		settings.OutputMode = "json"
	}
	if settings.Timeout <= 0 {
		settings.Timeout = 10 * time.Second
	}
	if settings.Retries < 0 {
		settings.Retries = 0
	}
	if settings.MaxStale < 0 {
		settings.MaxStale = 5 * time.Minute
	}

	settings.Network = normalizeNetwork(settings.Network)
	if settings.Network == "" {
		settings.Network = "mainnet"
	}
	if settings.RPCURL == "" {
		if settings.Network == "sepolia" {
			settings.RPCURL = settings.RPCURLSepolia
		} else {
			settings.RPCURL = settings.RPCURLMainnet
		}
	}

	for name, provider := range settings.Providers {
		provider.Configured = provider.APIKey != ""
		settings.Providers[name] = provider
	}

	return settings, nil
}

func defaultSettings() (Settings, error) {
	cachePath, lockPath, err := defaultCachePaths()
	if err != nil {
		return Settings{}, err
	}
	providers := map[string]ProviderSettings{}
	for _, name := range []string{"rpc", "agni", "merchant_moe", "lendle", "aurelius", "aave_v3", "meth", "mantle_bridge", "across", "pendle", "defillama"} {
		providers[name] = ProviderSettings{Enabled: true}
	}
	return Settings{
		OutputMode:    "json",
		Timeout:       10 * time.Second,
		Retries:       2,
		MaxStale:      5 * time.Minute,
		CacheEnabled:  true,
		CachePath:     cachePath,
		CacheLockPath: lockPath,
		Network:       "mainnet",
		RPCURLMainnet: mainnetRPCDefault,
		RPCURLSepolia: sepoliaRPCDefault,
		Providers:     providers,
	}, nil
}

func resolveConfigPath(input string) (string, error) {
	if strings.TrimSpace(input) != "" {
		return input, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mantle", "config.yaml"), nil
}

func defaultCachePaths() (string, string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "mantle")
	return filepath.Join(dir, "cache.db"), filepath.Join(dir, "cache.lock"), nil
}

func applyFileConfig(path string, settings *Settings) error {
	buf, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config: %w", err)
	}

	var cfg fileConfig
	if err := yaml.Unmarshal(buf, &cfg); err != nil {
		return fmt.Errorf("parse config yaml: %w", err)
	}

	if cfg.Output != "" {
		settings.OutputMode = strings.ToLower(cfg.Output)
	}
	if cfg.Strict != nil {
		settings.Strict = *cfg.Strict
	}
	if cfg.Timeout != "" {
		d, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return fmt.Errorf("config timeout: %w", err)
		}
		settings.Timeout = d
	}
	if cfg.Retries != nil {
		settings.Retries = *cfg.Retries
	}
	if cfg.Network != "" {
		settings.Network = normalizeNetwork(cfg.Network)
	}
	if cfg.RPCURL != "" {
		settings.RPCURLMainnet = cfg.RPCURL
	}
	if cfg.RPCURLSepolia != "" {
		settings.RPCURLSepolia = cfg.RPCURLSepolia
	}
	if cfg.Cache.Enabled != nil {
		settings.CacheEnabled = *cfg.Cache.Enabled
	}
	if cfg.Cache.MaxStale != "" {
		d, err := time.ParseDuration(cfg.Cache.MaxStale)
		if err != nil {
			return fmt.Errorf("config cache.max_stale: %w", err)
		}
		settings.MaxStale = d
	}
	if cfg.Cache.Path != "" {
		settings.CachePath = cfg.Cache.Path
	}
	if cfg.Cache.LockPath != "" {
		settings.CacheLockPath = cfg.Cache.LockPath
	}

	for name, providerCfg := range cfg.Providers {
		norm := strings.ToLower(name)
		provider := settings.Providers[norm]
		if providerCfg.Enabled != nil {
			provider.Enabled = *providerCfg.Enabled
		}
		if providerCfg.APIKey != "" {
			provider.APIKey = providerCfg.APIKey
		}
		if providerCfg.APIKeyEnv != "" {
			provider.APIKeyEnv = providerCfg.APIKeyEnv
			provider.APIKey = os.Getenv(providerCfg.APIKeyEnv)
		}
		settings.Providers[norm] = provider
	}

	if across, ok := settings.Providers["across"]; ok {
		settings.AcrossAPIKey = across.APIKey
	}

	return nil
}

func applyEnv(settings *Settings) {
	if v := os.Getenv("MANTLE_OUTPUT"); v != "" {
		settings.OutputMode = strings.ToLower(v)
	}
	if v := os.Getenv("MANTLE_STRICT"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			settings.Strict = parsed
		}
	}
	if v := os.Getenv("MANTLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			settings.Timeout = d
		}
	}
	if v := os.Getenv("MANTLE_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			settings.Retries = n
		}
	}
	if v := os.Getenv("MANTLE_MAX_STALE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			settings.MaxStale = d
		}
	}
	if v := os.Getenv("MANTLE_NETWORK"); v != "" {
		settings.Network = normalizeNetwork(v)
	}
	if v := os.Getenv("MANTLE_RPC_URL"); v != "" {
		settings.RPCURL = v
	}
	if v := os.Getenv("MANTLE_RPC_URL_SEPOLIA"); v != "" {
		settings.RPCURLSepolia = v
	}

	if v := os.Getenv("ACROSS_API_KEY"); v != "" {
		provider := settings.Providers["across"]
		provider.APIKey = v
		provider.APIKeyEnv = "ACROSS_API_KEY"
		settings.Providers["across"] = provider
		settings.AcrossAPIKey = v
	}
}

func applyFlags(flags GlobalFlags, settings *Settings) error {
	if flags.JSON && flags.Plain {
		return fmt.Errorf("--json and --plain are mutually exclusive")
	}
	if flags.JSON {
		settings.OutputMode = "json"
	}
	if flags.Plain {
		settings.OutputMode = "plain"
	}
	settings.SelectFields = splitCSV(flags.Select)
	settings.ResultsOnly = flags.ResultsOnly
	settings.EnableCommands = splitCSV(flags.EnableCommands)
	if flags.Strict {
		settings.Strict = true
	}
	if flags.Timeout != "" {
		d, err := time.ParseDuration(flags.Timeout)
		if err != nil {
			return fmt.Errorf("--timeout: %w", err)
		}
		settings.Timeout = d
	}
	if flags.Retries >= 0 {
		settings.Retries = flags.Retries
	}
	if flags.MaxStale != "" {
		d, err := time.ParseDuration(flags.MaxStale)
		if err != nil {
			return fmt.Errorf("--max-stale: %w", err)
		}
		settings.MaxStale = d
	}
	if flags.NoStale {
		settings.NoStale = true
	}
	if flags.NoCache {
		settings.CacheEnabled = false
	}
	if flags.Network != "" {
		norm := normalizeNetwork(flags.Network)
		if norm == "" {
			return fmt.Errorf("unsupported network: %s", flags.Network)
		}
		settings.Network = norm
	}
	if flags.RPCURL != "" {
		settings.RPCURL = flags.RPCURL
	}
	return nil
}

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		norm := strings.ToLower(strings.TrimSpace(part))
		if norm != "" {
			out = append(out, norm)
		}
	}
	return out
}

func normalizeNetwork(v string) string {
	norm := strings.ToLower(strings.TrimSpace(v))
	switch norm {
	case "", "mainnet", "mantle", "5000", "eip155:5000":
		if norm == "" {
			return ""
		}
		return "mainnet"
	case "sepolia", "mantle-sepolia", "5003", "eip155:5003":
		return "sepolia"
	default:
		return ""
	}
}

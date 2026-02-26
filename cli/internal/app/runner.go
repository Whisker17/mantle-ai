package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mantle/mantle-ai/cli/internal/cache"
	"github.com/mantle/mantle-ai/cli/internal/config"
	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/out"
	"github.com/mantle/mantle-ai/cli/internal/policy"
	"github.com/mantle/mantle-ai/cli/internal/providers"
	"github.com/mantle/mantle-ai/cli/internal/providers/rpc"
	"github.com/mantle/mantle-ai/cli/internal/schema"
	"github.com/mantle/mantle-ai/cli/internal/version"
	"github.com/spf13/cobra"
)

type Runner struct {
	stdout io.Writer
	stderr io.Writer
	now    func() time.Time
}

func NewRunner() *Runner {
	return NewRunnerWithWriters(os.Stdout, os.Stderr)
}

func NewRunnerWithWriters(stdout, stderr io.Writer) *Runner {
	return &Runner{
		stdout: stdout,
		stderr: stderr,
		now:    time.Now,
	}
}

type runtimeState struct {
	runner        *Runner
	flags         config.GlobalFlags
	settings      config.Settings
	cache         *cache.Store
	root          *cobra.Command
	lastCommand   string
	lastWarnings  []string
	lastProviders []model.ProviderStatus
	lastPartial   bool

	chainProvider providers.ChainProvider
	providerInfos []model.ProviderInfo
}

const cachePayloadSchemaVersion = "v1"

func (r *Runner) Run(args []string) int {
	state := &runtimeState{runner: r}
	root := state.newRootCommand()
	state.root = root
	state.resetCommandDiagnostics()
	root.SetArgs(args)
	root.SetOut(r.stdout)
	root.SetErr(r.stderr)
	root.SilenceUsage = true
	root.SilenceErrors = true

	err := root.Execute()
	err = normalizeRunError(err)
	if err == nil {
		if state.cache != nil {
			_ = state.cache.Close()
		}
		state.closeProvider()
		return 0
	}

	state.renderError("", err, state.lastWarnings, state.lastProviders, state.lastPartial)
	if state.cache != nil {
		_ = state.cache.Close()
	}
	state.closeProvider()
	return clierr.ExitCode(err)
}

func (s *runtimeState) closeProvider() {
	if closer, ok := s.chainProvider.(interface{ Close() }); ok {
		closer.Close()
	}
}

func (s *runtimeState) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   version.CLIName,
		Short: "Agent-first Mantle retrieval CLI",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "help" {
				return nil
			}
			settings, err := config.Load(s.flags)
			if err != nil {
				return clierr.Wrap(clierr.CodeUsage, "load configuration", err)
			}
			s.settings = settings

			path := trimRootPath(cmd.CommandPath())
			s.lastCommand = path
			if err := policy.CheckCommandAllowed(settings.EnableCommands, path); err != nil {
				return err
			}

			s.providerInfos = buildProviderInfos(settings)
			if shouldInitProvider(path) {
				provider, err := rpc.New(rpc.Config{Network: settings.Network, RPCURL: settings.RPCURL})
				if err != nil {
					return err
				}
				s.chainProvider = provider
			}

			if settings.CacheEnabled && shouldOpenCache(path) && s.cache == nil {
				cacheStore, err := cache.Open(settings.CachePath, settings.CacheLockPath)
				if err != nil {
					s.settings.CacheEnabled = false
				} else {
					s.cache = cacheStore
				}
			}
			return nil
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clierr.Wrap(clierr.CodeUsage, "parse flags", err)
	})

	cmd.PersistentFlags().BoolVar(&s.flags.JSON, "json", false, "Output JSON (default)")
	cmd.PersistentFlags().BoolVar(&s.flags.Plain, "plain", false, "Output plain text")
	cmd.PersistentFlags().StringVar(&s.flags.Select, "select", "", "Select fields from data (comma-separated)")
	cmd.PersistentFlags().BoolVar(&s.flags.ResultsOnly, "results-only", false, "Output only data payload")
	cmd.PersistentFlags().StringVar(&s.flags.EnableCommands, "enable-commands", "", "Allowlist command paths (comma-separated)")
	cmd.PersistentFlags().BoolVar(&s.flags.Strict, "strict", false, "Fail on partial results")
	cmd.PersistentFlags().StringVar(&s.flags.Timeout, "timeout", "", "Provider request timeout")
	cmd.PersistentFlags().IntVar(&s.flags.Retries, "retries", -1, "Retries per provider request")
	cmd.PersistentFlags().StringVar(&s.flags.MaxStale, "max-stale", "", "Maximum stale fallback window after TTL expiry")
	cmd.PersistentFlags().BoolVar(&s.flags.NoStale, "no-stale", false, "Reject stale cache entries")
	cmd.PersistentFlags().BoolVar(&s.flags.NoCache, "no-cache", false, "Disable cache reads and writes")
	cmd.PersistentFlags().StringVar(&s.flags.ConfigPath, "config", "", "Path to config file")
	cmd.PersistentFlags().StringVar(&s.flags.Network, "network", "", "Target network: mainnet (default) or sepolia")
	cmd.PersistentFlags().StringVar(&s.flags.RPCURL, "rpc-url", "", "Override RPC endpoint")

	cmd.AddCommand(s.newSchemaCommand())
	cmd.AddCommand(s.newProvidersCommand())
	cmd.AddCommand(s.newChainCommand())
	cmd.AddCommand(s.newBalanceCommand())
	cmd.AddCommand(s.newTransactionCommand())
	cmd.AddCommand(s.newContractCommand())
	cmd.AddCommand(s.newTokenCommand())
	cmd.AddCommand(s.newSwapCommand())
	cmd.AddCommand(s.newLendCommand())
	cmd.AddCommand(s.newStakeCommand())
	cmd.AddCommand(s.newYieldCommand())
	cmd.AddCommand(s.newBridgeCommand())
	cmd.AddCommand(newVersionCommand())

	return cmd
}

func newVersionCommand() *cobra.Command {
	var long bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			if long {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), version.Long())
				return
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), version.CLIVersion)
		},
	}
	cmd.Flags().BoolVar(&long, "long", false, "Print extended build metadata")
	return cmd
}

func (s *runtimeState) newSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema [command path]",
		Short: "Print machine-readable command schema",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = strings.Join(args, " ")
			}
			data, err := schema.Build(s.root, path)
			if err != nil {
				return clierr.Wrap(clierr.CodeUsage, "build schema", err)
			}
			return s.emitSuccess(trimRootPath(cmd.CommandPath()), data, nil, cacheMetaBypass(), nil, false)
		},
	}
	return cmd
}

func (s *runtimeState) newProvidersCommand() *cobra.Command {
	root := &cobra.Command{Use: "providers", Short: "Provider commands"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List available providers and auth status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.emitSuccess(trimRootPath(cmd.CommandPath()), s.providerInfos, nil, cacheMetaBypass(), nil, false)
		},
	}
	root.AddCommand(list)
	return root
}

func (s *runtimeState) newChainCommand() *cobra.Command {
	root := &cobra.Command{Use: "chain", Short: "Mantle chain commands"}

	info := &cobra.Command{
		Use:   "info",
		Short: "Get Mantle chain configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			key := cacheKey("chain info", map[string]any{"network": s.settings.Network, "rpc": s.settings.RPCURL})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, time.Hour, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				start := time.Now()
				data, err := s.chainProvider.ChainInfo(ctx)
				status := model.ProviderStatus{Name: "rpc", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					return nil, []model.ProviderStatus{status}, nil, false, err
				}
				return data, []model.ProviderStatus{status}, nil, false, nil
			})
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Get current chain status",
		RunE: func(cmd *cobra.Command, args []string) error {
			key := cacheKey("chain status", map[string]any{"network": s.settings.Network, "rpc": s.settings.RPCURL})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 15*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				start := time.Now()
				data, err := s.chainProvider.ChainStatus(ctx)
				status := model.ProviderStatus{Name: "rpc", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					return nil, []model.ProviderStatus{status}, nil, false, err
				}
				return data, []model.ProviderStatus{status}, nil, false, nil
			})
		},
	}

	root.AddCommand(info)
	root.AddCommand(status)
	return root
}

func (s *runtimeState) newBalanceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "balance <address>",
		Short: "Get MNT and known token balances",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			address := strings.TrimSpace(args[0])
			key := cacheKey("balance", map[string]any{"network": s.settings.Network, "address": strings.ToLower(address)})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 30*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				start := time.Now()
				data, err := s.chainProvider.GetBalance(ctx, address)
				status := model.ProviderStatus{Name: "rpc", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					return nil, []model.ProviderStatus{status}, nil, false, err
				}
				return data, []model.ProviderStatus{status}, nil, false, nil
			})
		},
	}
	return cmd
}

func (s *runtimeState) newTransactionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx <hash>",
		Short: "Get transaction details and receipt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hash := strings.TrimSpace(args[0])
			key := cacheKey("tx", map[string]any{"network": s.settings.Network, "hash": strings.ToLower(hash)})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 24*time.Hour, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				start := time.Now()
				data, err := s.chainProvider.GetTransaction(ctx, hash)
				status := model.ProviderStatus{Name: "rpc", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					return nil, []model.ProviderStatus{status}, nil, false, err
				}
				return data, []model.ProviderStatus{status}, nil, false, nil
			})
		},
	}
	return cmd
}

func (s *runtimeState) newContractCommand() *cobra.Command {
	root := &cobra.Command{Use: "contract", Short: "Contract commands"}

	read := &cobra.Command{
		Use:   "read <address> <function> [args...]",
		Short: "Read contract state via call",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := providers.ContractReadRequest{
				Address:  args[0],
				Function: args[1],
				Args:     args[2:],
			}

			ctx, cancel := context.WithTimeout(context.Background(), s.settings.Timeout)
			defer cancel()

			start := time.Now()
			data, err := s.chainProvider.ReadContract(ctx, req)
			providerStatus := []model.ProviderStatus{{
				Name:      "rpc",
				Status:    statusFromErr(err),
				LatencyMS: time.Since(start).Milliseconds(),
			}}
			if err != nil {
				s.captureCommandDiagnostics(nil, providerStatus, false)
				return err
			}
			return s.emitSuccess(trimRootPath(cmd.CommandPath()), data, nil, cacheMetaBypass(), providerStatus, false)
		},
	}

	var from string
	var value string
	call := &cobra.Command{
		Use:   "call <address> <function> [args...]",
		Short: "Simulate contract call and estimate gas",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := providers.ContractCallRequest{
				From:     from,
				To:       args[0],
				Function: args[1],
				Args:     args[2:],
				Value:    value,
			}

			ctx, cancel := context.WithTimeout(context.Background(), s.settings.Timeout)
			defer cancel()

			start := time.Now()
			data, err := s.chainProvider.SimulateCall(ctx, req)
			providerStatus := []model.ProviderStatus{{
				Name:      "rpc",
				Status:    statusFromErr(err),
				LatencyMS: time.Since(start).Milliseconds(),
			}}
			if err != nil {
				s.captureCommandDiagnostics(nil, providerStatus, false)
				return err
			}
			return s.emitSuccess(trimRootPath(cmd.CommandPath()), data, nil, cacheMetaBypass(), providerStatus, false)
		},
	}
	call.Flags().StringVar(&from, "from", "", "Sender address for simulation")
	call.Flags().StringVar(&value, "value", "", "MNT value in ether units")
	_ = call.MarkFlagRequired("from")

	root.AddCommand(read)
	root.AddCommand(call)
	return root
}

func (s *runtimeState) newTokenCommand() *cobra.Command {
	root := &cobra.Command{Use: "token", Short: "Token commands"}

	info := &cobra.Command{
		Use:   "info <address>",
		Short: "Get token metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			address := strings.TrimSpace(args[0])
			key := cacheKey("token info", map[string]any{"network": s.settings.Network, "address": strings.ToLower(address)})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, time.Hour, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				start := time.Now()
				data, err := s.chainProvider.GetTokenInfo(ctx, address)
				status := model.ProviderStatus{Name: "rpc", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					return nil, []model.ProviderStatus{status}, nil, false, err
				}
				return data, []model.ProviderStatus{status}, nil, false, nil
			})
		},
	}

	resolve := &cobra.Command{
		Use:   "resolve <symbol>",
		Short: "Resolve token symbol to address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			symbol := strings.TrimSpace(args[0])
			key := cacheKey("token resolve", map[string]any{"network": s.settings.Network, "symbol": strings.ToUpper(symbol)})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, time.Hour, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				start := time.Now()
				data, err := s.chainProvider.ResolveToken(ctx, symbol)
				status := model.ProviderStatus{Name: "rpc", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					return nil, []model.ProviderStatus{status}, nil, false, err
				}
				return data, []model.ProviderStatus{status}, nil, false, nil
			})
		},
	}

	balances := &cobra.Command{
		Use:   "balances <address>",
		Short: "Get known token balances for an address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			address := strings.TrimSpace(args[0])
			key := cacheKey("token balances", map[string]any{"network": s.settings.Network, "address": strings.ToLower(address)})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 30*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				start := time.Now()
				data, err := s.chainProvider.GetTokenBalances(ctx, address)
				status := model.ProviderStatus{Name: "rpc", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				if err != nil {
					return nil, []model.ProviderStatus{status}, nil, false, err
				}
				return data, []model.ProviderStatus{status}, nil, false, nil
			})
		},
	}

	root.AddCommand(info)
	root.AddCommand(resolve)
	root.AddCommand(balances)
	return root
}

type fetchFn func(ctx context.Context) (data any, providerStatus []model.ProviderStatus, warnings []string, partial bool, err error)

func (s *runtimeState) runCachedCommand(commandPath, key string, ttl time.Duration, fetch fetchFn) error {
	s.resetCommandDiagnostics()
	cacheStatus := cacheMetaMiss()
	warnings := []string{}
	var staleData any
	staleAvailable := false
	staleObservedAge := time.Duration(0)
	staleObservedAt := time.Time{}
	staleCacheStatus := cacheMetaMiss()

	if s.settings.CacheEnabled && s.cache != nil {
		cached, err := s.cache.Get(key, s.settings.MaxStale)
		if err == nil && cached.Hit {
			entryStatus := model.CacheStatus{Status: "hit", AgeMS: cached.Age.Milliseconds(), Stale: cached.Stale}
			if !cached.Stale {
				var data any
				if err := json.Unmarshal(cached.Value, &data); err == nil {
					s.captureCommandDiagnostics(warnings, nil, false)
					return s.emitSuccess(commandPath, data, warnings, entryStatus, nil, false)
				}
			} else {
				var data any
				if err := json.Unmarshal(cached.Value, &data); err == nil {
					staleData = data
					staleAvailable = true
					staleObservedAge = cached.Age
					staleObservedAt = time.Now()
					staleCacheStatus = entryStatus
				}
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.settings.Timeout)
	defer cancel()
	data, providerStatus, providerWarnings, partial, err := fetch(ctx)
	warnings = append(warnings, providerWarnings...)
	s.captureCommandDiagnostics(warnings, providerStatus, partial)
	if err != nil {
		if staleAvailable {
			if !staleFallbackAllowed(err) {
				return err
			}
			currentStaleAge := staleObservedAge
			if !staleObservedAt.IsZero() {
				currentStaleAge += time.Since(staleObservedAt)
			}
			staleCacheStatus.AgeMS = currentStaleAge.Milliseconds()
			if s.settings.NoStale {
				return clierr.Wrap(clierr.CodeStale, "fresh provider fetch failed and stale fallback is disabled (--no-stale)", err)
			}
			if staleExceedsBudget(currentStaleAge, ttl, s.settings.MaxStale) {
				return clierr.Wrap(clierr.CodeStale, "fresh provider fetch failed and cached data exceeded stale budget", err)
			}
			warnings = append(warnings, "provider fetch failed; serving stale data within max-stale budget")
			s.captureCommandDiagnostics(warnings, providerStatus, false)
			return s.emitSuccess(commandPath, staleData, warnings, staleCacheStatus, providerStatus, false)
		}
		return err
	}

	if partial && s.settings.Strict {
		s.captureCommandDiagnostics(warnings, providerStatus, true)
		return clierr.New(clierr.CodePartialStrict, "partial results returned in strict mode")
	}

	if s.settings.CacheEnabled && s.cache != nil {
		if payload, err := json.Marshal(data); err == nil {
			_ = s.cache.Set(key, payload, ttl)
			cacheStatus = model.CacheStatus{Status: "write", AgeMS: 0, Stale: false}
		}
	}

	s.captureCommandDiagnostics(warnings, providerStatus, partial)
	return s.emitSuccess(commandPath, data, warnings, cacheStatus, providerStatus, partial)
}

func (s *runtimeState) emitSuccess(commandPath string, data any, warnings []string, cacheStatus model.CacheStatus, providers []model.ProviderStatus, partial bool) error {
	env := model.Envelope{
		Version:  model.EnvelopeVersion,
		Success:  true,
		Data:     data,
		Error:    nil,
		Warnings: warnings,
		Meta: model.EnvelopeMeta{
			RequestID: newRequestID(),
			Timestamp: s.runner.now().UTC(),
			Command:   commandPath,
			Providers: providers,
			Cache:     cacheStatus,
			Partial:   partial,
		},
	}
	return out.Render(s.runner.stdout, env, s.settings)
}

func (s *runtimeState) renderError(commandPath string, err error, warnings []string, providers []model.ProviderStatus, partial bool) {
	if strings.TrimSpace(commandPath) == "" {
		commandPath = s.lastCommand
		if commandPath == "" {
			commandPath = version.CLIName
		}
	}
	code := clierr.ExitCode(err)
	typ := "internal_error"
	message := err.Error()
	if cErr, ok := clierr.As(err); ok {
		message = cErr.Message
		if cErr.Cause != nil {
			message = fmt.Sprintf("%s: %v", cErr.Message, cErr.Cause)
		}
		switch cErr.Code {
		case clierr.CodeUsage:
			typ = "usage_error"
		case clierr.CodeAuth:
			typ = "auth_error"
		case clierr.CodeRateLimited:
			typ = "rate_limited"
		case clierr.CodeUnavailable:
			typ = "provider_unavailable"
		case clierr.CodeUnsupported:
			typ = "unsupported"
		case clierr.CodeStale:
			typ = "stale_data"
		case clierr.CodePartialStrict:
			typ = "partial_results"
		case clierr.CodeBlocked:
			typ = "command_blocked"
		}
	}

	settings := s.settings
	if settings.OutputMode == "" {
		settings.OutputMode = "json"
	}
	settings.ResultsOnly = false
	settings.SelectFields = nil
	env := model.Envelope{
		Version: model.EnvelopeVersion,
		Success: false,
		Data:    []any{},
		Error: &model.ErrorBody{
			Code:    code,
			Type:    typ,
			Message: message,
		},
		Warnings: warnings,
		Meta: model.EnvelopeMeta{
			RequestID: newRequestID(),
			Timestamp: s.runner.now().UTC(),
			Command:   commandPath,
			Providers: providers,
			Cache:     cacheMetaBypass(),
			Partial:   partial,
		},
	}
	_ = out.Render(s.runner.stderr, env, settings)
}

func cacheKey(commandPath string, req any) string {
	buf, _ := json.Marshal(req)
	prefix := []byte(commandPath + "|" + cachePayloadSchemaVersion + "|")
	sum := sha256.Sum256(append(prefix, buf...))
	return hex.EncodeToString(sum[:])
}

func newRequestID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func trimRootPath(path string) string {
	parts := strings.Fields(path)
	if len(parts) <= 1 {
		return path
	}
	return strings.Join(parts[1:], " ")
}

func statusFromErr(err error) string {
	if err == nil {
		return "ok"
	}
	if cErr, ok := clierr.As(err); ok {
		switch cErr.Code {
		case clierr.CodeAuth:
			return "auth_error"
		case clierr.CodeRateLimited:
			return "rate_limited"
		case clierr.CodeUnavailable:
			return "unavailable"
		default:
			return "error"
		}
	}
	return "error"
}

func cacheMetaBypass() model.CacheStatus {
	return model.CacheStatus{Status: "bypass", AgeMS: 0, Stale: false}
}

func cacheMetaMiss() model.CacheStatus {
	return model.CacheStatus{Status: "miss", AgeMS: 0, Stale: false}
}

func normalizeRunError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := clierr.As(err); ok {
		return err
	}
	if isLikelyUsageError(err) {
		return clierr.Wrap(clierr.CodeUsage, "invalid command input", err)
	}
	return clierr.Wrap(clierr.CodeInternal, "execute command", err)
}

func isLikelyUsageError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	patterns := []string{
		"unknown command",
		"unknown flag",
		"required flag(s)",
		"flag needs an argument",
		"requires at least",
		"requires exactly",
		"accepts ",
		"invalid argument",
		"invalid args",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

func staleExceedsBudget(age, ttl, maxStale time.Duration) bool {
	if age <= ttl {
		return false
	}
	if maxStale < 0 {
		return false
	}
	return age > ttl+maxStale
}

func staleFallbackAllowed(err error) bool {
	cErr, ok := clierr.As(err)
	if !ok {
		return false
	}
	return cErr.Code == clierr.CodeUnavailable || cErr.Code == clierr.CodeRateLimited
}

func shouldOpenCache(commandPath string) bool {
	switch normalizeCommandPath(commandPath) {
	case "", "version", "schema", "providers", "providers list":
		return false
	default:
		return true
	}
}

func shouldInitProvider(commandPath string) bool {
	norm := normalizeCommandPath(commandPath)
	if norm == "" || norm == "version" || norm == "schema" || norm == "providers" || norm == "providers list" {
		return false
	}
	return strings.HasPrefix(norm, "chain") ||
		strings.HasPrefix(norm, "balance") ||
		strings.HasPrefix(norm, "tx") ||
		strings.HasPrefix(norm, "contract") ||
		strings.HasPrefix(norm, "token")
}

func normalizeCommandPath(commandPath string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(commandPath))), " ")
}

func (s *runtimeState) resetCommandDiagnostics() {
	s.lastWarnings = nil
	s.lastProviders = nil
	s.lastPartial = false
}

func (s *runtimeState) captureCommandDiagnostics(warnings []string, providers []model.ProviderStatus, partial bool) {
	if len(warnings) == 0 {
		s.lastWarnings = nil
	} else {
		s.lastWarnings = append([]string(nil), warnings...)
	}
	if len(providers) == 0 {
		s.lastProviders = nil
	} else {
		s.lastProviders = append([]model.ProviderStatus(nil), providers...)
	}
	s.lastPartial = partial
}

func buildProviderInfos(settings config.Settings) []model.ProviderInfo {
	providersList := []model.ProviderInfo{
		{Name: "rpc", Type: "onchain", Capabilities: []string{"chain", "balance", "tx", "contract", "token"}},
		{Name: "agni", Type: "dex", Capabilities: []string{"swap.quote"}},
		{Name: "merchant_moe", Type: "dex", Capabilities: []string{"swap.quote"}},
		{Name: "lendle", Type: "lending", Capabilities: []string{"lend.markets", "lend.rates"}},
		{Name: "aurelius", Type: "lending", Capabilities: []string{"lend.markets", "lend.rates"}},
		{Name: "meth", Type: "staking", Capabilities: []string{"stake.info", "stake.quote"}},
		{Name: "mantle_bridge", Type: "bridge", Capabilities: []string{"bridge.quote", "bridge.status"}},
		{Name: "across", Type: "bridge", RequiresKey: true, KeyEnvVarName: "ACROSS_API_KEY", Capabilities: []string{"bridge.quote"}},
		{Name: "pendle", Type: "yield", Capabilities: []string{"yield.opportunities"}},
		{Name: "defillama", Type: "market-data", Capabilities: []string{"yield.aggregate"}},
	}

	for i := range providersList {
		name := providersList[i].Name
		if cfg, ok := settings.Providers[name]; ok {
			providersList[i].Enabled = cfg.Enabled
			if providersList[i].RequiresKey {
				providersList[i].AuthConfigured = cfg.APIKey != ""
			}
		} else {
			providersList[i].Enabled = true
		}
		if !providersList[i].RequiresKey {
			providersList[i].AuthConfigured = true
		}
	}
	return providersList
}

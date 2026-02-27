package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/providers"
	"github.com/mantle/mantle-ai/cli/internal/providers/aavev3"
	"github.com/mantle/mantle-ai/cli/internal/providers/across"
	"github.com/mantle/mantle-ai/cli/internal/providers/agni"
	"github.com/mantle/mantle-ai/cli/internal/providers/aurelius"
	"github.com/mantle/mantle-ai/cli/internal/providers/defillama"
	"github.com/mantle/mantle-ai/cli/internal/providers/lendle"
	"github.com/mantle/mantle-ai/cli/internal/providers/mantlebridge"
	"github.com/mantle/mantle-ai/cli/internal/providers/merchantmoe"
	"github.com/mantle/mantle-ai/cli/internal/providers/meth"
	"github.com/mantle/mantle-ai/cli/internal/providers/pendle"
	"github.com/mantle/mantle-ai/cli/internal/providers/rpc"
	"github.com/spf13/cobra"
)

var hexAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var nativeAssetPattern = regexp.MustCompile(`^(?i:eth|mnt)$`)

const (
	providersDoctorHistoryKey = "providers_doctor_history_v1"
	providersDoctorHistoryTTL = 365 * 24 * time.Hour
)

type providersDoctorHistory struct {
	Entries map[string]providersDoctorHistoryEntry `json:"entries"`
}

type providersDoctorHistoryEntry struct {
	LastSuccessAt string `json:"last_success_at,omitempty"`
	LastFailureAt string `json:"last_failure_at,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

func (s *runtimeState) newTransferCommand() *cobra.Command {
	var from string
	var to string
	var asset string
	var amount string

	root := &cobra.Command{Use: "transfer", Short: "Transfer simulation commands"}
	simulate := &cobra.Command{
		Use:   "simulate",
		Short: "Simulate native or ERC-20 transfer and estimate gas",
		RunE: func(cmd *cobra.Command, args []string) error {
			fromAddr := strings.TrimSpace(from)
			toAddr := strings.TrimSpace(to)
			assetInput := strings.TrimSpace(asset)
			amountInput := strings.TrimSpace(amount)

			if !hexAddressPattern.MatchString(fromAddr) {
				return clierr.New(clierr.CodeUsage, "invalid from address")
			}
			if !hexAddressPattern.MatchString(toAddr) {
				return clierr.New(clierr.CodeUsage, "invalid to address")
			}
			if amountInput == "" {
				return clierr.New(clierr.CodeUsage, "amount is required")
			}
			if assetInput == "" {
				assetInput = "MNT"
			}

			key := cacheKey("transfer simulate", map[string]any{
				"network": s.settings.Network,
				"from":    strings.ToLower(fromAddr),
				"to":      strings.ToLower(toAddr),
				"asset":   strings.ToLower(assetInput),
				"amount":  amountInput,
			})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 30*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				return s.fetchTransferSimulation(ctx, fromAddr, toAddr, assetInput, amountInput)
			})
		},
	}
	simulate.Flags().StringVar(&from, "from", "", "Sender address")
	simulate.Flags().StringVar(&to, "to", "", "Recipient address")
	simulate.Flags().StringVar(&asset, "asset", "MNT", "Asset symbol/address/CAIP-19 (MNT for native transfer)")
	simulate.Flags().StringVar(&amount, "amount", "", "Amount in decimal units")
	_ = simulate.MarkFlagRequired("from")
	_ = simulate.MarkFlagRequired("to")
	_ = simulate.MarkFlagRequired("amount")

	root.AddCommand(simulate)
	return root
}

func (s *runtimeState) fetchTransferSimulation(ctx context.Context, from, to, assetInput, amountInput string) (any, []model.ProviderStatus, []string, bool, error) {
	start := time.Now()
	if nativeAssetPattern.MatchString(strings.TrimSpace(assetInput)) {
		status, err := s.chainProvider.ChainStatus(ctx)
		providerStatus := []model.ProviderStatus{{
			Name:      "rpc",
			Status:    statusFromErr(err),
			LatencyMS: time.Since(start).Milliseconds(),
		}}
		if err != nil {
			return nil, providerStatus, nil, false, err
		}
		baseUnits, amountDecimal, err := id.NormalizeAmount("", amountInput, 18)
		if err != nil {
			return nil, providerStatus, nil, false, err
		}
		gasPrice, ok := new(big.Int).SetString(strings.TrimSpace(status.GasPrice), 10)
		if !ok {
			return nil, providerStatus, nil, false, clierr.New(clierr.CodeUnavailable, "invalid gas price returned by rpc")
		}
		gasEstimate := big.NewInt(21000)
		feeWei := new(big.Int).Mul(gasEstimate, gasPrice)
		return model.TransferSimulation{
			Kind:  "native",
			From:  strings.ToLower(from),
			To:    strings.ToLower(to),
			Asset: "MNT",
			Amount: model.AmountInfo{
				AmountBaseUnits: baseUnits,
				AmountDecimal:   amountDecimal,
				Decimals:        18,
			},
			GasEstimate: gasEstimate.String(),
			FeeMNT:      id.FormatDecimalCompat(feeWei.String(), 18),
			Success:     true,
			Source:      "intrinsic:native-transfer + rpc chain.status",
			FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		}, providerStatus, nil, false, nil
	}

	chain, err := id.ParseChain(s.settings.Network)
	if err != nil {
		return nil, nil, nil, false, err
	}
	asset, err := id.ParseAsset(assetInput, chain)
	if err != nil {
		return nil, nil, nil, false, err
	}
	baseUnits, amountDecimal, err := id.NormalizeAmount("", amountInput, asset.Decimals)
	if err != nil {
		return nil, nil, nil, false, err
	}
	call, err := s.chainProvider.SimulateCall(ctx, providers.ContractCallRequest{
		From:     from,
		To:       asset.Address,
		Function: "transfer(address,uint256)",
		Args:     []string{to, baseUnits},
		Value:    "",
	})
	providerStatus := []model.ProviderStatus{{
		Name:      "rpc",
		Status:    statusFromErr(err),
		LatencyMS: time.Since(start).Milliseconds(),
	}}
	if err != nil {
		return nil, providerStatus, nil, false, err
	}
	return model.TransferSimulation{
		Kind:         "erc20",
		From:         strings.ToLower(from),
		To:           strings.ToLower(to),
		Asset:        asset.Symbol,
		TokenAddress: asset.Address,
		Amount: model.AmountInfo{
			AmountBaseUnits: baseUnits,
			AmountDecimal:   amountDecimal,
			Decimals:        asset.Decimals,
		},
		GasEstimate: call.GasEstimate,
		FeeMNT:      call.FeeMNT,
		Success:     call.Success,
		Error:       call.Error,
		Source:      "onchain:eth_estimateGas+eth_call erc20.transfer",
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}, providerStatus, nil, false, nil
}

func (s *runtimeState) newProvidersDoctorCommand() *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run provider diagnostics (availability, latency, last success/failure)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), s.settings.Timeout)
			defer cancel()

			data, providerStatus, warnings, partial, err := s.runProvidersDoctor(ctx, provider)
			if err != nil {
				s.captureCommandDiagnostics(warnings, providerStatus, partial)
				return err
			}
			s.captureCommandDiagnostics(warnings, providerStatus, partial)
			return s.emitSuccess(trimRootPath(cmd.CommandPath()), data, warnings, cacheMetaBypass(), providerStatus, partial)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "all", "Provider name or all")
	return cmd
}

func (s *runtimeState) runProvidersDoctor(ctx context.Context, providerOpt string) ([]model.ProviderDoctorStatus, []model.ProviderStatus, []string, bool, error) {
	selected := strings.ToLower(strings.TrimSpace(providerOpt))
	if selected == "" {
		selected = "all"
	}

	checks, err := s.providerDoctorChecks(selected)
	if err != nil {
		return nil, nil, nil, false, err
	}

	history := s.loadProvidersDoctorHistory()
	results := make([]model.ProviderDoctorStatus, 0, len(checks))
	providerStatus := make([]model.ProviderStatus, 0, len(checks))
	warnings := []string{}
	partial := false

	tasks := make([]collectTask[model.ProviderDoctorStatus], 0, len(checks))
	for _, check := range checks {
		current := check
		if !current.enabled {
			checkedAt := time.Now().UTC().Format(time.RFC3339)
			entry := history.Entries[current.name]
			status := model.ProviderDoctorStatus{
				Name:          current.name,
				Enabled:       false,
				Available:     false,
				Status:        "disabled",
				LatencyMS:     0,
				LastSuccessAt: entry.LastSuccessAt,
				LastFailureAt: entry.LastFailureAt,
				FailureReason: entry.FailureReason,
				CheckedAt:     checkedAt,
			}
			results = append(results, status)
			providerStatus = append(providerStatus, model.ProviderStatus{Name: current.name, Status: "error", LatencyMS: 0})
			continue
		}
		tasks = append(tasks, collectTask[model.ProviderDoctorStatus]{
			name: current.name,
			run: func(ctx context.Context) ([]model.ProviderDoctorStatus, error) {
				start := time.Now()
				err := current.run(ctx)
				statusLabel, available, reason := providerDoctorOutcome(err)
				checkedAt := time.Now().UTC().Format(time.RFC3339)
				entry := history.Entries[current.name]

				return []model.ProviderDoctorStatus{{
					Name:          current.name,
					Enabled:       true,
					Available:     available,
					Status:        statusLabel,
					LatencyMS:     time.Since(start).Milliseconds(),
					LastSuccessAt: entry.LastSuccessAt,
					LastFailureAt: entry.LastFailureAt,
					FailureReason: reason,
					CheckedAt:     checkedAt,
				}}, err
			},
		})
	}

	for _, taskResult := range runParallelCollectors(ctx, tasks) {
		if len(taskResult.items) == 0 {
			continue
		}
		item := taskResult.items[0]
		entry := history.Entries[item.Name]
		if item.Available {
			entry.LastSuccessAt = item.CheckedAt
		} else {
			entry.LastFailureAt = item.CheckedAt
			entry.FailureReason = item.FailureReason
		}
		item.LastSuccessAt = entry.LastSuccessAt
		item.LastFailureAt = entry.LastFailureAt
		item.FailureReason = entry.FailureReason
		history.Entries[item.Name] = entry

		results = append(results, item)
		providerStatus = append(providerStatus, model.ProviderStatus{
			Name:      taskResult.name,
			Status:    item.Status,
			LatencyMS: item.LatencyMS,
		})
		if taskResult.err != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("%s health check failed: %v", taskResult.name, taskResult.err))
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	sort.SliceStable(providerStatus, func(i, j int) bool {
		return providerStatus[i].Name < providerStatus[j].Name
	})
	s.saveProvidersDoctorHistory(history)
	return results, providerStatus, warnings, partial, nil
}

type providerDoctorCheck struct {
	name    string
	enabled bool
	run     func(context.Context) error
}

func (s *runtimeState) providerDoctorChecks(selected string) ([]providerDoctorCheck, error) {
	all := []string{"rpc", "agni", "merchant_moe", "lendle", "aurelius", "aave_v3", "meth", "mantle_bridge", "across", "pendle", "defillama"}
	valid := map[string]bool{}
	for _, name := range all {
		valid[name] = true
	}
	if selected != "all" && !valid[selected] {
		return nil, clierr.New(clierr.CodeUsage, "provider must be one of: all,rpc,agni,merchant_moe,lendle,aurelius,aave_v3,meth,mantle_bridge,across,pendle,defillama")
	}

	fromChain, _ := id.ParseChain("eip155:1")
	toChain, _ := id.ParseChain(defaultMantleChainID(s.settings.Network))
	bridgeAsset, _ := parseBridgeAsset("USDC", toChain)
	bridgeReq := providers.BridgeQuoteRequest{
		FromChain:     fromChain,
		ToChain:       toChain,
		Asset:         bridgeAsset,
		AmountDecimal: "1",
	}

	items := []providerDoctorCheck{
		{
			name:    "rpc",
			enabled: s.settings.Providers["rpc"].Enabled,
			run: func(ctx context.Context) error {
				provider, err := rpc.New(rpc.Config{Network: s.settings.Network, RPCURL: s.settings.RPCURL})
				if err != nil {
					return err
				}
				defer provider.Close()
				_, err = provider.ChainStatus(ctx)
				return err
			},
		},
		{
			name:    "agni",
			enabled: s.settings.Providers["agni"].Enabled,
			run: func(ctx context.Context) error {
				provider, err := s.newAgniProvider()
				if err != nil {
					return err
				}
				closeIfPossible(provider)
				return nil
			},
		},
		{
			name:    "merchant_moe",
			enabled: s.settings.Providers["merchant_moe"].Enabled,
			run: func(ctx context.Context) error {
				provider, err := s.newMerchantMoeProvider()
				if err != nil {
					return err
				}
				closeIfPossible(provider)
				return nil
			},
		},
		{
			name:    "lendle",
			enabled: s.settings.Providers["lendle"].Enabled,
			run: func(ctx context.Context) error {
				provider := lendle.New(lendle.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
				_, err := provider.LendRates(ctx, id.Asset{})
				return err
			},
		},
		{
			name:    "aurelius",
			enabled: s.settings.Providers["aurelius"].Enabled,
			run: func(ctx context.Context) error {
				provider := aurelius.New(aurelius.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
				_, err := provider.LendRates(ctx, id.Asset{})
				return err
			},
		},
		{
			name:    "aave_v3",
			enabled: s.settings.Providers["aave_v3"].Enabled,
			run: func(ctx context.Context) error {
				provider := aavev3.New(aavev3.Config{Network: s.settings.Network, RPCURL: s.settings.RPCURL})
				_, err := provider.LendRates(ctx, id.Asset{Symbol: "USDC"})
				return err
			},
		},
		{
			name:    "meth",
			enabled: s.settings.Providers["meth"].Enabled,
			run: func(ctx context.Context) error {
				provider, err := s.newStakingProvider()
				if err != nil {
					return err
				}
				_, err = provider.StakeInfo(ctx)
				return err
			},
		},
		{
			name:    "mantle_bridge",
			enabled: s.settings.Providers["mantle_bridge"].Enabled,
			run: func(ctx context.Context) error {
				provider := mantlebridge.New(mantlebridge.Config{Network: s.settings.Network})
				_, err := provider.QuoteBridge(ctx, bridgeReq)
				return err
			},
		},
		{
			name:    "across",
			enabled: s.settings.Providers["across"].Enabled,
			run: func(ctx context.Context) error {
				provider := s.newAcrossProvider()
				_, err := provider.QuoteBridge(ctx, bridgeReq)
				return err
			},
		},
		{
			name:    "pendle",
			enabled: s.settings.Providers["pendle"].Enabled,
			run: func(ctx context.Context) error {
				provider := pendle.New(pendle.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
				_, err := provider.YieldOpportunities(ctx, providers.YieldRequest{Limit: 1})
				return err
			},
		},
		{
			name:    "defillama",
			enabled: s.settings.Providers["defillama"].Enabled,
			run: func(ctx context.Context) error {
				provider := defillama.New(defillama.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
				_, err := provider.FetchPools(ctx)
				return err
			},
		},
	}

	if selected == "all" {
		return items, nil
	}
	for _, item := range items {
		if item.name == selected {
			return []providerDoctorCheck{item}, nil
		}
	}
	return nil, clierr.New(clierr.CodeUsage, "provider not found")
}

func providerDoctorOutcome(err error) (status string, available bool, reason string) {
	if err == nil {
		return "ok", true, ""
	}
	reason = err.Error()
	if typed, ok := clierr.As(err); ok {
		switch typed.Code {
		case clierr.CodeAuth:
			return "auth_error", false, reason
		case clierr.CodeRateLimited:
			return "rate_limited", false, reason
		case clierr.CodeUnavailable:
			return "unavailable", false, reason
		case clierr.CodeUnsupported:
			return "unsupported", false, reason
		case clierr.CodeUsage:
			return "usage_error", false, reason
		}
	}
	return "error", false, reason
}

func (s *runtimeState) loadProvidersDoctorHistory() providersDoctorHistory {
	history := providersDoctorHistory{Entries: map[string]providersDoctorHistoryEntry{}}
	if s.cache == nil {
		return history
	}
	res, err := s.cache.Get(providersDoctorHistoryKey, -1)
	if err != nil || !res.Hit {
		return history
	}
	if err := json.Unmarshal(res.Value, &history); err != nil {
		return providersDoctorHistory{Entries: map[string]providersDoctorHistoryEntry{}}
	}
	if history.Entries == nil {
		history.Entries = map[string]providersDoctorHistoryEntry{}
	}
	return history
}

func (s *runtimeState) saveProvidersDoctorHistory(history providersDoctorHistory) {
	if s.cache == nil {
		return
	}
	payload, err := json.Marshal(history)
	if err != nil {
		return
	}
	_ = s.cache.Set(providersDoctorHistoryKey, payload, providersDoctorHistoryTTL)
}

func (s *runtimeState) newSwapCommand() *cobra.Command {
	var from string
	var to string
	var amount string
	var feeTier int
	var providerOpt string

	root := &cobra.Command{Use: "swap", Short: "Swap quote commands"}
	quote := &cobra.Command{
		Use:   "quote",
		Short: "Get DEX swap quote",
		RunE: func(cmd *cobra.Command, args []string) error {
			chain, err := id.ParseChain(s.settings.Network)
			if err != nil {
				return err
			}
			fromAsset, err := id.ParseAsset(from, chain)
			if err != nil {
				return err
			}
			toAsset, err := id.ParseAsset(to, chain)
			if err != nil {
				return err
			}
			baseUnits, amountDecimal, err := id.NormalizeAmount("", amount, fromAsset.Decimals)
			if err != nil {
				return err
			}
			req := providers.SwapQuoteRequest{
				FromAsset:       fromAsset,
				ToAsset:         toAsset,
				AmountBaseUnits: baseUnits,
				AmountDecimal:   amountDecimal,
				FeeTier:         feeTier,
			}
			normalizedProvider := strings.ToLower(strings.TrimSpace(providerOpt))
			if normalizedProvider == "" {
				normalizedProvider = "best"
			}
			key := cacheKey("swap quote", map[string]any{
				"network":    s.settings.Network,
				"from":       strings.ToLower(fromAsset.Address),
				"to":         strings.ToLower(toAsset.Address),
				"amount":     baseUnits,
				"feeTier":    feeTier,
				"provider":   normalizedProvider,
				"decimalsIn": fromAsset.Decimals,
			})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 15*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				return s.fetchSwapQuote(ctx, req, normalizedProvider)
			})
		},
	}
	quote.Flags().StringVar(&from, "from", "", "Input token symbol/address/CAIP-19")
	quote.Flags().StringVar(&to, "to", "", "Output token symbol/address/CAIP-19")
	quote.Flags().StringVar(&amount, "amount", "", "Input amount in decimal units")
	quote.Flags().IntVar(&feeTier, "fee-tier", 3000, "V3 fee tier (500, 3000, 10000)")
	quote.Flags().StringVar(&providerOpt, "provider", "best", "Quote provider: agni|merchant_moe|best")
	_ = quote.MarkFlagRequired("from")
	_ = quote.MarkFlagRequired("to")
	_ = quote.MarkFlagRequired("amount")
	root.AddCommand(quote)
	return root
}

func (s *runtimeState) newLendCommand() *cobra.Command {
	var asset string
	var protocol string

	root := &cobra.Command{Use: "lend", Short: "Lending protocol commands"}
	markets := &cobra.Command{
		Use:   "markets",
		Short: "Get lending markets",
		RunE: func(cmd *cobra.Command, args []string) error {
			assetReq, err := s.parseOptionalAsset(asset)
			if err != nil {
				return err
			}
			key := cacheKey("lend markets", map[string]any{
				"network":  s.settings.Network,
				"protocol": protocol,
				"asset":    strings.ToLower(assetReq.Address + "|" + assetReq.Symbol),
			})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 5*time.Minute, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				return s.fetchLendMarkets(ctx, protocol, assetReq)
			})
		},
	}
	markets.Flags().StringVar(&asset, "asset", "", "Optional asset symbol/address filter")
	markets.Flags().StringVar(&protocol, "protocol", "all", "Protocol: lendle|aurelius|aave_v3|all")

	rates := &cobra.Command{
		Use:   "rates",
		Short: "Get lending rates",
		RunE: func(cmd *cobra.Command, args []string) error {
			assetReq, err := s.parseOptionalAsset(asset)
			if err != nil {
				return err
			}
			key := cacheKey("lend rates", map[string]any{
				"network":  s.settings.Network,
				"protocol": protocol,
				"asset":    strings.ToLower(assetReq.Address + "|" + assetReq.Symbol),
			})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 30*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				return s.fetchLendRates(ctx, protocol, assetReq)
			})
		},
	}
	rates.Flags().StringVar(&asset, "asset", "", "Optional asset symbol/address filter")
	rates.Flags().StringVar(&protocol, "protocol", "all", "Protocol: lendle|aurelius|aave_v3|all")

	root.AddCommand(markets, rates)
	return root
}

func (s *runtimeState) newStakeCommand() *cobra.Command {
	var action string
	var amount string

	root := &cobra.Command{Use: "stake", Short: "Staking commands"}
	info := &cobra.Command{
		Use:   "info",
		Short: "Get mETH staking info",
		RunE: func(cmd *cobra.Command, args []string) error {
			key := cacheKey("stake info", map[string]any{"network": s.settings.Network})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, time.Minute, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				provider, err := s.newStakingProvider()
				if err != nil {
					return nil, nil, nil, false, err
				}
				start := time.Now()
				data, err := provider.StakeInfo(ctx)
				status := model.ProviderStatus{Name: "meth", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				return data, []model.ProviderStatus{status}, nil, false, err
			})
		},
	}

	quote := &cobra.Command{
		Use:   "quote",
		Short: "Get stake/unstake quote",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := providers.StakeQuoteRequest{Action: action, AmountDecimal: amount}
			key := cacheKey("stake quote", map[string]any{
				"network": s.settings.Network,
				"action":  strings.ToLower(strings.TrimSpace(action)),
				"amount":  strings.TrimSpace(amount),
			})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 30*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				provider, err := s.newStakingProvider()
				if err != nil {
					return nil, nil, nil, false, err
				}
				start := time.Now()
				data, err := provider.StakeQuote(ctx, req)
				status := model.ProviderStatus{Name: "meth", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}
				return data, []model.ProviderStatus{status}, nil, false, err
			})
		},
	}
	quote.Flags().StringVar(&action, "action", "", "Action: stake|unstake")
	quote.Flags().StringVar(&amount, "amount", "", "Amount in decimal units")
	_ = quote.MarkFlagRequired("action")
	_ = quote.MarkFlagRequired("amount")

	root.AddCommand(info, quote)
	return root
}

func (s *runtimeState) newYieldCommand() *cobra.Command {
	var asset string
	var minAPY float64
	var limit int
	var sortBy string

	root := &cobra.Command{Use: "yield", Short: "Yield opportunity commands"}
	opportunities := &cobra.Command{
		Use:   "opportunities",
		Short: "Get ranked yield opportunities",
		RunE: func(cmd *cobra.Command, args []string) error {
			assetReq, err := s.parseOptionalAsset(asset)
			if err != nil {
				return err
			}
			key := cacheKey("yield opportunities", map[string]any{
				"network": s.settings.Network,
				"asset":   strings.ToLower(assetReq.Address + "|" + assetReq.Symbol),
				"minApy":  minAPY,
				"limit":   limit,
				"sortBy":  sortBy,
			})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 5*time.Minute, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				return s.fetchYieldOpportunities(ctx, assetReq, minAPY, limit, sortBy)
			})
		},
	}
	opportunities.Flags().StringVar(&asset, "asset", "", "Optional asset symbol/address filter")
	opportunities.Flags().Float64Var(&minAPY, "min-apy", 0, "Minimum APY")
	opportunities.Flags().IntVar(&limit, "limit", 20, "Maximum number of items")
	opportunities.Flags().StringVar(&sortBy, "sort-by", "score", "Sort field: score|apy|tvl")
	root.AddCommand(opportunities)
	return root
}

func (s *runtimeState) newBridgeCommand() *cobra.Command {
	var fromChain string
	var toChain string
	var asset string
	var amount string
	var providerOpt string
	var direction string

	root := &cobra.Command{Use: "bridge", Short: "Bridge commands"}
	quote := &cobra.Command{
		Use:   "quote",
		Short: "Get bridge quote",
		RunE: func(cmd *cobra.Command, args []string) error {
			toChainInput := strings.TrimSpace(toChain)
			if toChainInput == "" {
				toChainInput = defaultMantleChainID(s.settings.Network)
			}
			fromParsed, err := id.ParseChain(fromChain)
			if err != nil {
				return err
			}
			toParsed, err := id.ParseChain(toChainInput)
			if err != nil {
				return err
			}
			assetParsed, err := parseBridgeAsset(asset, toParsed)
			if err != nil {
				return err
			}
			req := providers.BridgeQuoteRequest{
				FromChain:     fromParsed,
				ToChain:       toParsed,
				Asset:         assetParsed,
				AmountDecimal: amount,
			}
			normalizedProvider := strings.ToLower(strings.TrimSpace(providerOpt))
			if normalizedProvider == "" {
				normalizedProvider = "best"
			}
			key := cacheKey("bridge quote", map[string]any{
				"network":  s.settings.Network,
				"from":     fromParsed.CAIP2,
				"to":       toParsed.CAIP2,
				"asset":    strings.ToLower(assetParsed.Address + "|" + assetParsed.Symbol),
				"amount":   amount,
				"provider": normalizedProvider,
			})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 30*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				return s.fetchBridgeQuote(ctx, req, normalizedProvider)
			})
		},
	}
	quote.Flags().StringVar(&fromChain, "from-chain", "eip155:1", "Source chain (e.g. eip155:1)")
	quote.Flags().StringVar(&toChain, "to-chain", "", "Destination chain (default: active Mantle network)")
	quote.Flags().StringVar(&asset, "asset", "", "Asset symbol or ERC-20 address")
	quote.Flags().StringVar(&amount, "amount", "", "Amount in decimal units")
	quote.Flags().StringVar(&providerOpt, "provider", "best", "Provider: official|across|best")
	_ = quote.MarkFlagRequired("asset")
	_ = quote.MarkFlagRequired("amount")

	status := &cobra.Command{
		Use:   "status <tx_hash>",
		Short: "Get bridge transaction status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			txHash := strings.TrimSpace(args[0])
			key := cacheKey("bridge status", map[string]any{
				"network":   s.settings.Network,
				"provider":  strings.ToLower(strings.TrimSpace(providerOpt)),
				"txHash":    strings.ToLower(txHash),
				"direction": strings.ToLower(strings.TrimSpace(direction)),
			})
			return s.runCachedCommand(trimRootPath(cmd.CommandPath()), key, 30*time.Second, func(ctx context.Context) (any, []model.ProviderStatus, []string, bool, error) {
				status, providersStatus, warnings, partial, err := s.fetchBridgeStatus(ctx, txHash, providerOpt)
				if err != nil {
					return nil, providersStatus, warnings, partial, err
				}
				status.Direction = strings.ToLower(strings.TrimSpace(direction))
				return status, providersStatus, warnings, partial, nil
			})
		},
	}
	status.Flags().StringVar(&providerOpt, "provider", "official", "Provider: official")
	status.Flags().StringVar(&direction, "direction", "withdrawal", "Direction: deposit|withdrawal")

	root.AddCommand(quote, status)
	return root
}

func (s *runtimeState) fetchSwapQuote(ctx context.Context, req providers.SwapQuoteRequest, providerOpt string) (any, []model.ProviderStatus, []string, bool, error) {
	providerOpt = strings.ToLower(strings.TrimSpace(providerOpt))
	switch providerOpt {
	case "agni":
		provider, err := s.newAgniProvider()
		if err != nil {
			return nil, nil, nil, false, err
		}
		defer closeIfPossible(provider)
		start := time.Now()
		quote, err := provider.QuoteSwap(ctx, req)
		return quote, []model.ProviderStatus{{Name: "agni", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}}, nil, false, err
	case "merchant_moe":
		provider, err := s.newMerchantMoeProvider()
		if err != nil {
			return nil, nil, nil, false, err
		}
		defer closeIfPossible(provider)
		start := time.Now()
		quote, err := provider.QuoteSwap(ctx, req)
		return quote, []model.ProviderStatus{{Name: "merchant_moe", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}}, nil, false, err
	case "best":
		return s.fetchBestSwapQuote(ctx, req)
	default:
		return nil, nil, nil, false, clierr.New(clierr.CodeUsage, "provider must be agni, merchant_moe, or best")
	}
}

func (s *runtimeState) fetchBestSwapQuote(ctx context.Context, req providers.SwapQuoteRequest) (any, []model.ProviderStatus, []string, bool, error) {
	statuses := []model.ProviderStatus{}
	warnings := []string{}
	quotes := []model.SwapQuote{}
	partial := false
	var firstErr error

	loaders := []struct {
		name string
		new  func() (providers.SwapProvider, error)
	}{
		{name: "agni", new: func() (providers.SwapProvider, error) { return s.newAgniProvider() }},
		{name: "merchant_moe", new: func() (providers.SwapProvider, error) { return s.newMerchantMoeProvider() }},
	}
	for _, loader := range loaders {
		start := time.Now()
		provider, err := loader.new()
		if err != nil {
			partial = true
			statuses = append(statuses, model.ProviderStatus{Name: loader.name, Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()})
			warnings = append(warnings, fmt.Sprintf("%s unavailable: %v", loader.name, err))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		quote, quoteErr := provider.QuoteSwap(ctx, req)
		closeIfPossible(provider)
		statuses = append(statuses, model.ProviderStatus{Name: loader.name, Status: statusFromErr(quoteErr), LatencyMS: time.Since(start).Milliseconds()})
		if quoteErr != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("%s quote failed: %v", loader.name, quoteErr))
			if firstErr == nil {
				firstErr = quoteErr
			}
			continue
		}
		quotes = append(quotes, quote)
	}
	if len(quotes) == 0 {
		if firstErr == nil {
			firstErr = clierr.New(clierr.CodeUnavailable, "no swap provider available")
		} else if typedErr, ok := clierr.As(firstErr); ok && typedErr.Code == clierr.CodeUnsupported {
			firstErr = clierr.New(clierr.CodeUnsupported, fmt.Sprintf("no swap quote providers configured for network %s", normalizeNetworkLabel(s.settings.Network)))
		}
		return nil, statuses, warnings, partial, firstErr
	}
	best := quotes[0]
	for i := 1; i < len(quotes); i++ {
		if compareAmountBaseUnits(quotes[i].EstimatedOut.AmountBaseUnits, best.EstimatedOut.AmountBaseUnits) > 0 {
			best = quotes[i]
		}
	}
	return best, statuses, warnings, partial, nil
}

func (s *runtimeState) fetchLendMarkets(ctx context.Context, protocol string, asset id.Asset) (any, []model.ProviderStatus, []string, bool, error) {
	entries, err := s.lendingProviderEntries(protocol)
	if err != nil {
		return nil, nil, nil, false, err
	}
	items := []model.LendMarket{}
	statuses := []model.ProviderStatus{}
	warnings := []string{}
	partial := false
	var firstErr error

	tasks := make([]collectTask[model.LendMarket], 0, len(entries))
	for _, entry := range entries {
		current := entry
		tasks = append(tasks, collectTask[model.LendMarket]{
			name: current.name,
			run: func(ctx context.Context) ([]model.LendMarket, error) {
				return current.provider.LendMarkets(ctx, asset)
			},
		})
	}

	for _, result := range runParallelCollectors(ctx, tasks) {
		statuses = append(statuses, model.ProviderStatus{Name: result.name, Status: statusFromErr(result.err), LatencyMS: result.latencyMS})
		if result.err != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("%s markets failed: %v", result.name, result.err))
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		items = append(items, result.items...)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].TVLUSD > items[j].TVLUSD })
	if len(items) == 0 {
		if firstErr == nil {
			firstErr = clierr.New(clierr.CodeUnavailable, "no lending markets available")
		}
		return nil, statuses, warnings, partial, firstErr
	}
	return items, statuses, warnings, partial, nil
}

func (s *runtimeState) fetchLendRates(ctx context.Context, protocol string, asset id.Asset) (any, []model.ProviderStatus, []string, bool, error) {
	entries, err := s.lendingProviderEntries(protocol)
	if err != nil {
		return nil, nil, nil, false, err
	}
	items := []model.LendRate{}
	statuses := []model.ProviderStatus{}
	warnings := []string{}
	partial := false
	var firstErr error

	tasks := make([]collectTask[model.LendRate], 0, len(entries))
	for _, entry := range entries {
		current := entry
		tasks = append(tasks, collectTask[model.LendRate]{
			name: current.name,
			run: func(ctx context.Context) ([]model.LendRate, error) {
				return current.provider.LendRates(ctx, asset)
			},
		})
	}

	for _, result := range runParallelCollectors(ctx, tasks) {
		statuses = append(statuses, model.ProviderStatus{Name: result.name, Status: statusFromErr(result.err), LatencyMS: result.latencyMS})
		if result.err != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("%s rates failed: %v", result.name, result.err))
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		items = append(items, result.items...)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].SupplyAPY > items[j].SupplyAPY })
	if len(items) == 0 {
		if firstErr == nil {
			firstErr = clierr.New(clierr.CodeUnavailable, "no lending rates available")
		}
		return nil, statuses, warnings, partial, firstErr
	}
	return items, statuses, warnings, partial, nil
}

func (s *runtimeState) fetchYieldOpportunities(ctx context.Context, asset id.Asset, minAPY float64, limit int, sortBy string) (any, []model.ProviderStatus, []string, bool, error) {
	statuses := []model.ProviderStatus{}
	warnings := []string{}
	items := []model.YieldOpportunity{}
	partial := false
	var firstErr error

	req := providers.YieldRequest{
		Asset:  asset,
		Limit:  limit,
		MinAPY: minAPY,
		SortBy: sortBy,
	}
	collectors := []yieldCollector{}
	if s.settings.Providers["pendle"].Enabled {
		provider := pendle.New(pendle.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
		collectors = append(collectors, yieldCollector{
			name: "pendle",
			run: func(ctx context.Context) ([]model.YieldOpportunity, error) {
				return provider.YieldOpportunities(ctx, req)
			},
			onErrFmt: "pendle opportunities failed: %v",
		})
	}
	if s.settings.Providers["lendle"].Enabled {
		provider := lendle.New(lendle.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
		collectors = append(collectors, yieldCollector{
			name: "lendle",
			run: func(ctx context.Context) ([]model.YieldOpportunity, error) {
				rates, err := provider.LendRates(ctx, asset)
				if err != nil {
					return nil, err
				}
				return lendRatesToYield("lendle", rates), nil
			},
			onErrFmt: "lendle rates failed: %v",
		})
	}
	if s.settings.Providers["aurelius"].Enabled {
		provider := aurelius.New(aurelius.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
		collectors = append(collectors, yieldCollector{
			name: "aurelius",
			run: func(ctx context.Context) ([]model.YieldOpportunity, error) {
				rates, err := provider.LendRates(ctx, asset)
				if err != nil {
					return nil, err
				}
				return lendRatesToYield("aurelius", rates), nil
			},
			onErrFmt: "aurelius rates failed: %v",
		})
	}
	if s.settings.Providers["aave_v3"].Enabled {
		provider := aavev3.New(aavev3.Config{Network: s.settings.Network, RPCURL: s.settings.RPCURL})
		collectors = append(collectors, yieldCollector{
			name: "aave_v3",
			run: func(ctx context.Context) ([]model.YieldOpportunity, error) {
				rates, err := provider.LendRates(ctx, asset)
				if err != nil {
					return nil, err
				}
				return lendRatesToYield("aave_v3", rates), nil
			},
			onErrFmt: "aave_v3 rates failed: %v",
		})
	}

	for _, result := range runYieldCollectors(ctx, collectors) {
		statuses = append(statuses, model.ProviderStatus{
			Name:      result.name,
			Status:    statusFromErr(result.err),
			LatencyMS: result.latencyMS,
		})
		if result.err != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf(result.onErrFmt, result.err))
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		items = append(items, result.items...)
	}

	filtered := make([]model.YieldOpportunity, 0, len(items))
	for _, item := range items {
		if item.APYTotal < minAPY {
			continue
		}
		if strings.TrimSpace(asset.Symbol) != "" && !strings.EqualFold(asset.Symbol, item.Asset) {
			continue
		}
		filtered = append(filtered, item)
	}
	sortYield(filtered, sortBy)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	if len(filtered) == 0 {
		if firstErr == nil {
			firstErr = clierr.New(clierr.CodeUnavailable, "no yield opportunities available")
		}
		return nil, statuses, warnings, partial, firstErr
	}
	return filtered, statuses, warnings, partial, nil
}

type yieldCollector struct {
	name     string
	run      func(context.Context) ([]model.YieldOpportunity, error)
	onErrFmt string
}

type yieldCollectorResult struct {
	name      string
	items     []model.YieldOpportunity
	err       error
	latencyMS int64
	onErrFmt  string
}

func runYieldCollectors(ctx context.Context, collectors []yieldCollector) []yieldCollectorResult {
	wrapped := make([]collectTask[model.YieldOpportunity], 0, len(collectors))
	for _, collector := range collectors {
		wrapped = append(wrapped, collectTask[model.YieldOpportunity]{
			name:     collector.name,
			run:      collector.run,
			onErrFmt: collector.onErrFmt,
		})
	}
	genericResults := runParallelCollectors(ctx, wrapped)
	results := make([]yieldCollectorResult, len(genericResults))
	for i, result := range genericResults {
		results[i] = yieldCollectorResult{
			name:      result.name,
			items:     result.items,
			err:       result.err,
			latencyMS: result.latencyMS,
			onErrFmt:  result.onErrFmt,
		}
	}
	return results
}

type collectTask[T any] struct {
	name     string
	run      func(context.Context) ([]T, error)
	onErrFmt string
}

type collectResult[T any] struct {
	name      string
	items     []T
	err       error
	latencyMS int64
	onErrFmt  string
}

func runParallelCollectors[T any](ctx context.Context, tasks []collectTask[T]) []collectResult[T] {
	results := make([]collectResult[T], len(tasks))
	var wg sync.WaitGroup
	for i := range tasks {
		idx := i
		task := tasks[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			items, err := task.run(ctx)
			results[idx] = collectResult[T]{
				name:      task.name,
				items:     items,
				err:       err,
				latencyMS: time.Since(start).Milliseconds(),
				onErrFmt:  task.onErrFmt,
			}
		}()
	}
	wg.Wait()
	return results
}

func (s *runtimeState) fetchBridgeQuote(ctx context.Context, req providers.BridgeQuoteRequest, providerOpt string) (any, []model.ProviderStatus, []string, bool, error) {
	opt := strings.ToLower(strings.TrimSpace(providerOpt))
	switch opt {
	case "official":
		provider := mantlebridge.New(mantlebridge.Config{Network: s.settings.Network})
		start := time.Now()
		data, err := provider.QuoteBridge(ctx, req)
		return data, []model.ProviderStatus{{Name: "mantle_bridge", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}}, nil, false, err
	case "across":
		provider := s.newAcrossProvider()
		start := time.Now()
		data, err := provider.QuoteBridge(ctx, req)
		return data, []model.ProviderStatus{{Name: "across", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}}, nil, false, err
	case "best":
		statuses := []model.ProviderStatus{}
		warnings := []string{}
		partial := false
		quotes := []model.BridgeQuote{}
		var firstErr error

		official := mantlebridge.New(mantlebridge.Config{Network: s.settings.Network})
		startOfficial := time.Now()
		qOfficial, errOfficial := official.QuoteBridge(ctx, req)
		statuses = append(statuses, model.ProviderStatus{Name: "mantle_bridge", Status: statusFromErr(errOfficial), LatencyMS: time.Since(startOfficial).Milliseconds()})
		if errOfficial != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("official bridge quote failed: %v", errOfficial))
			firstErr = errOfficial
		} else {
			quotes = append(quotes, qOfficial)
		}

		if s.settings.Providers["across"].Enabled {
			acrossProvider := s.newAcrossProvider()
			startAcross := time.Now()
			qAcross, errAcross := acrossProvider.QuoteBridge(ctx, req)
			statuses = append(statuses, model.ProviderStatus{Name: "across", Status: statusFromErr(errAcross), LatencyMS: time.Since(startAcross).Milliseconds()})
			if errAcross != nil {
				partial = true
				warnings = append(warnings, fmt.Sprintf("across quote failed: %v", errAcross))
				if firstErr == nil {
					firstErr = errAcross
				}
			} else {
				quotes = append(quotes, qAcross)
			}
		}
		if len(quotes) == 0 {
			if firstErr == nil {
				firstErr = clierr.New(clierr.CodeUnavailable, "no bridge provider available")
			}
			return nil, statuses, warnings, partial, firstErr
		}
		best := quotes[0]
		for i := 1; i < len(quotes); i++ {
			next := quotes[i]
			if compareBridgeQuote(next, best) > 0 {
				best = next
			}
		}
		return best, statuses, warnings, partial, nil
	default:
		return nil, nil, nil, false, clierr.New(clierr.CodeUsage, "provider must be official, across, or best")
	}
}

func (s *runtimeState) fetchBridgeStatus(ctx context.Context, txHash, providerOpt string) (model.BridgeStatus, []model.ProviderStatus, []string, bool, error) {
	opt := strings.ToLower(strings.TrimSpace(providerOpt))
	if opt == "" {
		opt = "official"
	}
	switch opt {
	case "official":
		provider := mantlebridge.New(mantlebridge.Config{Network: s.settings.Network})
		start := time.Now()
		status, err := provider.BridgeStatus(ctx, txHash)
		return status, []model.ProviderStatus{{Name: "mantle_bridge", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()}}, nil, false, err
	default:
		err := clierr.New(clierr.CodeUnsupported, "bridge status currently supports provider=official only")
		return model.BridgeStatus{}, []model.ProviderStatus{{Name: opt, Status: statusFromErr(err), LatencyMS: 0}}, nil, false, err
	}
}

func (s *runtimeState) parseOptionalAsset(input string) (id.Asset, error) {
	if strings.TrimSpace(input) == "" {
		return id.Asset{}, nil
	}
	chain, err := id.ParseChain(s.settings.Network)
	if err != nil {
		return id.Asset{}, err
	}
	return id.ParseAsset(input, chain)
}

func (s *runtimeState) newAgniProvider() (*agni.Provider, error) {
	if !s.settings.Providers["agni"].Enabled {
		return nil, clierr.New(clierr.CodeUnsupported, "agni provider is disabled")
	}
	return agni.New(agni.Config{Network: s.settings.Network, RPCURL: s.settings.RPCURL})
}

func (s *runtimeState) newMerchantMoeProvider() (*merchantmoe.Provider, error) {
	if !s.settings.Providers["merchant_moe"].Enabled {
		return nil, clierr.New(clierr.CodeUnsupported, "merchant_moe provider is disabled")
	}
	return merchantmoe.New(merchantmoe.Config{Network: s.settings.Network, RPCURL: s.settings.RPCURL})
}

func (s *runtimeState) lendingProviderEntries(protocol string) ([]struct {
	name     string
	provider providers.LendingProvider
}, error) {
	items := []struct {
		name     string
		provider providers.LendingProvider
	}{}
	opt := strings.ToLower(strings.TrimSpace(protocol))
	if opt == "" {
		opt = "all"
	}
	switch opt {
	case "lendle", "all":
		if s.settings.Providers["lendle"].Enabled {
			items = append(items, struct {
				name     string
				provider providers.LendingProvider
			}{name: "lendle", provider: lendle.New(lendle.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})})
		}
	}
	switch opt {
	case "aurelius", "all":
		if s.settings.Providers["aurelius"].Enabled {
			items = append(items, struct {
				name     string
				provider providers.LendingProvider
			}{name: "aurelius", provider: aurelius.New(aurelius.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})})
		}
	}
	switch opt {
	case "aave_v3", "aave", "all":
		if s.settings.Providers["aave_v3"].Enabled {
			items = append(items, struct {
				name     string
				provider providers.LendingProvider
			}{name: "aave_v3", provider: aavev3.New(aavev3.Config{Network: s.settings.Network, RPCURL: s.settings.RPCURL})})
		}
	}
	if opt != "all" && opt != "lendle" && opt != "aurelius" && opt != "aave_v3" && opt != "aave" {
		return nil, clierr.New(clierr.CodeUsage, "protocol must be lendle, aurelius, aave_v3, or all")
	}
	if len(items) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "no enabled lending providers for selected protocol")
	}
	return items, nil
}

func (s *runtimeState) newStakingProvider() (*meth.Provider, error) {
	if !s.settings.Providers["meth"].Enabled {
		return nil, clierr.New(clierr.CodeUnsupported, "meth provider is disabled")
	}
	return meth.New(meth.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries}), nil
}

func (s *runtimeState) newAcrossProvider() *across.Provider {
	return across.New(across.Config{
		APIKey:  s.settings.AcrossAPIKey,
		Timeout: s.settings.Timeout,
		Retries: s.settings.Retries,
	})
}

func parseBridgeAsset(input string, chain id.Chain) (id.Asset, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return id.Asset{}, clierr.New(clierr.CodeUsage, "asset is required")
	}
	if token, ok := id.ResolveTokenSymbol(chain.CAIP2, trimmed); ok {
		return id.Asset{
			ChainID:  chain.CAIP2,
			AssetID:  fmt.Sprintf("%s/erc20:%s", chain.CAIP2, strings.ToLower(token.Address)),
			Address:  strings.ToLower(token.Address),
			Symbol:   token.Symbol,
			Decimals: token.Decimals,
		}, nil
	}
	if token, ok := id.ResolveTokenSymbol("eip155:5000", trimmed); ok {
		return id.Asset{
			ChainID:  chain.CAIP2,
			AssetID:  fmt.Sprintf("%s/erc20:%s", chain.CAIP2, strings.ToLower(token.Address)),
			Address:  strings.ToLower(token.Address),
			Symbol:   token.Symbol,
			Decimals: token.Decimals,
		}, nil
	}
	if hexAddressPattern.MatchString(trimmed) {
		return id.Asset{
			ChainID:  chain.CAIP2,
			AssetID:  fmt.Sprintf("%s/erc20:%s", chain.CAIP2, strings.ToLower(trimmed)),
			Address:  strings.ToLower(trimmed),
			Symbol:   strings.ToUpper(trimmed[:6]),
			Decimals: 18,
		}, nil
	}
	symbol := strings.ToUpper(trimmed)
	switch symbol {
	case "ETH", "MNT":
		return id.Asset{
			ChainID:  chain.CAIP2,
			AssetID:  fmt.Sprintf("%s/native:%s", chain.CAIP2, strings.ToLower(symbol)),
			Address:  "",
			Symbol:   symbol,
			Decimals: 18,
		}, nil
	case "USDC", "USDT":
		if token, ok := id.ResolveTokenSymbol("eip155:5000", symbol); ok {
			return id.Asset{
				ChainID:  chain.CAIP2,
				AssetID:  fmt.Sprintf("%s/erc20:%s", chain.CAIP2, strings.ToLower(token.Address)),
				Address:  strings.ToLower(token.Address),
				Symbol:   token.Symbol,
				Decimals: token.Decimals,
			}, nil
		}
	}
	return id.Asset{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("unsupported bridge asset: %s", input))
}

func defaultMantleChainID(network string) string {
	if strings.EqualFold(strings.TrimSpace(network), "sepolia") {
		return "eip155:5003"
	}
	return "eip155:5000"
}

func closeIfPossible(v any) {
	if closer, ok := v.(interface{ Close() }); ok {
		closer.Close()
	}
}

func compareAmountBaseUnits(left, right string) int {
	l, lok := new(big.Int).SetString(strings.TrimSpace(left), 10)
	r, rok := new(big.Int).SetString(strings.TrimSpace(right), 10)
	if !lok && !rok {
		return 0
	}
	if !lok {
		return -1
	}
	if !rok {
		return 1
	}
	return l.Cmp(r)
}

func normalizeNetworkLabel(network string) string {
	norm := strings.ToLower(strings.TrimSpace(network))
	if norm == "" {
		return "mainnet"
	}
	return norm
}

func lendRatesToYield(protocol string, rates []model.LendRate) []model.YieldOpportunity {
	items := make([]model.YieldOpportunity, 0, len(rates))
	for _, rate := range rates {
		apy := rate.SupplyAPY
		items = append(items, model.YieldOpportunity{
			Protocol:  protocol,
			Asset:     strings.ToUpper(strings.TrimSpace(rate.Asset)),
			Type:      "lending",
			APYBase:   apy,
			APYReward: 0,
			APYTotal:  apy,
			TVLUSD:    0,
			RiskLevel: riskFromAPY(apy),
			Score:     score(apy, 0),
			FetchedAt: rate.FetchedAt,
			Source:    rate.Source,
		})
	}
	return items
}

func sortYield(items []model.YieldOpportunity, sortBy string) {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "apy":
		sort.SliceStable(items, func(i, j int) bool { return items[i].APYTotal > items[j].APYTotal })
	case "tvl":
		sort.SliceStable(items, func(i, j int) bool { return items[i].TVLUSD > items[j].TVLUSD })
	default:
		sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	}
}

func riskFromAPY(apy float64) string {
	switch {
	case apy >= 30:
		return "high"
	case apy >= 12:
		return "medium"
	default:
		return "low"
	}
}

func score(apy, tvl float64) float64 {
	return apy*0.7 + tvl/1_000_000*0.3
}

func compareBridgeQuote(left, right model.BridgeQuote) int {
	if cmp := compareAmountBaseUnits(left.EstimatedOut.AmountBaseUnits, right.EstimatedOut.AmountBaseUnits); cmp != 0 {
		return cmp
	}
	if left.EstimatedFeeUSD < right.EstimatedFeeUSD {
		return 1
	}
	if left.EstimatedFeeUSD > right.EstimatedFeeUSD {
		return -1
	}
	if left.EstimatedTimeS < right.EstimatedTimeS {
		return 1
	}
	if left.EstimatedTimeS > right.EstimatedTimeS {
		return -1
	}
	return 0
}

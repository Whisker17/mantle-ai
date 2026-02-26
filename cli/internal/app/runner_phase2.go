package app

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/providers"
	"github.com/mantle/mantle-ai/cli/internal/providers/across"
	"github.com/mantle/mantle-ai/cli/internal/providers/agni"
	"github.com/mantle/mantle-ai/cli/internal/providers/aurelius"
	"github.com/mantle/mantle-ai/cli/internal/providers/lendle"
	"github.com/mantle/mantle-ai/cli/internal/providers/mantlebridge"
	"github.com/mantle/mantle-ai/cli/internal/providers/merchantmoe"
	"github.com/mantle/mantle-ai/cli/internal/providers/meth"
	"github.com/mantle/mantle-ai/cli/internal/providers/pendle"
	"github.com/spf13/cobra"
)

var hexAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

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
	markets.Flags().StringVar(&protocol, "protocol", "all", "Protocol: lendle|aurelius|all")

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
	rates.Flags().StringVar(&protocol, "protocol", "all", "Protocol: lendle|aurelius|all")

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

	for _, entry := range entries {
		start := time.Now()
		data, fetchErr := entry.provider.LendMarkets(ctx, asset)
		statuses = append(statuses, model.ProviderStatus{Name: entry.name, Status: statusFromErr(fetchErr), LatencyMS: time.Since(start).Milliseconds()})
		if fetchErr != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("%s markets failed: %v", entry.name, fetchErr))
			if firstErr == nil {
				firstErr = fetchErr
			}
			continue
		}
		items = append(items, data...)
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

	for _, entry := range entries {
		start := time.Now()
		data, fetchErr := entry.provider.LendRates(ctx, asset)
		statuses = append(statuses, model.ProviderStatus{Name: entry.name, Status: statusFromErr(fetchErr), LatencyMS: time.Since(start).Milliseconds()})
		if fetchErr != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("%s rates failed: %v", entry.name, fetchErr))
			if firstErr == nil {
				firstErr = fetchErr
			}
			continue
		}
		items = append(items, data...)
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
	if s.settings.Providers["pendle"].Enabled {
		provider := pendle.New(pendle.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
		start := time.Now()
		data, err := provider.YieldOpportunities(ctx, req)
		statuses = append(statuses, model.ProviderStatus{Name: "pendle", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()})
		if err != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("pendle opportunities failed: %v", err))
			if firstErr == nil {
				firstErr = err
			}
		} else {
			items = append(items, data...)
		}
	}

	if s.settings.Providers["lendle"].Enabled {
		provider := lendle.New(lendle.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
		start := time.Now()
		rates, err := provider.LendRates(ctx, asset)
		statuses = append(statuses, model.ProviderStatus{Name: "lendle", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()})
		if err != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("lendle rates failed: %v", err))
			if firstErr == nil {
				firstErr = err
			}
		} else {
			items = append(items, lendRatesToYield("lendle", rates)...)
		}
	}
	if s.settings.Providers["aurelius"].Enabled {
		provider := aurelius.New(aurelius.Config{Timeout: s.settings.Timeout, Retries: s.settings.Retries})
		start := time.Now()
		rates, err := provider.LendRates(ctx, asset)
		statuses = append(statuses, model.ProviderStatus{Name: "aurelius", Status: statusFromErr(err), LatencyMS: time.Since(start).Milliseconds()})
		if err != nil {
			partial = true
			warnings = append(warnings, fmt.Sprintf("aurelius rates failed: %v", err))
			if firstErr == nil {
				firstErr = err
			}
		} else {
			items = append(items, lendRatesToYield("aurelius", rates)...)
		}
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
	if opt != "all" && opt != "lendle" && opt != "aurelius" {
		return nil, clierr.New(clierr.CodeUsage, "protocol must be lendle, aurelius, or all")
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

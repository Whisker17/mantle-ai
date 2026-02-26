package mantlebridge

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/providers"
)

var txHashPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

type Config struct {
	Network string
}

type Provider struct {
	network string
}

func New(cfg Config) *Provider {
	return &Provider{network: strings.ToLower(strings.TrimSpace(cfg.Network))}
}

func (p *Provider) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:           "mantle_bridge",
		Type:           "bridge",
		Enabled:        true,
		RequiresKey:    false,
		AuthConfigured: true,
		Capabilities:   []string{"bridge.quote", "bridge.status"},
	}
}

func (p *Provider) QuoteBridge(_ context.Context, req providers.BridgeQuoteRequest) (model.BridgeQuote, error) {
	if strings.TrimSpace(req.AmountDecimal) == "" {
		return model.BridgeQuote{}, clierr.New(clierr.CodeUsage, "amount is required")
	}
	decimals := req.Asset.Decimals
	if decimals <= 0 {
		decimals = 18
	}
	baseUnits, decimal, err := id.NormalizeAmount("", req.AmountDecimal, decimals)
	if err != nil {
		return model.BridgeQuote{}, err
	}
	estimatedTime := int64(600)
	if isMantleToEthereum(req.FromChain, req.ToChain) {
		estimatedTime = 604800
	}

	return model.BridgeQuote{
		Provider:  "official",
		FromChain: req.FromChain.CAIP2,
		ToChain:   req.ToChain.CAIP2,
		Asset:     bridgeAssetLabel(req.Asset),
		InputAmount: model.AmountInfo{
			AmountBaseUnits: baseUnits,
			AmountDecimal:   decimal,
			Decimals:        decimals,
		},
		EstimatedOut: model.AmountInfo{
			AmountBaseUnits: baseUnits,
			AmountDecimal:   decimal,
			Decimals:        decimals,
		},
		EstimatedFeeUSD: 0,
		EstimatedTimeS:  estimatedTime,
		Route:           "official_bridge",
		FetchedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (p *Provider) BridgeStatus(_ context.Context, txHash string) (model.BridgeStatus, error) {
	hash := strings.TrimSpace(txHash)
	if !txHashPattern.MatchString(hash) {
		return model.BridgeStatus{}, clierr.New(clierr.CodeUsage, "invalid tx hash")
	}
	explorerBase := "https://explorer.mantle.xyz"
	if p.network == "sepolia" {
		explorerBase = "https://explorer.sepolia.mantle.xyz"
	}
	return model.BridgeStatus{
		TxHash:      strings.ToLower(hash),
		Direction:   "unknown",
		Status:      "pending",
		ExplorerURL: fmt.Sprintf("%s/tx/%s", explorerBase, strings.ToLower(hash)),
	}, nil
}

func isMantleToEthereum(from, to id.Chain) bool {
	return from.EVMChainID == 5000 && to.EVMChainID == 1
}

func bridgeAssetLabel(asset id.Asset) string {
	if strings.TrimSpace(asset.Symbol) != "" {
		return strings.ToUpper(strings.TrimSpace(asset.Symbol))
	}
	if strings.TrimSpace(asset.Address) != "" {
		return strings.ToLower(strings.TrimSpace(asset.Address))
	}
	return strings.TrimSpace(asset.AssetID)
}

var _ providers.BridgeProvider = (*Provider)(nil)
var _ providers.BridgeStatusProvider = (*Provider)(nil)

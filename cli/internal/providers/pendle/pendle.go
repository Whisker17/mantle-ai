package pendle

import (
	"context"
	"sort"
	"strings"
	"time"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/providers"
	"github.com/mantle/mantle-ai/cli/internal/providers/defillama"
)

type Config struct {
	BaseURL string
	Timeout time.Duration
	Retries int
}

type Provider struct {
	llama *defillama.Provider
}

func New(cfg Config) *Provider {
	return &Provider{
		llama: defillama.New(defillama.Config{
			BaseURL: cfg.BaseURL,
			Timeout: cfg.Timeout,
			Retries: cfg.Retries,
		}),
	}
}

func (p *Provider) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:           "pendle",
		Type:           "yield",
		Enabled:        true,
		RequiresKey:    false,
		AuthConfigured: true,
		Capabilities:   []string{"yield.opportunities"},
	}
}

func (p *Provider) YieldOpportunities(ctx context.Context, req providers.YieldRequest) ([]model.YieldOpportunity, error) {
	pools, err := p.llama.FetchPools(ctx)
	if err != nil {
		return nil, err
	}
	filtered := defillama.FilterPools(pools, "mantle", "pendle")
	out := make([]model.YieldOpportunity, 0, len(filtered))
	now := time.Now().UTC().Format(time.RFC3339)
	for _, pool := range filtered {
		if strings.TrimSpace(req.Asset.Symbol) != "" && !strings.EqualFold(req.Asset.Symbol, pool.Symbol) {
			continue
		}
		total := pool.APY
		if total < req.MinAPY {
			continue
		}
		item := model.YieldOpportunity{
			Protocol:  "pendle",
			Asset:     strings.ToUpper(strings.TrimSpace(pool.Symbol)),
			Type:      "yield-token",
			APYBase:   pool.APYBase,
			APYReward: pool.APYReward,
			APYTotal:  total,
			TVLUSD:    pool.TVLUSD,
			RiskLevel: riskFromAPY(total),
			Score:     score(total, pool.TVLUSD),
			FetchedAt: now,
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "pendle opportunities unavailable")
	}
	sortBy(out, req.SortBy)
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

func sortBy(items []model.YieldOpportunity, sortKey string) {
	switch strings.ToLower(strings.TrimSpace(sortKey)) {
	case "apy":
		sort.SliceStable(items, func(i, j int) bool { return items[i].APYTotal > items[j].APYTotal })
	case "tvl":
		sort.SliceStable(items, func(i, j int) bool { return items[i].TVLUSD > items[j].TVLUSD })
	default:
		sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	}
}

func score(apy, tvl float64) float64 {
	return apy*0.7 + tvl/1_000_000*0.3
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

var _ providers.YieldProvider = (*Provider)(nil)
var _ providers.Provider = (*Provider)(nil)

package aurelius

import (
	"context"
	"math"
	"strings"
	"time"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
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
		Name:           "aurelius",
		Type:           "lending",
		Enabled:        true,
		RequiresKey:    false,
		AuthConfigured: true,
		Capabilities:   []string{"lend.markets", "lend.rates"},
	}
}

func (p *Provider) LendMarkets(ctx context.Context, asset id.Asset) ([]model.LendMarket, error) {
	pools, err := p.llama.FetchPools(ctx)
	if err != nil {
		return nil, err
	}
	filtered := defillama.FilterPools(pools, "mantle", "aurelius")
	out := make([]model.LendMarket, 0, len(filtered))
	now := time.Now().UTC().Format(time.RFC3339)
	for _, pool := range filtered {
		if !defillama.MatchesAsset(pool, asset) {
			continue
		}
		supply := pool.APYBase
		if supply == 0 && pool.APY > 0 {
			supply = pool.APY
		}
		borrow := math.Max(pool.APY-supply, 0)
		out = append(out, model.LendMarket{
			Protocol:  "aurelius",
			Asset:     strings.ToUpper(strings.TrimSpace(pool.Symbol)),
			SupplyAPY: supply,
			BorrowAPY: borrow,
			TVLUSD:    pool.TVLUSD,
			FetchedAt: now,
			Source:    "defillama:/pools project=aurelius chain=mantle",
		})
	}
	if len(out) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "aurelius markets unavailable")
	}
	return out, nil
}

func (p *Provider) LendRates(ctx context.Context, asset id.Asset) ([]model.LendRate, error) {
	pools, err := p.llama.FetchPools(ctx)
	if err != nil {
		return nil, err
	}
	filtered := defillama.FilterPools(pools, "mantle", "aurelius")
	out := make([]model.LendRate, 0, len(filtered))
	now := time.Now().UTC().Format(time.RFC3339)
	for _, pool := range filtered {
		if !defillama.MatchesAsset(pool, asset) {
			continue
		}
		supply := pool.APYBase
		if supply == 0 && pool.APY > 0 {
			supply = pool.APY
		}
		borrow := math.Max(pool.APY-supply, 0)
		out = append(out, model.LendRate{
			Protocol:    "aurelius",
			Asset:       strings.ToUpper(strings.TrimSpace(pool.Symbol)),
			SupplyAPY:   supply,
			BorrowAPY:   borrow,
			Utilization: 0,
			FetchedAt:   now,
			Source:      "defillama:/pools project=aurelius chain=mantle",
		})
	}
	if len(out) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "aurelius rates unavailable")
	}
	return out, nil
}

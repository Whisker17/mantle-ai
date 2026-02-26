package defillama

import (
	"context"
	"net/http"
	"strings"
	"time"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/httpx"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
)

const defaultBaseURL = "https://yields.llama.fi"

type Config struct {
	BaseURL string
	Timeout time.Duration
	Retries int
}

type Provider struct {
	client  *httpx.Client
	baseURL string
}

type PoolsResponse struct {
	Status string `json:"status"`
	Data   []Pool `json:"data"`
}

type Pool struct {
	Pool             string   `json:"pool"`
	Chain            string   `json:"chain"`
	Project          string   `json:"project"`
	Symbol           string   `json:"symbol"`
	APY              float64  `json:"apy"`
	APYBase          float64  `json:"apyBase"`
	APYReward        float64  `json:"apyReward"`
	TVLUSD           float64  `json:"tvlUsd"`
	UnderlyingTokens []string `json:"underlyingTokens"`
}

func New(cfg Config) *Provider {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Provider{
		client:  httpx.New(timeout, cfg.Retries),
		baseURL: baseURL,
	}
}

func (p *Provider) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:           "defillama",
		Type:           "market-data",
		Enabled:        true,
		RequiresKey:    false,
		AuthConfigured: true,
		Capabilities:   []string{"yield.aggregate", "lending.markets", "lending.rates"},
	}
}

func (p *Provider) FetchPools(ctx context.Context) ([]Pool, error) {
	url := p.baseURL + "/pools"
	var out PoolsResponse
	if _, err := httpx.DoBodyJSON(ctx, p.client, http.MethodGet, url, nil, nil, &out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "defillama returned no pool data")
	}
	return out.Data, nil
}

func FilterPools(pools []Pool, chain, projectContains string) []Pool {
	normChain := strings.ToLower(strings.TrimSpace(chain))
	normProject := strings.ToLower(strings.TrimSpace(projectContains))
	items := make([]Pool, 0, len(pools))
	for _, pool := range pools {
		if normChain != "" && !strings.Contains(strings.ToLower(pool.Chain), normChain) {
			continue
		}
		if normProject != "" && !strings.Contains(strings.ToLower(pool.Project), normProject) {
			continue
		}
		items = append(items, pool)
	}
	return items
}

func MatchesAsset(pool Pool, asset id.Asset) bool {
	if strings.TrimSpace(asset.Symbol) == "" && strings.TrimSpace(asset.Address) == "" {
		return true
	}
	if strings.EqualFold(asset.Symbol, pool.Symbol) {
		return true
	}
	if strings.TrimSpace(asset.Address) != "" {
		for _, token := range pool.UnderlyingTokens {
			if strings.EqualFold(token, asset.Address) {
				return true
			}
		}
	}
	return false
}

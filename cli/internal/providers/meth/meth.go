package meth

import (
	"context"
	"math/big"
	"strings"
	"time"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/id"
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
		Name:           "meth",
		Type:           "staking",
		Enabled:        true,
		RequiresKey:    false,
		AuthConfigured: true,
		Capabilities:   []string{"stake.info", "stake.quote"},
	}
}

func (p *Provider) StakeInfo(ctx context.Context) (model.StakeInfo, error) {
	pools, err := p.llama.FetchPools(ctx)
	if err != nil {
		return model.StakeInfo{}, err
	}

	apy := 0.0
	for _, pool := range pools {
		if !strings.Contains(strings.ToLower(pool.Chain), "mantle") {
			continue
		}
		symbol := strings.ToLower(strings.TrimSpace(pool.Symbol))
		project := strings.ToLower(strings.TrimSpace(pool.Project))
		if strings.Contains(symbol, "meth") || strings.Contains(project, "meth") {
			apy = pool.APY
			break
		}
	}

	return model.StakeInfo{
		Protocol:     "meth",
		METHToETH:    "1",
		ETHToMETH:    "1",
		APY:          apy,
		TotalStaked:  "0",
		TotalMETH:    "0",
		UnstakeDelay: "7d",
		FetchedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (p *Provider) StakeQuote(ctx context.Context, req providers.StakeQuoteRequest) (model.StakeQuote, error) {
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "stake" && action != "unstake" {
		return model.StakeQuote{}, clierr.New(clierr.CodeUsage, "action must be stake or unstake")
	}
	baseIn, decimalIn, err := id.NormalizeAmount("", req.AmountDecimal, 18)
	if err != nil {
		return model.StakeQuote{}, err
	}
	info, err := p.StakeInfo(ctx)
	if err != nil {
		return model.StakeQuote{}, err
	}

	baseOut := baseIn
	decimalOut := decimalIn
	if action == "stake" && info.ETHToMETH != "1" {
		if converted, convErr := multiplyByRatio(baseIn, info.ETHToMETH); convErr == nil {
			baseOut = converted
			decimalOut = id.FormatDecimalCompat(baseOut, 18)
		}
	}
	if action == "unstake" && info.METHToETH != "1" {
		if converted, convErr := multiplyByRatio(baseIn, info.METHToETH); convErr == nil {
			baseOut = converted
			decimalOut = id.FormatDecimalCompat(baseOut, 18)
		}
	}

	return model.StakeQuote{
		Action: action,
		InputAmount: model.AmountInfo{
			AmountBaseUnits: baseIn,
			AmountDecimal:   decimalIn,
			Decimals:        18,
		},
		EstimatedOut: model.AmountInfo{
			AmountBaseUnits: baseOut,
			AmountDecimal:   decimalOut,
			Decimals:        18,
		},
		ExchangeRate: info.METHToETH,
		MinOutput:    decimalOut,
		FetchedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func multiplyByRatio(baseUnits, ratio string) (string, error) {
	base := new(big.Float)
	if _, ok := base.SetString(baseUnits); !ok {
		return "", clierr.New(clierr.CodeUsage, "invalid base units")
	}
	r := new(big.Float)
	if _, ok := r.SetString(strings.TrimSpace(ratio)); !ok {
		return "", clierr.New(clierr.CodeUsage, "invalid exchange rate")
	}
	v := new(big.Float).Mul(base, r)
	out := new(big.Int)
	v.Int(out)
	return out.String(), nil
}

var _ providers.StakingProvider = (*Provider)(nil)

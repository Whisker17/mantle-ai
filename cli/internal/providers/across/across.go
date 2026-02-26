package across

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/httpx"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/providers"
)

const defaultBaseURL = "https://api.across.to"

type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	Retries int
}

type Provider struct {
	client  *httpx.Client
	baseURL string
	apiKey  string
}

type quoteResponse struct {
	OutputAmount          string `json:"outputAmount"`
	EstimatedFillTimeSec  int64  `json:"estimatedFillTimeSec"`
	EstimatedFillTime     int64  `json:"estimatedFillTime"`
	RelayFeePct           string `json:"relayFeePct"`
	TotalRelayFeePct      string `json:"totalRelayFeePct"`
	TotalRelayFeeTotal    string `json:"totalRelayFeeTotal"`
	EstimatedFeeTotal     string `json:"estimatedFeeTotal"`
	EstimatedFeeAmount    string `json:"estimatedFeeAmount"`
	EstimatedFeeUSD       string `json:"estimatedFeeUsd"`
	EstimatedFillTimeSecs int64  `json:"estimatedFillTimeSeconds"`
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
		apiKey:  strings.TrimSpace(cfg.APIKey),
	}
}

func (p *Provider) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:           "across",
		Type:           "bridge",
		Enabled:        true,
		RequiresKey:    true,
		AuthConfigured: p.apiKey != "",
		KeyEnvVarName:  "ACROSS_API_KEY",
		Capabilities:   []string{"bridge.quote"},
	}
}

func (p *Provider) QuoteBridge(ctx context.Context, req providers.BridgeQuoteRequest) (model.BridgeQuote, error) {
	decimals := req.Asset.Decimals
	if decimals <= 0 {
		decimals = 18
	}
	amountBase, amountDecimal, err := id.NormalizeAmount("", req.AmountDecimal, decimals)
	if err != nil {
		return model.BridgeQuote{}, err
	}
	if strings.TrimSpace(req.Asset.Address) == "" {
		return model.BridgeQuote{}, clierr.New(clierr.CodeUsage, "across requires an ERC-20 token address")
	}

	query := url.Values{}
	query.Set("originChainId", strconv.FormatInt(req.FromChain.EVMChainID, 10))
	query.Set("destinationChainId", strconv.FormatInt(req.ToChain.EVMChainID, 10))
	query.Set("token", req.Asset.Address)
	query.Set("amount", amountBase)
	endpoint := fmt.Sprintf("%s/api/suggested-fees?%s", p.baseURL, query.Encode())

	headers := map[string]string{}
	if p.apiKey != "" {
		headers["Authorization"] = "Bearer " + p.apiKey
	}
	var resp quoteResponse
	if _, err := httpx.DoBodyJSON(ctx, p.client, http.MethodGet, endpoint, nil, headers, &resp); err != nil {
		return model.BridgeQuote{}, err
	}

	outputBase := strings.TrimSpace(resp.OutputAmount)
	if outputBase == "" {
		outputBase = amountBase
	}
	estimatedTime := resp.EstimatedFillTimeSec
	if estimatedTime == 0 {
		estimatedTime = resp.EstimatedFillTime
	}
	if estimatedTime == 0 {
		estimatedTime = resp.EstimatedFillTimeSecs
	}
	if estimatedTime == 0 {
		estimatedTime = 900
	}

	feeUSD := parseFloat(resp.EstimatedFeeUSD)
	if feeUSD == 0 {
		feeUSD = parseFloat(resp.EstimatedFeeAmount)
	}

	return model.BridgeQuote{
		Provider:  "across",
		FromChain: req.FromChain.CAIP2,
		ToChain:   req.ToChain.CAIP2,
		Asset:     bridgeAssetLabel(req.Asset),
		InputAmount: model.AmountInfo{
			AmountBaseUnits: amountBase,
			AmountDecimal:   amountDecimal,
			Decimals:        decimals,
		},
		EstimatedOut: model.AmountInfo{
			AmountBaseUnits: outputBase,
			AmountDecimal:   id.FormatDecimalCompat(outputBase, decimals),
			Decimals:        decimals,
		},
		EstimatedFeeUSD: feeUSD,
		EstimatedTimeS:  estimatedTime,
		Route:           "across",
		FetchedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func parseFloat(v string) float64 {
	norm := strings.TrimSpace(v)
	if norm == "" {
		return 0
	}
	f, err := strconv.ParseFloat(norm, 64)
	if err != nil {
		return 0
	}
	return f
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

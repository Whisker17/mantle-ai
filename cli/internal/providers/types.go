package providers

import (
	"context"

	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
)

type Provider interface {
	Info() model.ProviderInfo
}

type ChainProvider interface {
	Provider
	ChainInfo(ctx context.Context) (model.ChainInfo, error)
	ChainStatus(ctx context.Context) (model.ChainStatus, error)
	GetBalance(ctx context.Context, address string) (model.AddressBalance, error)
	GetTransaction(ctx context.Context, hash string) (model.TransactionInfo, error)
	ReadContract(ctx context.Context, req ContractReadRequest) (model.ContractResult, error)
	SimulateCall(ctx context.Context, req ContractCallRequest) (model.CallSimulation, error)
	GetTokenInfo(ctx context.Context, address string) (model.TokenInfo, error)
	ResolveToken(ctx context.Context, symbol string) (model.TokenResolution, error)
	GetTokenBalances(ctx context.Context, address string) ([]model.TokenBalance, error)
}

type ContractReadRequest struct {
	Address  string
	Function string
	Args     []string
}

type ContractCallRequest struct {
	From     string
	To       string
	Function string
	Args     []string
	Value    string
}

type SwapProvider interface {
	Provider
	QuoteSwap(ctx context.Context, req SwapQuoteRequest) (model.SwapQuote, error)
}

type SwapQuoteRequest struct {
	FromAsset       id.Asset
	ToAsset         id.Asset
	AmountBaseUnits string
	AmountDecimal   string
	FeeTier         int
}

type LendingProvider interface {
	Provider
	LendMarkets(ctx context.Context, asset id.Asset) ([]model.LendMarket, error)
	LendRates(ctx context.Context, asset id.Asset) ([]model.LendRate, error)
}

type StakingProvider interface {
	Provider
	StakeInfo(ctx context.Context) (model.StakeInfo, error)
	StakeQuote(ctx context.Context, req StakeQuoteRequest) (model.StakeQuote, error)
}

type StakeQuoteRequest struct {
	Action        string
	AmountDecimal string
}

type YieldProvider interface {
	Provider
	YieldOpportunities(ctx context.Context, req YieldRequest) ([]model.YieldOpportunity, error)
}

type YieldRequest struct {
	Asset     id.Asset
	Limit     int
	MinTVLUSD float64
	MinAPY    float64
	SortBy    string
}

type BridgeProvider interface {
	Provider
	QuoteBridge(ctx context.Context, req BridgeQuoteRequest) (model.BridgeQuote, error)
}

type BridgeQuoteRequest struct {
	FromChain     id.Chain
	ToChain       id.Chain
	Asset         id.Asset
	AmountDecimal string
}

type BridgeStatusProvider interface {
	Provider
	BridgeStatus(ctx context.Context, txHash string) (model.BridgeStatus, error)
}

package merchantmoe

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/providers"
)

const routerABIJSON = `[
  {
    "type":"function",
    "name":"getAmountsOut",
    "stateMutability":"view",
    "inputs":[
      {"name":"amountIn","type":"uint256"},
      {"name":"path","type":"address[]"}
    ],
    "outputs":[
      {"name":"amounts","type":"uint256[]"}
    ]
  }
]`

type Config struct {
	Network string
	RPCURL  string
}

type Provider struct {
	client        *ethclient.Client
	routerAddress common.Address
	abi           abi.ABI
}

func New(cfg Config) (*Provider, error) {
	if strings.TrimSpace(cfg.RPCURL) == "" {
		return nil, clierr.New(clierr.CodeUsage, "rpc url is required")
	}
	router, err := routerForNetwork(cfg.Network)
	if err != nil {
		return nil, err
	}
	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "connect merchant moe rpc", err)
	}
	parsed, err := abi.JSON(strings.NewReader(routerABIJSON))
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, "parse merchant moe abi", err)
	}
	return &Provider{
		client:        client,
		routerAddress: router,
		abi:           parsed,
	}, nil
}

func (p *Provider) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

func (p *Provider) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:           "merchant_moe",
		Type:           "dex",
		Enabled:        true,
		RequiresKey:    false,
		AuthConfigured: true,
		Capabilities:   []string{"swap.quote"},
	}
}

func (p *Provider) QuoteSwap(ctx context.Context, req providers.SwapQuoteRequest) (model.SwapQuote, error) {
	if !common.IsHexAddress(req.FromAsset.Address) || !common.IsHexAddress(req.ToAsset.Address) {
		return model.SwapQuote{}, clierr.New(clierr.CodeUsage, "swap assets must resolve to addresses")
	}
	amountIn, ok := new(big.Int).SetString(strings.TrimSpace(req.AmountBaseUnits), 10)
	if !ok || amountIn.Sign() <= 0 {
		return model.SwapQuote{}, clierr.New(clierr.CodeUsage, "invalid amount")
	}

	path := []common.Address{
		common.HexToAddress(req.FromAsset.Address),
		common.HexToAddress(req.ToAsset.Address),
	}
	data, err := p.abi.Pack("getAmountsOut", amountIn, path)
	if err != nil {
		return model.SwapQuote{}, clierr.Wrap(clierr.CodeInternal, "encode merchant moe quote call", err)
	}
	msg := geth.CallMsg{To: &p.routerAddress, Data: data}
	raw, err := p.client.CallContract(ctx, msg, nil)
	if err != nil {
		return model.SwapQuote{}, clierr.Wrap(clierr.CodeUnavailable, "merchant moe quote call failed", err)
	}
	out, err := p.abi.Unpack("getAmountsOut", raw)
	if err != nil || len(out) != 1 {
		return model.SwapQuote{}, clierr.Wrap(clierr.CodeUnavailable, "decode merchant moe quote", err)
	}
	amounts, ok := out[0].([]*big.Int)
	if !ok || len(amounts) == 0 {
		return model.SwapQuote{}, clierr.New(clierr.CodeUnavailable, "merchant moe quote returned empty path")
	}
	amountOut := amounts[len(amounts)-1]

	gasPrice, err := p.client.SuggestGasPrice(ctx)
	if err != nil {
		gasPrice = big.NewInt(0)
	}
	estimatedGas := new(big.Int).Mul(gasPrice, big.NewInt(220000))

	return model.SwapQuote{
		Provider:  "merchant_moe",
		FromAsset: req.FromAsset.Symbol,
		ToAsset:   req.ToAsset.Symbol,
		InputAmount: model.AmountInfo{
			AmountBaseUnits: amountIn.String(),
			AmountDecimal:   req.AmountDecimal,
			Decimals:        req.FromAsset.Decimals,
		},
		EstimatedOut: model.AmountInfo{
			AmountBaseUnits: amountOut.String(),
			AmountDecimal:   id.FormatDecimalCompat(amountOut.String(), req.ToAsset.Decimals),
			Decimals:        req.ToAsset.Decimals,
		},
		PriceImpactPct:  0,
		EstimatedGasMNT: id.FormatDecimalCompat(estimatedGas.String(), 18),
		Route:           fmt.Sprintf("%s/%s", req.FromAsset.Symbol, req.ToAsset.Symbol),
		RouterAddress:   strings.ToLower(p.routerAddress.Hex()),
		FetchedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func routerForNetwork(network string) (common.Address, error) {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "", "mainnet":
		return common.HexToAddress("0xeaEE7EE68874218c3558b40063c42B82D3E7232a"), nil
	case "sepolia":
		return common.Address{}, clierr.New(clierr.CodeUnsupported, "merchant moe is not configured on sepolia")
	default:
		return common.Address{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("unsupported network: %s", network))
	}
}

var _ providers.SwapProvider = (*Provider)(nil)

package agni

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

const quoterABIJSON = `[
  {
    "type":"function",
    "name":"quoteExactInputSingle",
    "stateMutability":"view",
    "inputs":[
      {
        "name":"params",
        "type":"tuple",
        "components":[
          {"name":"tokenIn","type":"address"},
          {"name":"tokenOut","type":"address"},
          {"name":"amountIn","type":"uint256"},
          {"name":"fee","type":"uint24"},
          {"name":"sqrtPriceLimitX96","type":"uint160"}
        ]
      }
    ],
    "outputs":[
      {"name":"amountOut","type":"uint256"}
    ]
  }
]`

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
	quoterAddress common.Address
	routerAddress common.Address
	quoterABI     abi.ABI
	routerABI     abi.ABI
}

type quoteExactInputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	AmountIn          *big.Int
	Fee               *big.Int
	SqrtPriceLimitX96 *big.Int
}

func New(cfg Config) (*Provider, error) {
	if strings.TrimSpace(cfg.RPCURL) == "" {
		return nil, clierr.New(clierr.CodeUsage, "rpc url is required")
	}
	quoter, router, err := addressesForNetwork(cfg.Network)
	if err != nil {
		return nil, err
	}
	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "connect agni rpc", err)
	}
	quoterABI, err := abi.JSON(strings.NewReader(quoterABIJSON))
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, "parse agni quoter abi", err)
	}
	routerABI, err := abi.JSON(strings.NewReader(routerABIJSON))
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, "parse agni router abi", err)
	}
	return &Provider{
		client:        client,
		quoterAddress: quoter,
		routerAddress: router,
		quoterABI:     quoterABI,
		routerABI:     routerABI,
	}, nil
}

func (p *Provider) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

func (p *Provider) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:           "agni",
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
	if !ok {
		return model.SwapQuote{}, clierr.New(clierr.CodeUsage, "invalid amount")
	}
	if amountIn.Sign() <= 0 {
		return model.SwapQuote{}, clierr.New(clierr.CodeUsage, "amount must be greater than zero")
	}
	feeTier := req.FeeTier
	if feeTier == 0 {
		feeTier = 3000
	}

	var amountOut *big.Int
	gasUnits := int64(200000)
	if p.quoterAddress != (common.Address{}) {
		data, err := p.quoterABI.Pack(
			"quoteExactInputSingle",
			quoteExactInputSingleParams{
				TokenIn:           common.HexToAddress(req.FromAsset.Address),
				TokenOut:          common.HexToAddress(req.ToAsset.Address),
				AmountIn:          amountIn,
				Fee:               big.NewInt(int64(feeTier)),
				SqrtPriceLimitX96: new(big.Int),
			},
		)
		if err != nil {
			return model.SwapQuote{}, clierr.Wrap(clierr.CodeInternal, "encode agni quote call", err)
		}
		msg := geth.CallMsg{To: &p.quoterAddress, Data: data}
		raw, err := p.client.CallContract(ctx, msg, nil)
		if err != nil {
			return model.SwapQuote{}, clierr.Wrap(clierr.CodeUnavailable, "agni quote call failed", err)
		}
		out, err := p.quoterABI.Unpack("quoteExactInputSingle", raw)
		if err != nil || len(out) != 1 {
			return model.SwapQuote{}, clierr.Wrap(clierr.CodeUnavailable, "decode agni quote", err)
		}
		var ok bool
		amountOut, ok = out[0].(*big.Int)
		if !ok {
			return model.SwapQuote{}, clierr.New(clierr.CodeUnavailable, "invalid agni quote output type")
		}
	} else {
		path := []common.Address{
			common.HexToAddress(req.FromAsset.Address),
			common.HexToAddress(req.ToAsset.Address),
		}
		data, err := p.routerABI.Pack("getAmountsOut", amountIn, path)
		if err != nil {
			return model.SwapQuote{}, clierr.Wrap(clierr.CodeInternal, "encode agni router quote call", err)
		}
		msg := geth.CallMsg{To: &p.routerAddress, Data: data}
		raw, err := p.client.CallContract(ctx, msg, nil)
		if err != nil {
			return model.SwapQuote{}, clierr.Wrap(clierr.CodeUnavailable, "agni router quote call failed", err)
		}
		out, err := p.routerABI.Unpack("getAmountsOut", raw)
		if err != nil || len(out) != 1 {
			return model.SwapQuote{}, clierr.Wrap(clierr.CodeUnavailable, "decode agni router quote", err)
		}
		amounts, ok := out[0].([]*big.Int)
		if !ok || len(amounts) == 0 {
			return model.SwapQuote{}, clierr.New(clierr.CodeUnavailable, "agni router quote returned empty path")
		}
		amountOut = amounts[len(amounts)-1]
		gasUnits = 220000
	}

	gasPrice, err := p.client.SuggestGasPrice(ctx)
	if err != nil {
		gasPrice = big.NewInt(0)
	}
	estimatedGas := new(big.Int).Mul(gasPrice, big.NewInt(gasUnits))

	return model.SwapQuote{
		Provider:  "agni",
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

func addressesForNetwork(network string) (common.Address, common.Address, error) {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "", "mainnet":
		return common.HexToAddress("0xc4aaDc921E1cdb66c5300Bc158a313292923C0cb"), common.HexToAddress("0x319B69888b0d11cEC22caA5034e25FfFBDc88421"), nil
	case "sepolia":
		return common.Address{}, common.HexToAddress("0x3E30894AaEB2ba741b8E2999604D1d01fF6244ea"), nil
	default:
		return common.Address{}, common.Address{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("unsupported network: %s", network))
	}
}

var _ providers.SwapProvider = (*Provider)(nil)

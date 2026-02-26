package aavev3

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
)

const mantleMainnetProtocolDataProvider = "0x487c5c669D9eee6057C44973207101276cf73b68"

const dataProviderABIJSON = `[
  {
    "type":"function",
    "name":"getReserveConfigurationData",
    "stateMutability":"view",
    "inputs":[{"name":"asset","type":"address"}],
    "outputs":[
      {"name":"decimals","type":"uint256"},
      {"name":"ltv","type":"uint256"},
      {"name":"liquidationThreshold","type":"uint256"},
      {"name":"liquidationBonus","type":"uint256"},
      {"name":"reserveFactor","type":"uint256"},
      {"name":"usageAsCollateralEnabled","type":"bool"},
      {"name":"borrowingEnabled","type":"bool"},
      {"name":"stableBorrowRateEnabled","type":"bool"},
      {"name":"isActive","type":"bool"},
      {"name":"isFrozen","type":"bool"}
    ]
  },
  {
    "type":"function",
    "name":"getReserveData",
    "stateMutability":"view",
    "inputs":[{"name":"asset","type":"address"}],
    "outputs":[
      {"name":"unbacked","type":"uint256"},
      {"name":"accruedToTreasuryScaled","type":"uint256"},
      {"name":"totalAToken","type":"uint256"},
      {"name":"totalStableDebt","type":"uint256"},
      {"name":"totalVariableDebt","type":"uint256"},
      {"name":"liquidityRate","type":"uint256"},
      {"name":"variableBorrowRate","type":"uint256"},
      {"name":"stableBorrowRate","type":"uint256"},
      {"name":"averageStableBorrowRate","type":"uint256"},
      {"name":"liquidityIndex","type":"uint256"},
      {"name":"variableBorrowIndex","type":"uint256"},
      {"name":"lastUpdateTimestamp","type":"uint40"}
    ]
  }
]`

type Config struct {
	Network            string
	RPCURL             string
	ProtocolDataSource string
}

type Provider struct {
	cfg     Config
	abi     abi.ABI
	initErr error
}

type reserveConfig struct {
	Symbol   string
	Address  common.Address
	Decimals int
}

var mantleMainnetReserves = []reserveConfig{
	{Symbol: "WETH", Address: common.HexToAddress("0xdEAddEaDdeadDEadDEADDEAddEADDEAddead1111"), Decimals: 18},
	{Symbol: "WMNT", Address: common.HexToAddress("0x78c1b0C915c4FAA5FffA6CAbf0219DA63d7f4cb8"), Decimals: 18},
	{Symbol: "USDT0", Address: common.HexToAddress("0x779Ded0c9e1022225f8E0630b35a9b54bE713736"), Decimals: 6},
	{Symbol: "USDC", Address: common.HexToAddress("0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9"), Decimals: 6},
	{Symbol: "USDE", Address: common.HexToAddress("0x5d3a1Ff2b6BAb83b63cd9AD0787074081a52ef34"), Decimals: 18},
	{Symbol: "SUSDE", Address: common.HexToAddress("0x211Cc4DD073734dA055fbF44a2b4667d5E5fE5d2"), Decimals: 18},
	{Symbol: "FBTC", Address: common.HexToAddress("0xC96dE26018A54D51c097160568752c4E3BD6C364"), Decimals: 8},
	{Symbol: "SYRUPUSDT", Address: common.HexToAddress("0x051665f2455116e929b9972c36d23070F5054Ce0"), Decimals: 6},
	{Symbol: "WRSETH", Address: common.HexToAddress("0x93e855643e940D025bE2e529272e4Dbd15a2Cf74"), Decimals: 18},
	{Symbol: "GHO", Address: common.HexToAddress("0xfc421aD3C883Bf9E7C4f42dE845C4e4405799e73"), Decimals: 18},
}

func New(cfg Config) *Provider {
	parsedABI, err := abi.JSON(strings.NewReader(dataProviderABIJSON))
	if err != nil {
		return &Provider{
			cfg:     cfg,
			initErr: clierr.Wrap(clierr.CodeInternal, "parse aave v3 data provider abi", err),
		}
	}
	return &Provider{
		cfg: cfg,
		abi: parsedABI,
	}
}

func (p *Provider) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:           "aave_v3",
		Type:           "lending",
		Enabled:        true,
		RequiresKey:    false,
		AuthConfigured: true,
		Capabilities:   []string{"lend.markets", "lend.rates"},
	}
}

func (p *Provider) LendMarkets(ctx context.Context, asset id.Asset) ([]model.LendMarket, error) {
	reserves, err := p.queryReserves(ctx, asset)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]model.LendMarket, 0, len(reserves))
	for _, reserve := range reserves {
		out = append(out, model.LendMarket{
			Protocol:  "aave_v3",
			Asset:     reserve.asset.Symbol,
			SupplyAPY: reserve.supplyAPY,
			BorrowAPY: reserve.borrowAPY,
			TVLUSD:    0,
			FetchedAt: now,
		})
	}
	if len(out) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "aave v3 markets unavailable")
	}
	return out, nil
}

func (p *Provider) LendRates(ctx context.Context, asset id.Asset) ([]model.LendRate, error) {
	reserves, err := p.queryReserves(ctx, asset)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]model.LendRate, 0, len(reserves))
	for _, reserve := range reserves {
		out = append(out, model.LendRate{
			Protocol:    "aave_v3",
			Asset:       reserve.asset.Symbol,
			SupplyAPY:   reserve.supplyAPY,
			BorrowAPY:   reserve.borrowAPY,
			Utilization: reserve.utilization,
			FetchedAt:   now,
		})
	}
	if len(out) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "aave v3 rates unavailable")
	}
	return out, nil
}

type reserveSnapshot struct {
	asset       reserveConfig
	supplyAPY   float64
	borrowAPY   float64
	utilization float64
}

func (p *Provider) queryReserves(ctx context.Context, asset id.Asset) ([]reserveSnapshot, error) {
	if p.initErr != nil {
		return nil, p.initErr
	}
	network := normalizeNetwork(p.cfg.Network)
	if network != "mainnet" {
		return nil, clierr.New(clierr.CodeUnsupported, "aave v3 lending is currently supported on mainnet only")
	}
	rpcURL := strings.TrimSpace(p.cfg.RPCURL)
	if rpcURL == "" {
		return nil, clierr.New(clierr.CodeUsage, "rpc url is required for aave v3")
	}

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "connect aave v3 rpc", err)
	}
	defer client.Close()

	dataProviderAddress, err := p.protocolDataProviderAddress()
	if err != nil {
		return nil, err
	}

	targets := filterReserves(mantleMainnetReserves, asset)
	if len(targets) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "aave v3 has no reserves for requested asset")
	}

	results := make([]reserveSnapshot, 0, len(targets))
	var firstErr error
	for _, reserve := range targets {
		cfgOut, cfgErr := p.call(ctx, client, dataProviderAddress, "getReserveConfigurationData", reserve.Address)
		if cfgErr != nil {
			if firstErr == nil {
				firstErr = cfgErr
			}
			continue
		}
		isActive, isFrozen, cfgParseErr := parseActiveFrozen(cfgOut)
		if cfgParseErr != nil {
			if firstErr == nil {
				firstErr = cfgParseErr
			}
			continue
		}
		if !isActive || isFrozen {
			continue
		}

		dataOut, dataErr := p.call(ctx, client, dataProviderAddress, "getReserveData", reserve.Address)
		if dataErr != nil {
			if firstErr == nil {
				firstErr = dataErr
			}
			continue
		}
		totalAToken, totalStableDebt, totalVariableDebt, liquidityRate, variableBorrowRate, parseErr := parseReserveData(dataOut)
		if parseErr != nil {
			if firstErr == nil {
				firstErr = parseErr
			}
			continue
		}

		totalDebt := new(big.Int).Add(totalStableDebt, totalVariableDebt)
		totalSupply := toDecimal(totalAToken, reserve.Decimals)
		util := 0.0
		if totalSupply > 0 {
			util = toDecimal(totalDebt, reserve.Decimals) / totalSupply * 100
		}

		results = append(results, reserveSnapshot{
			asset:       reserve,
			supplyAPY:   rayToPercent(liquidityRate),
			borrowAPY:   rayToPercent(variableBorrowRate),
			utilization: util,
		})
	}

	if len(results) == 0 {
		if firstErr == nil {
			firstErr = clierr.New(clierr.CodeUnavailable, "aave v3 reserves unavailable")
		}
		return nil, firstErr
	}
	return results, nil
}

func (p *Provider) protocolDataProviderAddress() (common.Address, error) {
	override := strings.TrimSpace(p.cfg.ProtocolDataSource)
	if override == "" {
		override = mantleMainnetProtocolDataProvider
	}
	if !common.IsHexAddress(override) {
		return common.Address{}, clierr.New(clierr.CodeUsage, "invalid aave v3 protocol data provider address")
	}
	return common.HexToAddress(override), nil
}

func (p *Provider) call(ctx context.Context, client *ethclient.Client, to common.Address, method string, args ...any) ([]any, error) {
	data, err := p.abi.Pack(method, args...)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, fmt.Sprintf("encode aave v3 %s call", method), err)
	}
	msg := geth.CallMsg{To: &to, Data: data}
	raw, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, fmt.Sprintf("aave v3 %s call failed", method), err)
	}
	out, err := p.abi.Unpack(method, raw)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, fmt.Sprintf("decode aave v3 %s response", method), err)
	}
	return out, nil
}

func parseActiveFrozen(values []any) (bool, bool, error) {
	if len(values) < 10 {
		return false, false, clierr.New(clierr.CodeUnavailable, "unexpected aave v3 reserve configuration response")
	}
	isActive, okActive := values[8].(bool)
	isFrozen, okFrozen := values[9].(bool)
	if !okActive || !okFrozen {
		return false, false, clierr.New(clierr.CodeUnavailable, "invalid aave v3 reserve configuration flags")
	}
	return isActive, isFrozen, nil
}

func parseReserveData(values []any) (*big.Int, *big.Int, *big.Int, *big.Int, *big.Int, error) {
	if len(values) < 7 {
		return nil, nil, nil, nil, nil, clierr.New(clierr.CodeUnavailable, "unexpected aave v3 reserve data response")
	}
	totalAToken, okAToken := values[2].(*big.Int)
	totalStableDebt, okStableDebt := values[3].(*big.Int)
	totalVariableDebt, okVariableDebt := values[4].(*big.Int)
	liquidityRate, okLiquidityRate := values[5].(*big.Int)
	variableBorrowRate, okVariableBorrowRate := values[6].(*big.Int)
	if !okAToken || !okStableDebt || !okVariableDebt || !okLiquidityRate || !okVariableBorrowRate {
		return nil, nil, nil, nil, nil, clierr.New(clierr.CodeUnavailable, "invalid aave v3 reserve numeric fields")
	}
	return totalAToken, totalStableDebt, totalVariableDebt, liquidityRate, variableBorrowRate, nil
}

func filterReserves(reserves []reserveConfig, asset id.Asset) []reserveConfig {
	if strings.TrimSpace(asset.Symbol) == "" && strings.TrimSpace(asset.Address) == "" {
		out := make([]reserveConfig, 0, len(reserves))
		out = append(out, reserves...)
		return out
	}
	out := make([]reserveConfig, 0, len(reserves))
	symbol := strings.ToUpper(strings.TrimSpace(asset.Symbol))
	address := strings.ToLower(strings.TrimSpace(asset.Address))
	for _, reserve := range reserves {
		if symbol != "" && strings.EqualFold(symbol, reserve.Symbol) {
			out = append(out, reserve)
			continue
		}
		if address != "" && strings.EqualFold(address, reserve.Address.Hex()) {
			out = append(out, reserve)
			continue
		}
	}
	return out
}

func rayToPercent(ray *big.Int) float64 {
	if ray == nil || ray.Sign() <= 0 {
		return 0
	}
	r := new(big.Float).SetPrec(256).SetInt(ray)
	rayScale, _ := new(big.Float).SetString("1000000000000000000000000000") // 1e27
	r.Quo(r, rayScale)
	r.Mul(r, big.NewFloat(100))
	value, _ := r.Float64()
	return value
}

func toDecimal(amount *big.Int, decimals int) float64 {
	if amount == nil {
		return 0
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	n := new(big.Float).SetPrec(256).SetInt(amount)
	n.Quo(n, new(big.Float).SetInt(scale))
	out, _ := n.Float64()
	return out
}

func normalizeNetwork(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "mainnet", "mantle", "5000", "eip155:5000":
		return "mainnet"
	case "sepolia", "mantle-sepolia", "5003", "eip155:5003":
		return "sepolia"
	default:
		return ""
	}
}

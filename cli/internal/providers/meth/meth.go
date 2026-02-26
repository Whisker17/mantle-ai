package meth

import (
	"context"
	"fmt"
	"math/big"
	"os"
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
	"github.com/mantle/mantle-ai/cli/internal/providers/defillama"
)

const (
	defaultMainnetEthereumRPC = "https://ethereum-rpc.publicnode.com"
	defaultSepoliaEthereumRPC = "https://ethereum-sepolia-rpc.publicnode.com"

	mainnetStakingAddress = "0xe3cBd06D7dadB3F4e6557bAb7EdD924CD1489E8f"
	sepoliaStakingAddress = "0xF4fe5697C34BCa078419070FD4AAA9ebBE05de61"
)

const stakingABIJSON = `[
  {"type":"function","name":"ethToMETH","stateMutability":"view","inputs":[{"name":"ethAmount","type":"uint256"}],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"mETHToETH","stateMutability":"view","inputs":[{"name":"mETHAmount","type":"uint256"}],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"totalControlled","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"mETH","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]}
]`

const erc20TotalSupplyABIJSON = `[
  {"type":"function","name":"totalSupply","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]}
]`

type Config struct {
	Network        string
	BaseURL        string
	Timeout        time.Duration
	Retries        int
	EthereumRPCURL string
}

type Provider struct {
	cfg        Config
	llama      *defillama.Provider
	stakingABI abi.ABI
	erc20ABI   abi.ABI
	initErr    error
}

func New(cfg Config) *Provider {
	stakingABI, stakingErr := abi.JSON(strings.NewReader(stakingABIJSON))
	erc20ABI, erc20Err := abi.JSON(strings.NewReader(erc20TotalSupplyABIJSON))
	var initErr error
	if stakingErr != nil {
		initErr = clierr.Wrap(clierr.CodeInternal, "parse meth staking abi", stakingErr)
	}
	if erc20Err != nil && initErr == nil {
		initErr = clierr.Wrap(clierr.CodeInternal, "parse meth erc20 abi", erc20Err)
	}
	return &Provider{
		cfg: cfg,
		llama: defillama.New(defillama.Config{
			BaseURL: cfg.BaseURL,
			Timeout: cfg.Timeout,
			Retries: cfg.Retries,
		}),
		stakingABI: stakingABI,
		erc20ABI:   erc20ABI,
		initErr:    initErr,
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
	if p.initErr != nil {
		return model.StakeInfo{}, p.initErr
	}
	rpcURL, stakingAddress, err := resolveL1Config(p.cfg)
	if err != nil {
		return model.StakeInfo{}, err
	}

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return model.StakeInfo{}, clierr.Wrap(clierr.CodeUnavailable, "connect ethereum rpc for meth stake info", err)
	}
	defer client.Close()

	oneETH := big.NewInt(1_000_000_000_000_000_000)
	ethToMETHBase, err := p.callUint(ctx, client, stakingAddress, "ethToMETH", oneETH)
	if err != nil {
		return model.StakeInfo{}, err
	}
	mETHToETHBase, err := p.callUint(ctx, client, stakingAddress, "mETHToETH", oneETH)
	if err != nil {
		return model.StakeInfo{}, err
	}
	totalControlled, err := p.callUint(ctx, client, stakingAddress, "totalControlled")
	if err != nil {
		return model.StakeInfo{}, err
	}
	mETHAddress, err := p.callAddress(ctx, client, stakingAddress, "mETH")
	if err != nil {
		return model.StakeInfo{}, err
	}
	totalMETH, err := p.callERC20TotalSupply(ctx, client, mETHAddress)
	if err != nil {
		return model.StakeInfo{}, err
	}

	apy, err := p.fetchAPY(ctx)
	if err != nil {
		return model.StakeInfo{}, err
	}

	return model.StakeInfo{
		Protocol:     "meth",
		METHToETH:    id.FormatDecimalCompat(mETHToETHBase.String(), 18),
		ETHToMETH:    id.FormatDecimalCompat(ethToMETHBase.String(), 18),
		APY:          apy,
		TotalStaked:  id.FormatDecimalCompat(totalControlled.String(), 18),
		TotalMETH:    id.FormatDecimalCompat(totalMETH.String(), 18),
		UnstakeDelay: "~7d",
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

func (p *Provider) fetchAPY(ctx context.Context) (float64, error) {
	pools, err := p.llama.FetchPools(ctx)
	if err != nil {
		return 0, err
	}

	apy := 0.0
	for _, pool := range pools {
		chain := strings.ToLower(strings.TrimSpace(pool.Chain))
		if !strings.Contains(chain, "mantle") && !strings.Contains(chain, "ethereum") {
			continue
		}
		symbol := strings.ToLower(strings.TrimSpace(pool.Symbol))
		project := strings.ToLower(strings.TrimSpace(pool.Project))
		if strings.Contains(symbol, "meth") || strings.Contains(project, "meth") {
			apy = pool.APY
			break
		}
	}
	return apy, nil
}

func (p *Provider) callUint(ctx context.Context, client *ethclient.Client, contract common.Address, method string, args ...any) (*big.Int, error) {
	raw, err := p.call(ctx, client, contract, p.stakingABI, method, args...)
	if err != nil {
		return nil, err
	}
	value, ok := raw.(*big.Int)
	if !ok {
		return nil, clierr.New(clierr.CodeUnavailable, fmt.Sprintf("unexpected %s output type", method))
	}
	return value, nil
}

func (p *Provider) callAddress(ctx context.Context, client *ethclient.Client, contract common.Address, method string, args ...any) (common.Address, error) {
	raw, err := p.call(ctx, client, contract, p.stakingABI, method, args...)
	if err != nil {
		return common.Address{}, err
	}
	value, ok := raw.(common.Address)
	if !ok {
		return common.Address{}, clierr.New(clierr.CodeUnavailable, fmt.Sprintf("unexpected %s output type", method))
	}
	return value, nil
}

func (p *Provider) callERC20TotalSupply(ctx context.Context, client *ethclient.Client, contract common.Address) (*big.Int, error) {
	raw, err := p.call(ctx, client, contract, p.erc20ABI, "totalSupply")
	if err != nil {
		return nil, err
	}
	value, ok := raw.(*big.Int)
	if !ok {
		return nil, clierr.New(clierr.CodeUnavailable, "unexpected totalSupply output type")
	}
	return value, nil
}

func (p *Provider) call(ctx context.Context, client *ethclient.Client, contract common.Address, abiSpec abi.ABI, method string, args ...any) (any, error) {
	data, err := abiSpec.Pack(method, args...)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, fmt.Sprintf("encode meth %s call", method), err)
	}
	msg := geth.CallMsg{To: &contract, Data: data}
	raw, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, fmt.Sprintf("meth %s call failed", method), err)
	}
	out, err := abiSpec.Unpack(method, raw)
	if err != nil || len(out) != 1 {
		return nil, clierr.Wrap(clierr.CodeUnavailable, fmt.Sprintf("decode meth %s response", method), err)
	}
	return out[0], nil
}

func resolveL1Config(cfg Config) (string, common.Address, error) {
	network := normalizeNetwork(cfg.Network)
	if network == "" {
		network = normalizeNetwork(os.Getenv("MANTLE_NETWORK"))
	}
	if network == "" {
		network = "mainnet"
	}

	rpcURL := strings.TrimSpace(cfg.EthereumRPCURL)
	if rpcURL == "" {
		rpcURL = strings.TrimSpace(os.Getenv("MANTLE_ETHEREUM_RPC_URL"))
	}
	if rpcURL == "" {
		switch network {
		case "sepolia":
			rpcURL = defaultSepoliaEthereumRPC
		default:
			rpcURL = defaultMainnetEthereumRPC
		}
	}

	stakingAddress := strings.TrimSpace(os.Getenv("MANTLE_METH_STAKING_ADDRESS"))
	if stakingAddress == "" {
		switch network {
		case "mainnet":
			stakingAddress = mainnetStakingAddress
		case "sepolia":
			stakingAddress = sepoliaStakingAddress
		default:
			return "", common.Address{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("unsupported network for meth staking: %s", network))
		}
	}
	if !common.IsHexAddress(stakingAddress) {
		return "", common.Address{}, clierr.New(clierr.CodeUsage, "invalid mETH staking address")
	}
	return rpcURL, common.HexToAddress(stakingAddress), nil
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

var _ providers.StakingProvider = (*Provider)(nil)

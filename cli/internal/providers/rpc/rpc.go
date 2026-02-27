package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/model"
	"github.com/mantle/mantle-ai/cli/internal/providers"
)

const erc20ABIJSON = `[
  {"type":"function","name":"name","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"string"}]},
  {"type":"function","name":"symbol","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"string"}]},
  {"type":"function","name":"decimals","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint8"}]},
  {"type":"function","name":"totalSupply","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"balanceOf","stateMutability":"view","inputs":[{"name":"owner","type":"address"}],"outputs":[{"name":"","type":"uint256"}]}
]`

type Config struct {
	Network string
	RPCURL  string
}

type Provider struct {
	client      *ethclient.Client
	network     string
	chainConfig model.ChainInfo
	erc20ABI    abi.ABI
}

func New(cfg Config) (*Provider, error) {
	if strings.TrimSpace(cfg.RPCURL) == "" {
		return nil, clierr.New(clierr.CodeUsage, "rpc url is required")
	}
	chainCfg, err := chainInfoByNetwork(cfg.Network, cfg.RPCURL)
	if err != nil {
		return nil, err
	}
	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "connect rpc provider", err)
	}
	erc20ABI, err := abi.JSON(strings.NewReader(erc20ABIJSON))
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, "parse erc20 abi", err)
	}
	return &Provider{
		client:      client,
		network:     cfg.Network,
		chainConfig: chainCfg,
		erc20ABI:    erc20ABI,
	}, nil
}

func (p *Provider) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

func (p *Provider) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:           "rpc",
		Type:           "onchain",
		Enabled:        true,
		RequiresKey:    false,
		AuthConfigured: true,
		Capabilities: []string{
			"chain.info",
			"chain.status",
			"balance.get",
			"tx.get",
			"contract.read",
			"contract.call.simulate",
			"token.info",
			"token.resolve",
			"token.balances",
		},
	}
}

func (p *Provider) ChainInfo(_ context.Context) (model.ChainInfo, error) {
	return p.chainConfig, nil
}

func (p *Provider) ChainStatus(ctx context.Context) (model.ChainStatus, error) {
	blockNumber, err := p.client.BlockNumber(ctx)
	if err != nil {
		return model.ChainStatus{}, clierr.Wrap(clierr.CodeUnavailable, "get block number", err)
	}
	gasPrice, err := p.client.SuggestGasPrice(ctx)
	if err != nil {
		return model.ChainStatus{}, clierr.Wrap(clierr.CodeUnavailable, "get gas price", err)
	}

	syncStatus := "synced"
	progress, err := p.client.SyncProgress(ctx)
	if err == nil && progress != nil {
		syncStatus = "syncing"
	}

	peerCount := 0
	if p.client.Client() != nil {
		var result string
		if err := p.client.Client().CallContext(ctx, &result, "net_peerCount"); err == nil {
			if strings.HasPrefix(result, "0x") {
				if n, convErr := strconv.ParseInt(strings.TrimPrefix(result, "0x"), 16, 64); convErr == nil {
					peerCount = int(n)
				}
			}
		}
	}

	return model.ChainStatus{
		ChainID:     p.chainConfig.ChainID,
		BlockNumber: blockNumber,
		GasPrice:    gasPrice.String(),
		GasPriceMNT: id.FormatDecimalCompat(gasPrice.String(), 18),
		L1DataFee:   "0",
		SyncStatus:  syncStatus,
		PeerCount:   peerCount,
	}, nil
}

func (p *Provider) GetBalance(ctx context.Context, address string) (model.AddressBalance, error) {
	if !common.IsHexAddress(address) {
		return model.AddressBalance{}, clierr.New(clierr.CodeUsage, "invalid address")
	}
	wallet := common.HexToAddress(address)
	balance, err := p.client.BalanceAt(ctx, wallet, nil)
	if err != nil {
		return model.AddressBalance{}, clierr.Wrap(clierr.CodeUnavailable, "get native balance", err)
	}

	tokens, err := p.GetTokenBalances(ctx, address)
	if err != nil {
		return model.AddressBalance{}, err
	}

	return model.AddressBalance{
		Address: strings.ToLower(wallet.Hex()),
		MNTBalance: model.AmountInfo{
			AmountBaseUnits: balance.String(),
			AmountDecimal:   id.FormatDecimalCompat(balance.String(), 18),
			Decimals:        18,
		},
		Tokens: tokens,
	}, nil
}

func (p *Provider) GetTransaction(ctx context.Context, hash string) (model.TransactionInfo, error) {
	if !regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`).MatchString(strings.TrimSpace(hash)) {
		return model.TransactionInfo{}, clierr.New(clierr.CodeUsage, "invalid transaction hash")
	}
	txHash := common.HexToHash(hash)
	tx, _, err := p.client.TransactionByHash(ctx, txHash)
	unsupportedType := err != nil && strings.Contains(strings.ToLower(err.Error()), "transaction type not supported")
	if err != nil && !unsupportedType {
		return model.TransactionInfo{}, clierr.Wrap(clierr.CodeUnavailable, "get transaction", err)
	}

	from := ""
	to := ""
	value := big.NewInt(0)
	gasPriceFromTx := big.NewInt(0)
	input := "0x"

	if unsupportedType {
		rawTx, rawErr := p.getTransactionRaw(ctx, txHash)
		if rawErr != nil {
			return model.TransactionInfo{}, clierr.Wrap(clierr.CodeUnavailable, "get transaction", rawErr)
		}
		from = strings.ToLower(rawTx.From)
		if rawTx.To != nil {
			to = strings.ToLower(*rawTx.To)
		}
		value, err = parseRPCQuantityBigInt(rawTx.Value)
		if err != nil {
			return model.TransactionInfo{}, clierr.Wrap(clierr.CodeUnavailable, "parse transaction value", err)
		}
		gasPriceFromTx, err = parseRPCQuantityBigInt(rawTx.GasPrice)
		if err != nil {
			return model.TransactionInfo{}, clierr.Wrap(clierr.CodeUnavailable, "parse transaction gas price", err)
		}
		if strings.TrimSpace(rawTx.Input) != "" {
			input = rawTx.Input
		}
	} else {
		if signer := types.LatestSignerForChainID(tx.ChainId()); signer != nil {
			if sender, senderErr := types.Sender(signer, tx); senderErr == nil {
				from = strings.ToLower(sender.Hex())
			}
		}
		if tx.To() != nil {
			to = strings.ToLower(tx.To().Hex())
		}
		value = tx.Value()
		gasPriceFromTx = tx.GasPrice()
		input = fmt.Sprintf("0x%x", tx.Data())
	}

	receipt, err := p.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return model.TransactionInfo{}, clierr.Wrap(clierr.CodeUnavailable, "get transaction receipt", err)
	}

	timestamp, err := p.getBlockTimestamp(ctx, receipt.BlockNumber)
	if err != nil {
		return model.TransactionInfo{}, clierr.Wrap(clierr.CodeUnavailable, "get transaction block", err)
	}

	gasPrice := receipt.EffectiveGasPrice
	if gasPrice == nil {
		gasPrice = gasPriceFromTx
	}
	if gasPrice == nil {
		gasPrice = big.NewInt(0)
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), gasPrice)

	status := "reverted"
	if receipt.Status == types.ReceiptStatusSuccessful {
		status = "success"
	}

	return model.TransactionInfo{
		Hash: txHash.Hex(),
		From: from,
		To:   to,
		Value: model.AmountInfo{
			AmountBaseUnits: value.String(),
			AmountDecimal:   id.FormatDecimalCompat(value.String(), 18),
			Decimals:        18,
		},
		GasUsed:     strconv.FormatUint(receipt.GasUsed, 10),
		GasPrice:    gasPrice.String(),
		FeeMNT:      id.FormatDecimalCompat(fee.String(), 18),
		BlockNumber: receipt.BlockNumber.Uint64(),
		Timestamp:   timestamp,
		Status:      status,
		Input:       input,
		ExplorerURL: fmt.Sprintf("%s/tx/%s", p.chainConfig.ExplorerURL, txHash.Hex()),
	}, nil
}

type rawRPCTransaction struct {
	Hash     string  `json:"hash"`
	From     string  `json:"from"`
	To       *string `json:"to"`
	Value    string  `json:"value"`
	Input    string  `json:"input"`
	GasPrice string  `json:"gasPrice"`
}

func (p *Provider) getTransactionRaw(ctx context.Context, txHash common.Hash) (rawRPCTransaction, error) {
	if p.client == nil || p.client.Client() == nil {
		return rawRPCTransaction{}, clierr.New(clierr.CodeUnavailable, "rpc client unavailable")
	}

	var tx *rawRPCTransaction
	if err := p.client.Client().CallContext(ctx, &tx, "eth_getTransactionByHash", txHash.Hex()); err != nil {
		return rawRPCTransaction{}, err
	}
	if tx == nil {
		return rawRPCTransaction{}, clierr.New(clierr.CodeUnavailable, "transaction not found")
	}
	return *tx, nil
}

type rawRPCBlock struct {
	Timestamp string `json:"timestamp"`
}

func (p *Provider) getBlockTimestamp(ctx context.Context, blockNumber *big.Int) (time.Time, error) {
	if blockNumber == nil {
		return time.Time{}, clierr.New(clierr.CodeUnavailable, "transaction block number is missing")
	}
	if p.client == nil || p.client.Client() == nil {
		return time.Time{}, clierr.New(clierr.CodeUnavailable, "rpc client unavailable")
	}

	var block *rawRPCBlock
	if err := p.client.Client().CallContext(ctx, &block, "eth_getBlockByNumber", fmt.Sprintf("0x%x", blockNumber), false); err != nil {
		return time.Time{}, err
	}
	if block == nil {
		return time.Time{}, clierr.New(clierr.CodeUnavailable, "transaction block not found")
	}

	ts, err := parseRPCQuantityUint64(block.Timestamp)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(int64(ts), 0).UTC(), nil
}

func (p *Provider) ReadContract(ctx context.Context, req providers.ContractReadRequest) (model.ContractResult, error) {
	contract, method, parsedABI, methodName, hasOutputs, err := p.prepareContractCall(req.Address, req.Function, req.Args)
	if err != nil {
		return model.ContractResult{}, err
	}

	msg := geth.CallMsg{To: &contract, Data: method}
	result, err := p.client.CallContract(ctx, msg, nil)
	if err != nil {
		return model.ContractResult{}, clierr.Wrap(clierr.CodeUnsupported, "contract read reverted", err)
	}

	decoded := any(fmt.Sprintf("0x%x", result))
	if hasOutputs {
		unpacked, unpackErr := parsedABI.Unpack(methodName, result)
		if unpackErr == nil {
			if len(unpacked) == 1 {
				decoded = normalizeABIValue(unpacked[0])
			} else {
				vals := make([]any, 0, len(unpacked))
				for _, item := range unpacked {
					vals = append(vals, normalizeABIValue(item))
				}
				decoded = vals
			}
		}
	}

	return model.ContractResult{
		Address:  strings.ToLower(contract.Hex()),
		Function: req.Function,
		Result:   decoded,
	}, nil
}

func (p *Provider) SimulateCall(ctx context.Context, req providers.ContractCallRequest) (model.CallSimulation, error) {
	if !common.IsHexAddress(req.To) {
		return model.CallSimulation{}, clierr.New(clierr.CodeUsage, "invalid to address")
	}
	if req.From != "" && !common.IsHexAddress(req.From) {
		return model.CallSimulation{}, clierr.New(clierr.CodeUsage, "invalid from address")
	}
	abiSpec, methodName, err := parseFunctionSignature(req.Function, false)
	if err != nil {
		return model.CallSimulation{}, err
	}
	method := abiSpec.Methods[methodName]
	parsedInputs, err := parseMethodArgs(method.Inputs, req.Args)
	if err != nil {
		return model.CallSimulation{}, err
	}
	data, err := abiSpec.Pack(methodName, parsedInputs...)
	if err != nil {
		return model.CallSimulation{}, clierr.Wrap(clierr.CodeUsage, "encode function call", err)
	}

	to := common.HexToAddress(req.To)
	msg := geth.CallMsg{To: &to, Data: data}
	if req.From != "" {
		from := common.HexToAddress(req.From)
		msg.From = from
	}
	if strings.TrimSpace(req.Value) != "" {
		value, convErr := parseEtherDecimal(req.Value)
		if convErr != nil {
			return model.CallSimulation{}, convErr
		}
		msg.Value = value
	}

	gasEstimate, err := p.client.EstimateGas(ctx, msg)
	if err != nil {
		return model.CallSimulation{Success: false, Error: err.Error()}, nil
	}
	gasPrice, err := p.client.SuggestGasPrice(ctx)
	if err != nil {
		return model.CallSimulation{}, clierr.Wrap(clierr.CodeUnavailable, "estimate gas price", err)
	}
	feeWei := new(big.Int).Mul(new(big.Int).SetUint64(gasEstimate), gasPrice)

	callResult, callErr := p.client.CallContract(ctx, msg, nil)
	sim := model.CallSimulation{
		Success:     callErr == nil,
		GasEstimate: strconv.FormatUint(gasEstimate, 10),
		FeeMNT:      id.FormatDecimalCompat(feeWei.String(), 18),
		ReturnData:  fmt.Sprintf("0x%x", callResult),
	}
	if callErr != nil {
		sim.Error = callErr.Error()
	}
	return sim, nil
}

func (p *Provider) GetTokenInfo(ctx context.Context, address string) (model.TokenInfo, error) {
	if !common.IsHexAddress(address) {
		return model.TokenInfo{}, clierr.New(clierr.CodeUsage, "invalid token address")
	}
	contract := common.HexToAddress(address)

	name, err := p.callERC20String(ctx, contract, "name")
	if err != nil {
		return model.TokenInfo{}, err
	}
	symbol, err := p.callERC20String(ctx, contract, "symbol")
	if err != nil {
		return model.TokenInfo{}, err
	}
	decimals, err := p.callERC20Uint8(ctx, contract, "decimals")
	if err != nil {
		return model.TokenInfo{}, err
	}
	totalSupply, err := p.callERC20BigInt(ctx, contract, "totalSupply")
	if err != nil {
		return model.TokenInfo{}, err
	}

	return model.TokenInfo{
		Address:     strings.ToLower(contract.Hex()),
		Name:        name,
		Symbol:      symbol,
		Decimals:    int(decimals),
		TotalSupply: totalSupply.String(),
		ExplorerURL: fmt.Sprintf("%s/address/%s", p.chainConfig.ExplorerURL, contract.Hex()),
	}, nil
}

func (p *Provider) ResolveToken(_ context.Context, symbol string) (model.TokenResolution, error) {
	token, ok := id.ResolveTokenSymbol(chainIDFromNetwork(p.network), symbol)
	if !ok {
		return model.TokenResolution{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("token not found: %s", symbol))
	}
	return model.TokenResolution{
		Input:       symbol,
		Symbol:      token.Symbol,
		Address:     strings.ToLower(token.Address),
		Decimals:    token.Decimals,
		Unambiguous: true,
	}, nil
}

func (p *Provider) GetTokenBalances(ctx context.Context, address string) ([]model.TokenBalance, error) {
	if !common.IsHexAddress(address) {
		return nil, clierr.New(clierr.CodeUsage, "invalid address")
	}
	wallet := common.HexToAddress(address)
	chainID := chainIDFromNetwork(p.network)
	tokens := id.TokensForChain(chainID)
	balances := make([]model.TokenBalance, 0, len(tokens))
	for _, token := range tokens {
		contract := common.HexToAddress(token.Address)
		value, err := p.callERC20BigIntWithArg(ctx, contract, "balanceOf", wallet)
		if err != nil {
			continue
		}
		balances = append(balances, model.TokenBalance{
			Symbol:  token.Symbol,
			Address: strings.ToLower(token.Address),
			Balance: model.AmountInfo{
				AmountBaseUnits: value.String(),
				AmountDecimal:   id.FormatDecimalCompat(value.String(), token.Decimals),
				Decimals:        token.Decimals,
			},
		})
	}
	return balances, nil
}

func (p *Provider) callERC20String(ctx context.Context, contract common.Address, method string) (string, error) {
	output, err := p.callERC20(ctx, contract, method)
	if err != nil {
		return "", err
	}
	if len(output) != 1 {
		return "", clierr.New(clierr.CodeUnavailable, fmt.Sprintf("unexpected %s output", method))
	}
	v, ok := output[0].(string)
	if !ok {
		return "", clierr.New(clierr.CodeUnavailable, fmt.Sprintf("unexpected %s output type", method))
	}
	return v, nil
}

func (p *Provider) callERC20Uint8(ctx context.Context, contract common.Address, method string) (uint8, error) {
	output, err := p.callERC20(ctx, contract, method)
	if err != nil {
		return 0, err
	}
	if len(output) != 1 {
		return 0, clierr.New(clierr.CodeUnavailable, fmt.Sprintf("unexpected %s output", method))
	}
	v, ok := output[0].(uint8)
	if !ok {
		return 0, clierr.New(clierr.CodeUnavailable, fmt.Sprintf("unexpected %s output type", method))
	}
	return v, nil
}

func (p *Provider) callERC20BigInt(ctx context.Context, contract common.Address, method string) (*big.Int, error) {
	output, err := p.callERC20(ctx, contract, method)
	if err != nil {
		return nil, err
	}
	if len(output) != 1 {
		return nil, clierr.New(clierr.CodeUnavailable, fmt.Sprintf("unexpected %s output", method))
	}
	v, ok := output[0].(*big.Int)
	if !ok {
		return nil, clierr.New(clierr.CodeUnavailable, fmt.Sprintf("unexpected %s output type", method))
	}
	return v, nil
}

func (p *Provider) callERC20BigIntWithArg(ctx context.Context, contract common.Address, method string, arg common.Address) (*big.Int, error) {
	data, err := p.erc20ABI.Pack(method, arg)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, "pack erc20 method", err)
	}
	msg := geth.CallMsg{To: &contract, Data: data}
	res, err := p.client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "call erc20 contract", err)
	}
	out, err := p.erc20ABI.Unpack(method, res)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "decode erc20 response", err)
	}
	if len(out) != 1 {
		return nil, clierr.New(clierr.CodeUnavailable, "unexpected erc20 output")
	}
	v, ok := out[0].(*big.Int)
	if !ok {
		return nil, clierr.New(clierr.CodeUnavailable, "unexpected erc20 output type")
	}
	return v, nil
}

func (p *Provider) callERC20(ctx context.Context, contract common.Address, method string) ([]any, error) {
	data, err := p.erc20ABI.Pack(method)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, "pack erc20 method", err)
	}
	msg := geth.CallMsg{To: &contract, Data: data}
	res, err := p.client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "call erc20 contract", err)
	}
	out, err := p.erc20ABI.Unpack(method, res)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "decode erc20 response", err)
	}
	return out, nil
}

func chainInfoByNetwork(network, rpcURL string) (model.ChainInfo, error) {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "", "mainnet":
		return model.ChainInfo{
			ChainID:     5000,
			Name:        "Mantle Mainnet",
			NativeToken: "MNT",
			RPCURL:      rpcURL,
			ExplorerURL: "https://explorer.mantle.xyz",
			BridgeURL:   "https://bridge.mantle.xyz",
			DALayer:     "Ethereum blobs",
		}, nil
	case "sepolia":
		return model.ChainInfo{
			ChainID:     5003,
			Name:        "Mantle Sepolia",
			NativeToken: "MNT",
			RPCURL:      rpcURL,
			ExplorerURL: "https://explorer.sepolia.mantle.xyz",
			BridgeURL:   "https://bridge.sepolia.mantle.xyz",
			DALayer:     "Ethereum blobs",
		}, nil
	default:
		return model.ChainInfo{}, clierr.New(clierr.CodeUsage, fmt.Sprintf("unsupported network: %s", network))
	}
}

func chainIDFromNetwork(network string) string {
	if strings.EqualFold(strings.TrimSpace(network), "sepolia") {
		return "eip155:5003"
	}
	return "eip155:5000"
}

func parseRPCQuantityBigInt(raw string) (*big.Int, error) {
	norm := strings.TrimSpace(raw)
	if norm == "" {
		return big.NewInt(0), nil
	}

	base := 10
	if strings.HasPrefix(norm, "0x") || strings.HasPrefix(norm, "0X") {
		base = 16
		norm = norm[2:]
	}
	if norm == "" {
		return big.NewInt(0), nil
	}

	v, ok := new(big.Int).SetString(norm, base)
	if !ok {
		return nil, clierr.New(clierr.CodeUnavailable, fmt.Sprintf("invalid rpc quantity: %s", raw))
	}
	return v, nil
}

func parseRPCQuantityUint64(raw string) (uint64, error) {
	norm := strings.TrimSpace(raw)
	if norm == "" {
		return 0, nil
	}

	base := 10
	if strings.HasPrefix(norm, "0x") || strings.HasPrefix(norm, "0X") {
		base = 16
		norm = norm[2:]
	}
	if norm == "" {
		return 0, nil
	}

	v, err := strconv.ParseUint(norm, base, 64)
	if err != nil {
		return 0, clierr.New(clierr.CodeUnavailable, fmt.Sprintf("invalid rpc quantity: %s", raw))
	}
	return v, nil
}

func parseEtherDecimal(input string) (*big.Int, error) {
	norm := strings.TrimSpace(input)
	if norm == "" {
		return big.NewInt(0), nil
	}
	f, ok := new(big.Float).SetString(norm)
	if !ok {
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("invalid value amount: %s", input))
	}
	weiFloat := new(big.Float).Mul(f, new(big.Float).SetFloat64(math.Pow10(18)))
	wei := new(big.Int)
	weiFloat.Int(wei)
	return wei, nil
}

var bareSignaturePattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\((.*)\)$`)

func parseFunctionSignature(signature string, isView bool) (abi.ABI, string, error) {
	trimmed := strings.TrimSpace(signature)
	if trimmed == "" {
		return abi.ABI{}, "", clierr.New(clierr.CodeUsage, "function signature is required")
	}

	stateMutability := "nonpayable"
	if isView {
		stateMutability = "view"
	}

	if strings.HasPrefix(trimmed, "function ") {
		withoutPrefix := strings.TrimPrefix(trimmed, "function ")
		return buildABIFromFunctionDecl(withoutPrefix)
	}
	return buildABIFromBareSignature(trimmed, stateMutability)
}

func buildABIFromBareSignature(sig, stateMutability string) (abi.ABI, string, error) {
	m := bareSignaturePattern.FindStringSubmatch(strings.TrimSpace(sig))
	if len(m) != 3 {
		return abi.ABI{}, "", clierr.New(clierr.CodeUsage, fmt.Sprintf("invalid function signature: %s", sig))
	}
	name := m[1]
	inputTypes := splitAndTrim(m[2])
	inputs := make([]map[string]string, 0, len(inputTypes))
	for i, typ := range inputTypes {
		if typ == "" {
			continue
		}
		inputs = append(inputs, map[string]string{"name": fmt.Sprintf("arg%d", i), "type": typ})
	}
	items := []map[string]any{{
		"type":            "function",
		"name":            name,
		"stateMutability": stateMutability,
		"inputs":          inputs,
		"outputs":         []map[string]string{},
	}}
	buf, _ := json.Marshal(items)
	parsed, err := abi.JSON(strings.NewReader(string(buf)))
	if err != nil {
		return abi.ABI{}, "", clierr.Wrap(clierr.CodeUsage, "parse function signature", err)
	}
	return parsed, name, nil
}

func buildABIFromFunctionDecl(decl string) (abi.ABI, string, error) {
	head := strings.TrimSpace(decl)
	outputs := []map[string]string{}
	stateMutability := "nonpayable"

	if strings.Contains(head, " view ") || strings.HasSuffix(head, " view") {
		stateMutability = "view"
	}
	if strings.Contains(head, " pure ") || strings.HasSuffix(head, " pure") {
		stateMutability = "pure"
	}

	if idx := strings.Index(head, "returns"); idx >= 0 {
		retPart := strings.TrimSpace(head[idx+len("returns"):])
		head = strings.TrimSpace(head[:idx])
		retPart = strings.TrimSpace(strings.TrimPrefix(retPart, "("))
		retPart = strings.TrimSpace(strings.TrimSuffix(retPart, ")"))
		for i, typ := range splitAndTrim(retPart) {
			if typ == "" {
				continue
			}
			parts := strings.Fields(typ)
			outputs = append(outputs, map[string]string{"name": fmt.Sprintf("ret%d", i), "type": parts[0]})
		}
	}

	match := bareSignaturePattern.FindStringSubmatch(head)
	if len(match) != 3 {
		return abi.ABI{}, "", clierr.New(clierr.CodeUsage, fmt.Sprintf("invalid function declaration: %s", decl))
	}
	name := match[1]
	inputs := []map[string]string{}
	for i, item := range splitAndTrim(match[2]) {
		if item == "" {
			continue
		}
		parts := strings.Fields(item)
		typ := parts[0]
		inputs = append(inputs, map[string]string{"name": fmt.Sprintf("arg%d", i), "type": typ})
	}
	if outputs == nil {
		outputs = []map[string]string{}
	}
	items := []map[string]any{{
		"type":            "function",
		"name":            name,
		"stateMutability": stateMutability,
		"inputs":          inputs,
		"outputs":         outputs,
	}}
	buf, _ := json.Marshal(items)
	parsed, err := abi.JSON(strings.NewReader(string(buf)))
	if err != nil {
		return abi.ABI{}, "", clierr.Wrap(clierr.CodeUsage, "parse function declaration", err)
	}
	return parsed, name, nil
}

func splitAndTrim(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		norm := strings.TrimSpace(part)
		if norm != "" {
			out = append(out, norm)
		}
	}
	return out
}

func parseMethodArgs(args abi.Arguments, raw []string) ([]any, error) {
	if len(args) != len(raw) {
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("expected %d args, got %d", len(args), len(raw)))
	}
	out := make([]any, 0, len(args))
	for i, arg := range args {
		v, err := parseArgValue(arg.Type, raw[i])
		if err != nil {
			return nil, clierr.Wrap(clierr.CodeUsage, fmt.Sprintf("parse arg %d", i), err)
		}
		out = append(out, v)
	}
	return out, nil
}

func parseArgValue(typ abi.Type, raw string) (any, error) {
	switch typ.T {
	case abi.AddressTy:
		if !common.IsHexAddress(raw) {
			return nil, fmt.Errorf("invalid address %s", raw)
		}
		return common.HexToAddress(raw), nil
	case abi.StringTy:
		return raw, nil
	case abi.BoolTy:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, err
		}
		return v, nil
	case abi.BytesTy:
		if strings.HasPrefix(raw, "0x") {
			return common.FromHex(raw), nil
		}
		return []byte(raw), nil
	case abi.FixedBytesTy:
		data := common.FromHex(raw)
		if len(data) == 0 && raw != "" && !strings.HasPrefix(raw, "0x") {
			data = []byte(raw)
		}
		if len(data) != typ.Size {
			return nil, fmt.Errorf("expected %d bytes, got %d", typ.Size, len(data))
		}
		arr := make([]byte, typ.Size)
		copy(arr, data)
		return arr, nil
	case abi.UintTy, abi.IntTy:
		base := 10
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "0x") {
			base = 16
			trimmed = strings.TrimPrefix(trimmed, "0x")
		}
		bigVal, ok := new(big.Int).SetString(trimmed, base)
		if !ok {
			return nil, fmt.Errorf("invalid integer %s", raw)
		}
		if typ.T == abi.UintTy && bigVal.Sign() < 0 {
			return nil, fmt.Errorf("negative value for uint: %s", raw)
		}
		if typ.Size <= 64 {
			if typ.T == abi.UintTy {
				return bigVal.Uint64(), nil
			}
			return bigVal.Int64(), nil
		}
		return bigVal, nil
	default:
		return nil, fmt.Errorf("unsupported arg type %s", typ.String())
	}
}

func normalizeABIValue(v any) any {
	switch t := v.(type) {
	case *big.Int:
		if t == nil {
			return "0"
		}
		return t.String()
	case common.Address:
		return strings.ToLower(t.Hex())
	case []byte:
		return fmt.Sprintf("0x%x", t)
	default:
		return t
	}
}

func (p *Provider) prepareContractCall(address, function string, args []string) (common.Address, []byte, abi.ABI, string, bool, error) {
	if !common.IsHexAddress(address) {
		return common.Address{}, nil, abi.ABI{}, "", false, clierr.New(clierr.CodeUsage, "invalid contract address")
	}
	contract := common.HexToAddress(address)
	abiSpec, methodName, err := parseFunctionSignature(function, true)
	if err != nil {
		return common.Address{}, nil, abi.ABI{}, "", false, err
	}
	method := abiSpec.Methods[methodName]
	parsedInputs, err := parseMethodArgs(method.Inputs, args)
	if err != nil {
		return common.Address{}, nil, abi.ABI{}, "", false, err
	}
	data, err := abiSpec.Pack(methodName, parsedInputs...)
	if err != nil {
		return common.Address{}, nil, abi.ABI{}, "", false, clierr.Wrap(clierr.CodeUsage, "encode function call", err)
	}
	hasOutputs := len(method.Outputs) > 0
	return contract, data, abiSpec, methodName, hasOutputs, nil
}

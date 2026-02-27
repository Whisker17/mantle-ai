package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type rpcReq struct {
	ID      any    `json:"id"`
	Method  string `json:"method"`
	JSONRPC string `json:"jsonrpc"`
}

type rpcResp struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func mockRPCServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req rpcReq
		_ = json.NewDecoder(r.Body).Decode(&req)

		result := any(nil)
		switch req.Method {
		case "eth_blockNumber":
			result = "0x10"
		case "eth_gasPrice":
			result = "0x3b9aca00"
		case "eth_syncing":
			result = false
		case "net_peerCount":
			result = "0x2"
		case "eth_getBalance":
			result = "0xde0b6b3a7640000"
		case "eth_call":
			result = "0x0000000000000000000000000000000000000000000000000000000000000000"
		default:
			result = "0x0"
		}

		_ = json.NewEncoder(w).Encode(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: result})
	}))
}

func TestChainStatusWithMockRPC(t *testing.T) {
	srv := mockRPCServer()
	defer srv.Close()

	provider, err := New(Config{Network: "mainnet", RPCURL: srv.URL})
	if err != nil {
		t.Fatalf("New provider failed: %v", err)
	}
	defer provider.Close()

	status, err := provider.ChainStatus(context.Background())
	if err != nil {
		t.Fatalf("ChainStatus failed: %v", err)
	}
	if status.BlockNumber != 16 {
		t.Fatalf("expected block 16, got %d", status.BlockNumber)
	}
	if status.ChainID != 5000 {
		t.Fatalf("expected chain 5000, got %d", status.ChainID)
	}
}

func TestGetBalanceWithMockRPC(t *testing.T) {
	srv := mockRPCServer()
	defer srv.Close()

	provider, err := New(Config{Network: "mainnet", RPCURL: srv.URL})
	if err != nil {
		t.Fatalf("New provider failed: %v", err)
	}
	defer provider.Close()

	balance, err := provider.GetBalance(context.Background(), "0x1111111111111111111111111111111111111111")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	if balance.MNTBalance.AmountBaseUnits == "" {
		t.Fatal("expected balance amount")
	}
}

func TestChainInfoDAReflectsEthereumBlobs(t *testing.T) {
	mainnet, err := chainInfoByNetwork("mainnet", "https://example-rpc")
	if err != nil {
		t.Fatalf("mainnet chain info failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(mainnet.DALayer), "blob") {
		t.Fatalf("expected mainnet DA layer to mention blobs, got %q", mainnet.DALayer)
	}
	if strings.TrimSpace(mainnet.Source) == "" {
		t.Fatalf("expected mainnet source to be set, got %q", mainnet.Source)
	}

	sepolia, err := chainInfoByNetwork("sepolia", "https://example-rpc")
	if err != nil {
		t.Fatalf("sepolia chain info failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(sepolia.DALayer), "blob") {
		t.Fatalf("expected sepolia DA layer to mention blobs, got %q", sepolia.DALayer)
	}
	if strings.TrimSpace(sepolia.Source) == "" {
		t.Fatalf("expected sepolia source to be set, got %q", sepolia.Source)
	}
}

func TestGetTransactionFallsBackForUnsupportedTxType(t *testing.T) {
	const (
		txHash      = "0x1111111111111111111111111111111111111111111111111111111111111111"
		blockHash   = "0x2222222222222222222222222222222222222222222222222222222222222222"
		fromAddress = "0xdeaddeaddeaddeaddeaddeaddeaddeaddead0001"
		toAddress   = "0x4200000000000000000000000000000000000015"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req rpcReq
		_ = json.NewDecoder(r.Body).Decode(&req)

		var result any
		switch req.Method {
		case "eth_getTransactionByHash":
			result = map[string]any{
				"hash":      txHash,
				"type":      "0x7e",
				"from":      fromAddress,
				"to":        toAddress,
				"value":     "0x2386f26fc10000",
				"gasPrice":  "0x0",
				"input":     "0x1234",
				"blockHash": blockHash,
			}
		case "eth_getTransactionReceipt":
			result = map[string]any{
				"transactionHash":   txHash,
				"transactionIndex":  "0x0",
				"blockHash":         blockHash,
				"blockNumber":       "0x10",
				"from":              fromAddress,
				"to":                toAddress,
				"cumulativeGasUsed": "0x5208",
				"gasUsed":           "0x5208",
				"effectiveGasPrice": "0x3b9aca00",
				"logs":              []any{},
				"logsBloom":         "0x" + strings.Repeat("0", 512),
				"status":            "0x1",
				"type":              "0x7e",
			}
		case "eth_getBlockByNumber":
			result = map[string]any{
				"timestamp": "0x65f15b80",
			}
		default:
			result = "0x0"
		}

		_ = json.NewEncoder(w).Encode(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: result})
	}))
	defer srv.Close()

	provider, err := New(Config{Network: "mainnet", RPCURL: srv.URL})
	if err != nil {
		t.Fatalf("New provider failed: %v", err)
	}
	defer provider.Close()

	info, err := provider.GetTransaction(context.Background(), txHash)
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}
	if info.Hash != txHash {
		t.Fatalf("expected hash %s, got %s", txHash, info.Hash)
	}
	if info.From != fromAddress {
		t.Fatalf("expected from %s, got %s", fromAddress, info.From)
	}
	if info.To != toAddress {
		t.Fatalf("expected to %s, got %s", toAddress, info.To)
	}
	if info.Value.AmountBaseUnits != "10000000000000000" {
		t.Fatalf("expected value 10000000000000000, got %s", info.Value.AmountBaseUnits)
	}
	if info.GasPrice != "1000000000" {
		t.Fatalf("expected gas price 1000000000, got %s", info.GasPrice)
	}
	if info.GasUsed != "21000" {
		t.Fatalf("expected gas used 21000, got %s", info.GasUsed)
	}
	if info.Status != "success" {
		t.Fatalf("expected success status, got %s", info.Status)
	}
	if info.Input != "0x1234" {
		t.Fatalf("expected input 0x1234, got %s", info.Input)
	}
	if info.BlockNumber != 16 {
		t.Fatalf("expected block number 16, got %d", info.BlockNumber)
	}
	expectedTimestamp := time.Unix(1710316416, 0).UTC()
	if !info.Timestamp.Equal(expectedTimestamp) {
		t.Fatalf("expected timestamp %s, got %s", expectedTimestamp, info.Timestamp)
	}
}

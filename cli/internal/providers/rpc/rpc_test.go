package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	sepolia, err := chainInfoByNetwork("sepolia", "https://example-rpc")
	if err != nil {
		t.Fatalf("sepolia chain info failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(sepolia.DALayer), "blob") {
		t.Fatalf("expected sepolia DA layer to mention blobs, got %q", sepolia.DALayer)
	}
}

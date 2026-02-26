package meth

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestStakeInfoUsesOnChainData(t *testing.T) {
	defillama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pools" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": []map[string]any{
				{
					"chain":   "Mantle",
					"project": "meth",
					"symbol":  "mETH",
					"apy":     4.2,
				},
			},
		})
	}))
	defer defillama.Close()

	ethRPC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			ID      any    `json:"id"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			JSONRPC string `json:"jsonrpc"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		result := "0x0"
		switch req.Method {
		case "eth_call":
			call := req.Params[0].(map[string]any)
			data := ""
			if raw, ok := call["data"].(string); ok {
				data = raw
			}
			if data == "" {
				if raw, ok := call["input"].(string); ok {
					data = raw
				}
			}
			data = strings.ToLower(strings.TrimSpace(data))
			switch {
			case strings.HasPrefix(data, methodSelector("mETH()")):
				result = encodeAddress("0xd5F7838F5C461fefF7FE49ea5ebaF7728bB0ADfa")
			case strings.HasPrefix(data, methodSelector("ethToMETH(uint256)")):
				result = encodeUint(new(big.Int).SetUint64(950000000000000000))
			case strings.HasPrefix(data, methodSelector("mETHToETH(uint256)")):
				result = encodeUint(new(big.Int).SetUint64(1050000000000000000))
			case strings.HasPrefix(data, methodSelector("totalControlled()")):
				result = encodeUint(new(big.Int).SetUint64(1230000000000000000))
			case strings.HasPrefix(data, methodSelector("totalSupply()")):
				result = encodeUint(new(big.Int).SetUint64(1170000000000000000))
			}
		case "eth_chainId":
			result = "0x1"
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	}))
	defer ethRPC.Close()

	t.Setenv("MANTLE_ETHEREUM_RPC_URL", ethRPC.URL)

	provider := New(Config{BaseURL: defillama.URL, Timeout: time.Second, Retries: 0})
	info, err := provider.StakeInfo(context.Background())
	if err != nil {
		t.Fatalf("StakeInfo failed: %v", err)
	}

	if info.METHToETH == "1" || info.ETHToMETH == "1" || info.TotalStaked == "0" || info.TotalMETH == "0" {
		t.Fatalf("expected on-chain stake data, got placeholder values: %+v", info)
	}
	if info.METHToETH != "1.05" {
		t.Fatalf("expected mETH->ETH rate 1.05, got %s", info.METHToETH)
	}
	if info.ETHToMETH != "0.95" {
		t.Fatalf("expected ETH->mETH rate 0.95, got %s", info.ETHToMETH)
	}
	if info.TotalStaked != "1.23" {
		t.Fatalf("expected total_staked 1.23, got %s", info.TotalStaked)
	}
	if info.TotalMETH != "1.17" {
		t.Fatalf("expected total_meth 1.17, got %s", info.TotalMETH)
	}
	if info.APY != 4.2 {
		t.Fatalf("expected APY 4.2, got %f", info.APY)
	}
}

func methodSelector(signature string) string {
	return "0x" + fmt.Sprintf("%x", crypto.Keccak256([]byte(signature))[:4])
}

func encodeUint(v *big.Int) string {
	return "0x" + fmt.Sprintf("%064x", v)
}

func encodeAddress(addr string) string {
	norm := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(addr)), "0x")
	return "0x" + strings.Repeat("0", 64-len(norm)) + norm
}

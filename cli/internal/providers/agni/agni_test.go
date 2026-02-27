package agni

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/providers"
)

func TestAddressesForNetworkSepoliaConfigured(t *testing.T) {
	quoter, router, err := addressesForNetwork("sepolia")
	if err != nil {
		t.Fatalf("expected sepolia to be configured, got err: %v", err)
	}
	if quoter != (commonZeroAddress()) {
		t.Fatalf("expected no quoter address on sepolia, got %s", quoter.Hex())
	}
	if strings.ToLower(router.Hex()) != "0x3e30894aaeb2ba741b8e2999604d1d01ff6244ea" {
		t.Fatalf("unexpected sepolia router address: %s", router.Hex())
	}
}

func TestQuoteSwapSepoliaRouterMode(t *testing.T) {
	const selectorGetAmountsOut = "0xd06ca61f"
	amountIn := big.NewInt(1_000_000)
	amountOut := big.NewInt(3_100_000_000_000_000_000)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			ID      any               `json:"id"`
			Method  string            `json:"method"`
			Params  []json.RawMessage `json:"params"`
			JSONRPC string            `json:"jsonrpc"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		result := any("0x0")
		switch req.Method {
		case "eth_gasPrice":
			result = "0x3b9aca00"
		case "eth_call":
			var callArgs struct {
				To   string `json:"to"`
				Data string `json:"data"`
				Input string `json:"input"`
			}
			if len(req.Params) > 0 {
				_ = json.Unmarshal(req.Params[0], &callArgs)
			}
			payload := callArgs.Data
			if strings.TrimSpace(payload) == "" {
				payload = callArgs.Input
			}
			if strings.HasPrefix(strings.ToLower(payload), selectorGetAmountsOut) {
				// ABI-encoded uint256[] with two entries: [amountIn, amountOut].
				result = "0x" +
					fmt.Sprintf("%064x", big.NewInt(32)) +
					fmt.Sprintf("%064x", big.NewInt(2)) +
					fmt.Sprintf("%064x", amountIn) +
					fmt.Sprintf("%064x", amountOut)
			} else {
				result = "0x"
			}
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	}))
	defer srv.Close()

	provider, err := New(Config{Network: "sepolia", RPCURL: srv.URL})
	if err != nil {
		t.Fatalf("New provider failed: %v", err)
	}
	defer provider.Close()

	quote, err := provider.QuoteSwap(context.Background(), providers.SwapQuoteRequest{
		FromAsset: id.Asset{
			Symbol:   "USDC",
			Address:  "0xacab8129e2ce587fd203fd770ec9ecafa2c88080",
			Decimals: 6,
		},
		ToAsset: id.Asset{
			Symbol:   "WMNT",
			Address:  "0x67A1f4A939b477A6b7c5BF94D97E45dE87E608eF",
			Decimals: 18,
		},
		AmountBaseUnits: amountIn.String(),
		AmountDecimal:   "1",
		FeeTier:         3000,
	})
	if err != nil {
		t.Fatalf("QuoteSwap failed: %v", err)
	}

	if quote.Provider != "agni" {
		t.Fatalf("expected provider agni, got %s", quote.Provider)
	}
	if quote.EstimatedOut.AmountBaseUnits != amountOut.String() {
		t.Fatalf("expected amount out %s, got %s", amountOut.String(), quote.EstimatedOut.AmountBaseUnits)
	}
	if quote.RouterAddress != "0x3e30894aaeb2ba741b8e2999604d1d01ff6244ea" {
		t.Fatalf("unexpected router address %s", quote.RouterAddress)
	}
}

func TestQuoteSwapMainnetQuoterEncoding(t *testing.T) {
	const selectorQuoteExactInputSingleTuple = "0xc6a5026a"
	var seenSelector string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			ID      any               `json:"id"`
			Method  string            `json:"method"`
			Params  []json.RawMessage `json:"params"`
			JSONRPC string            `json:"jsonrpc"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		result := any("0x0")
		switch req.Method {
		case "eth_gasPrice":
			result = "0x3b9aca00"
		case "eth_call":
			var callArgs struct {
				To    string `json:"to"`
				Data  string `json:"data"`
				Input string `json:"input"`
			}
			if len(req.Params) > 0 {
				_ = json.Unmarshal(req.Params[0], &callArgs)
			}
			payload := callArgs.Data
			if strings.TrimSpace(payload) == "" {
				payload = callArgs.Input
			}
			if len(payload) >= 10 {
				seenSelector = strings.ToLower(payload[:10])
			}
			// ABI-encoded uint256 amountOut = 100.
			result = "0x" + fmt.Sprintf("%064x", big.NewInt(100))
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	}))
	defer srv.Close()

	provider, err := New(Config{Network: "mainnet", RPCURL: srv.URL})
	if err != nil {
		t.Fatalf("New provider failed: %v", err)
	}
	defer provider.Close()

	quote, err := provider.QuoteSwap(context.Background(), providers.SwapQuoteRequest{
		FromAsset: id.Asset{
			Symbol:   "USDC",
			Address:  "0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9",
			Decimals: 6,
		},
		ToAsset: id.Asset{
			Symbol:   "WMNT",
			Address:  "0x78c1b0C915c4FAA5FffA6CAbf0219DA63d7f4cb8",
			Decimals: 18,
		},
		AmountBaseUnits: "1000000",
		AmountDecimal:   "1",
		FeeTier:         3000,
	})
	if err != nil {
		t.Fatalf("QuoteSwap failed: %v", err)
	}
	if seenSelector != selectorQuoteExactInputSingleTuple {
		t.Fatalf("expected selector %s, got %s", selectorQuoteExactInputSingleTuple, seenSelector)
	}
	if quote.EstimatedOut.AmountBaseUnits != "100" {
		t.Fatalf("expected amount out 100, got %s", quote.EstimatedOut.AmountBaseUnits)
	}
}

func commonZeroAddress() common.Address {
	return common.Address{}
}

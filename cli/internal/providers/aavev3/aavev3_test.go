package aavev3

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mantle/mantle-ai/cli/internal/id"
)

func TestProviderReturnsLendMarketsAndRates(t *testing.T) {
	spec, err := abi.JSON(strings.NewReader(dataProviderABIJSON))
	if err != nil {
		t.Fatalf("parse abi: %v", err)
	}

	usdc := strings.ToLower("0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9")
	rayBase, _ := new(big.Int).SetString("1000000000000000000000000", 10)
	liqRate := big.NewInt(0).Mul(big.NewInt(5), rayBase)    // 5% in ray
	borrowRate := big.NewInt(0).Mul(big.NewInt(8), rayBase) // 8% in ray
	configSelector := strings.TrimPrefix(methodSelector("getReserveConfigurationData(address)"), "0x")
	dataSelector := strings.TrimPrefix(methodSelector("getReserveData(address)"), "0x")

	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			ID      any    `json:"id"`
			Method  string `json:"method"`
			Params  []any  `json:"params"`
			JSONRPC string `json:"jsonrpc"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		result := "0x00"
		switch req.Method {
		case "eth_chainId":
			result = "0x1388"
		case "eth_call":
			call, _ := req.Params[0].(map[string]any)
			rawData, _ := call["data"].(string)
			if rawData == "" {
				rawData, _ = call["input"].(string)
			}
			data := strings.ToLower(strings.TrimSpace(rawData))
			trimmed := strings.TrimPrefix(data, "0x")
			if len(trimmed) < 8 {
				break
			}
			sig := trimmed[:8]
			switch sig {
			case configSelector:
				addr := decodeAddressArg(data)
				decimals := big.NewInt(18)
				isActive := false
				if addr == usdc {
					decimals = big.NewInt(6)
					isActive = true
				}
				out, packErr := spec.Methods["getReserveConfigurationData"].Outputs.Pack(
					decimals,      // decimals
					big.NewInt(0), // ltv
					big.NewInt(0), // liquidationThreshold
					big.NewInt(0), // liquidationBonus
					big.NewInt(0), // reserveFactor
					true,          // usageAsCollateralEnabled
					true,          // borrowingEnabled
					false,         // stableBorrowRateEnabled
					isActive,      // isActive
					false,         // isFrozen
				)
				if packErr == nil {
					result = "0x" + hex.EncodeToString(out)
				}
			case dataSelector:
				addr := decodeAddressArg(data)
				totalAToken := big.NewInt(0)
				totalVariableDebt := big.NewInt(0)
				liquidityRate := big.NewInt(0)
				variableBorrowRate := big.NewInt(0)
				if addr == usdc {
					totalAToken = big.NewInt(2_500_000_000_000) // 2.5m USDC, 6 decimals
					totalVariableDebt = big.NewInt(1_250_000_000_000)
					liquidityRate = liqRate
					variableBorrowRate = borrowRate
				}
				out, packErr := spec.Methods["getReserveData"].Outputs.Pack(
					big.NewInt(0),                       // unbacked
					big.NewInt(0),                       // accruedToTreasuryScaled
					totalAToken,                         // totalAToken
					big.NewInt(0),                       // totalStableDebt
					totalVariableDebt,                   // totalVariableDebt
					liquidityRate,                       // liquidityRate
					variableBorrowRate,                  // variableBorrowRate
					big.NewInt(0),                       // stableBorrowRate
					big.NewInt(0),                       // averageStableBorrowRate
					big.NewInt(0),                       // liquidityIndex
					big.NewInt(0),                       // variableBorrowIndex
					big.NewInt(time.Now().UTC().Unix()), // lastUpdateTimestamp
				)
				if packErr == nil {
					result = "0x" + hex.EncodeToString(out)
				}
			}
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	}))
	defer rpc.Close()

	provider := New(Config{
		Network: "mainnet",
		RPCURL:  rpc.URL,
	})

	asset := id.Asset{Symbol: "USDC"}
	rates, err := provider.LendRates(context.Background(), asset)
	if err != nil {
		t.Fatalf("LendRates failed: %v", err)
	}
	if len(rates) == 0 {
		t.Fatalf("expected aave_v3 lend rates, got none")
	}
	if rates[0].Protocol != "aave_v3" {
		t.Fatalf("expected protocol aave_v3, got %s", rates[0].Protocol)
	}
	if rates[0].SupplyAPY <= 0 || rates[0].BorrowAPY <= 0 {
		t.Fatalf("expected positive rates, got %+v", rates[0])
	}

	markets, err := provider.LendMarkets(context.Background(), asset)
	if err != nil {
		t.Fatalf("LendMarkets failed: %v", err)
	}
	if len(markets) == 0 {
		t.Fatalf("expected aave_v3 lend markets, got none")
	}
	if markets[0].Protocol != "aave_v3" {
		t.Fatalf("expected protocol aave_v3, got %s", markets[0].Protocol)
	}
	if markets[0].SupplyAPY <= 0 || markets[0].BorrowAPY <= 0 {
		t.Fatalf("expected positive market rates, got %+v", markets[0])
	}
}

func methodSelector(signature string) string {
	return "0x" + hex.EncodeToString(crypto.Keccak256([]byte(signature))[:4])
}

func decodeAddressArg(data string) string {
	trimmed := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(data)), "0x")
	if len(trimmed) < 8+64 {
		return ""
	}
	encodedArg := trimmed[8 : 8+64]
	return strings.ToLower(common.HexToAddress("0x" + encodedArg[24:]).Hex())
}

func TestParseReserveOverrides(t *testing.T) {
	input := "USDC:0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9:6,WMNT:0x78c1b0C915c4FAA5FffA6CAbf0219DA63d7f4cb8:18"
	reserves, err := parseReserveOverrides(input)
	if err != nil {
		t.Fatalf("parseReserveOverrides failed: %v", err)
	}
	if len(reserves) != 2 {
		t.Fatalf("expected 2 reserves, got %d", len(reserves))
	}
	if reserves[0].Symbol != "USDC" || reserves[0].Decimals != 6 {
		t.Fatalf("unexpected first reserve: %+v", reserves[0])
	}
	if !strings.EqualFold(reserves[1].Address.Hex(), "0x78c1b0C915c4FAA5FffA6CAbf0219DA63d7f4cb8") {
		t.Fatalf("unexpected second reserve address: %s", reserves[1].Address.Hex())
	}
}

func TestParseReserveOverridesRejectsInvalidEntry(t *testing.T) {
	_, err := parseReserveOverrides("USDC:not-an-address:6")
	if err == nil {
		t.Fatal("expected parseReserveOverrides to fail for invalid address")
	}
}

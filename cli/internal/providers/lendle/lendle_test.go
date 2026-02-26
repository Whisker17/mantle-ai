package lendle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/providers/defillama"
)

func TestLendMarketsAndRates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(defillama.PoolsResponse{
			Status: "success",
			Data: []defillama.Pool{
				{Chain: "Mantle", Project: "lendle", Symbol: "USDC", APY: 8, APYBase: 6, TVLUSD: 2_500_000, UnderlyingTokens: []string{"0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9"}},
				{Chain: "Mantle", Project: "aurelius", Symbol: "USDT", APY: 7, APYBase: 5, TVLUSD: 900_000},
			},
		})
	}))
	defer srv.Close()

	provider := New(Config{BaseURL: srv.URL, Timeout: time.Second, Retries: 0})
	asset := id.Asset{Symbol: "USDC", Address: "0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9"}

	markets, err := provider.LendMarkets(context.Background(), asset)
	if err != nil {
		t.Fatalf("LendMarkets failed: %v", err)
	}
	if len(markets) != 1 {
		t.Fatalf("expected 1 market, got %d", len(markets))
	}
	if markets[0].Protocol != "lendle" {
		t.Fatalf("unexpected protocol: %s", markets[0].Protocol)
	}

	rates, err := provider.LendRates(context.Background(), asset)
	if err != nil {
		t.Fatalf("LendRates failed: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("expected 1 rate, got %d", len(rates))
	}
	if rates[0].BorrowAPY <= 0 {
		t.Fatalf("expected borrow apy > 0, got %f", rates[0].BorrowAPY)
	}
}

package defillama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantle/mantle-ai/cli/internal/id"
)

func TestFetchPoolsAndFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pools" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(PoolsResponse{
			Status: "success",
			Data: []Pool{
				{Chain: "Mantle", Project: "lendle", Symbol: "USDC", APY: 7.5, TVLUSD: 1_500_000, UnderlyingTokens: []string{"0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9"}},
				{Chain: "Ethereum", Project: "other", Symbol: "ETH", APY: 4.2, TVLUSD: 2_000_000},
			},
		})
	}))
	defer srv.Close()

	provider := New(Config{BaseURL: srv.URL, Timeout: time.Second, Retries: 0})
	pools, err := provider.FetchPools(context.Background())
	if err != nil {
		t.Fatalf("FetchPools failed: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(pools))
	}

	filtered := FilterPools(pools, "mantle", "lendle")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered pool, got %d", len(filtered))
	}
	asset := id.Asset{Symbol: "USDC", Address: "0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9"}
	if !MatchesAsset(filtered[0], asset) {
		t.Fatalf("expected asset to match filtered pool")
	}
}

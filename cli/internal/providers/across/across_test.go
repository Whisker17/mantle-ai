package across

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/providers"
)

func TestQuoteBridgeParsesSuggestedFees(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/suggested-fees" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"outputAmount":         "990000",
			"estimatedFillTimeSec": 420,
			"estimatedFeeUsd":      "1.25",
		})
	}))
	defer srv.Close()

	p := New(Config{
		BaseURL: srv.URL,
		Timeout: time.Second,
		Retries: 0,
	})
	req := providers.BridgeQuoteRequest{
		FromChain:     id.Chain{CAIP2: "eip155:1", EVMChainID: 1},
		ToChain:       id.Chain{CAIP2: "eip155:5000", EVMChainID: 5000},
		Asset:         id.Asset{Address: "0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9", Symbol: "USDC", Decimals: 6},
		AmountDecimal: "1",
	}
	quote, err := p.QuoteBridge(context.Background(), req)
	if err != nil {
		t.Fatalf("QuoteBridge failed: %v", err)
	}
	if quote.EstimatedOut.AmountBaseUnits != "990000" {
		t.Fatalf("unexpected output amount: %s", quote.EstimatedOut.AmountBaseUnits)
	}
	if quote.EstimatedTimeS != 420 {
		t.Fatalf("unexpected fill time: %d", quote.EstimatedTimeS)
	}
}

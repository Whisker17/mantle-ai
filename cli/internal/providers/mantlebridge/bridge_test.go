package mantlebridge

import (
	"context"
	"testing"

	"github.com/mantle/mantle-ai/cli/internal/id"
	"github.com/mantle/mantle-ai/cli/internal/providers"
)

func TestQuoteBridgeDirectionAffectsTime(t *testing.T) {
	p := New(Config{Network: "mainnet"})

	deposit, err := p.QuoteBridge(context.Background(), providers.BridgeQuoteRequest{
		FromChain:     id.Chain{CAIP2: "eip155:1", EVMChainID: 1},
		ToChain:       id.Chain{CAIP2: "eip155:5000", EVMChainID: 5000},
		Asset:         id.Asset{Symbol: "ETH", Decimals: 18},
		AmountDecimal: "1",
	})
	if err != nil {
		t.Fatalf("deposit quote failed: %v", err)
	}
	if deposit.EstimatedTimeS != 600 {
		t.Fatalf("expected deposit time 600, got %d", deposit.EstimatedTimeS)
	}

	withdrawal, err := p.QuoteBridge(context.Background(), providers.BridgeQuoteRequest{
		FromChain:     id.Chain{CAIP2: "eip155:5000", EVMChainID: 5000},
		ToChain:       id.Chain{CAIP2: "eip155:1", EVMChainID: 1},
		Asset:         id.Asset{Symbol: "ETH", Decimals: 18},
		AmountDecimal: "1",
	})
	if err != nil {
		t.Fatalf("withdrawal quote failed: %v", err)
	}
	if withdrawal.EstimatedTimeS != 604800 {
		t.Fatalf("expected withdrawal time 604800, got %d", withdrawal.EstimatedTimeS)
	}
}

func TestBridgeStatusRejectsInvalidHash(t *testing.T) {
	p := New(Config{Network: "mainnet"})
	if _, err := p.BridgeStatus(context.Background(), "0x1234"); err == nil {
		t.Fatalf("expected invalid hash error")
	}
}

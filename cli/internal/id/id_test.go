package id

import "testing"

func TestParseChainVariants(t *testing.T) {
	cases := []struct {
		input string
		caip2 string
	}{
		{input: "mantle", caip2: "eip155:5000"},
		{input: "5000", caip2: "eip155:5000"},
		{input: "eip155:5000", caip2: "eip155:5000"},
		{input: "sepolia", caip2: "eip155:5003"},
		{input: "5003", caip2: "eip155:5003"},
	}

	for _, tc := range cases {
		chain, err := ParseChain(tc.input)
		if err != nil {
			t.Fatalf("ParseChain(%s) failed: %v", tc.input, err)
		}
		if chain.CAIP2 != tc.caip2 {
			t.Fatalf("expected %s, got %s", tc.caip2, chain.CAIP2)
		}
	}
}

func TestParseAssetSymbolAddressAndCAIP19(t *testing.T) {
	chain, err := ParseChain("mantle")
	if err != nil {
		t.Fatalf("ParseChain failed: %v", err)
	}

	assetBySymbol, err := ParseAsset("USDC", chain)
	if err != nil {
		t.Fatalf("ParseAsset symbol failed: %v", err)
	}
	if assetBySymbol.Symbol != "USDC" {
		t.Fatalf("expected USDC symbol, got %s", assetBySymbol.Symbol)
	}

	assetByAddress, err := ParseAsset("0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9", chain)
	if err != nil {
		t.Fatalf("ParseAsset address failed: %v", err)
	}
	if assetByAddress.Symbol != "USDC" {
		t.Fatalf("expected USDC symbol from address, got %s", assetByAddress.Symbol)
	}

	assetByCAIP, err := ParseAsset("eip155:5000/erc20:0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9", chain)
	if err != nil {
		t.Fatalf("ParseAsset CAIP-19 failed: %v", err)
	}
	if assetByCAIP.Symbol != "USDC" {
		t.Fatalf("expected USDC symbol from CAIP-19, got %s", assetByCAIP.Symbol)
	}
}

func TestNormalizeAmount(t *testing.T) {
	base, dec, err := NormalizeAmount("1000000", "", 6)
	if err != nil {
		t.Fatalf("NormalizeAmount failed: %v", err)
	}
	if base != "1000000" || dec != "1" {
		t.Fatalf("unexpected output base=%s dec=%s", base, dec)
	}
}

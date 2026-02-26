package id

import (
	"fmt"
	"regexp"
	"strings"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
)

type Asset struct {
	ChainID  string
	AssetID  string
	Address  string
	Symbol   string
	Decimals int
}

var (
	evmAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	caip19Pattern     = regexp.MustCompile(`^eip155:[0-9]+/erc20:0x[0-9a-fA-F]{40}$`)
)

func ParseAsset(input string, chain Chain) (Asset, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Asset{}, clierr.New(clierr.CodeUsage, "asset is required")
	}
	norm := strings.ToLower(trimmed)

	if caip19Pattern.MatchString(norm) {
		parts := strings.Split(norm, "/")
		chainID := parts[0]
		if chainID != chain.CAIP2 {
			return Asset{}, clierr.New(clierr.CodeUsage, fmt.Sprintf("asset chain mismatch: got %s expected %s", chainID, chain.CAIP2))
		}
		address := strings.TrimPrefix(parts[1], "erc20:")
		if token, ok := ResolveTokenAddress(chain.CAIP2, address); ok {
			return Asset{
				ChainID:  chain.CAIP2,
				AssetID:  fmt.Sprintf("%s/erc20:%s", chain.CAIP2, strings.ToLower(token.Address)),
				Address:  strings.ToLower(token.Address),
				Symbol:   token.Symbol,
				Decimals: token.Decimals,
			}, nil
		}
		return Asset{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("token address not found in registry: %s", address))
	}

	if evmAddressPattern.MatchString(trimmed) {
		if token, ok := ResolveTokenAddress(chain.CAIP2, trimmed); ok {
			return Asset{
				ChainID:  chain.CAIP2,
				AssetID:  fmt.Sprintf("%s/erc20:%s", chain.CAIP2, strings.ToLower(token.Address)),
				Address:  strings.ToLower(token.Address),
				Symbol:   token.Symbol,
				Decimals: token.Decimals,
			}, nil
		}
		return Asset{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("token address not found in registry: %s", trimmed))
	}

	if token, ok := ResolveTokenSymbol(chain.CAIP2, trimmed); ok {
		return Asset{
			ChainID:  chain.CAIP2,
			AssetID:  fmt.Sprintf("%s/erc20:%s", chain.CAIP2, strings.ToLower(token.Address)),
			Address:  strings.ToLower(token.Address),
			Symbol:   token.Symbol,
			Decimals: token.Decimals,
		}, nil
	}
	return Asset{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("token symbol not found: %s", input))
}

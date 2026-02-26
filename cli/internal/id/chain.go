package id

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	clierr "github.com/mantle/mantle-ai/cli/internal/errors"
)

type Chain struct {
	Name       string
	Slug       string
	CAIP2      string
	EVMChainID int64
}

var eip155Pattern = regexp.MustCompile(`^eip155:[0-9]+$`)

var chainAliases = map[string]Chain{
	"mainnet":        {Name: "Mantle Mainnet", Slug: "mantle", CAIP2: "eip155:5000", EVMChainID: 5000},
	"mantle":         {Name: "Mantle Mainnet", Slug: "mantle", CAIP2: "eip155:5000", EVMChainID: 5000},
	"5000":           {Name: "Mantle Mainnet", Slug: "mantle", CAIP2: "eip155:5000", EVMChainID: 5000},
	"eip155:5000":    {Name: "Mantle Mainnet", Slug: "mantle", CAIP2: "eip155:5000", EVMChainID: 5000},
	"sepolia":        {Name: "Mantle Sepolia", Slug: "mantle-sepolia", CAIP2: "eip155:5003", EVMChainID: 5003},
	"mantle-sepolia": {Name: "Mantle Sepolia", Slug: "mantle-sepolia", CAIP2: "eip155:5003", EVMChainID: 5003},
	"5003":           {Name: "Mantle Sepolia", Slug: "mantle-sepolia", CAIP2: "eip155:5003", EVMChainID: 5003},
	"eip155:5003":    {Name: "Mantle Sepolia", Slug: "mantle-sepolia", CAIP2: "eip155:5003", EVMChainID: 5003},
}

func ParseChain(input string) (Chain, error) {
	norm := strings.ToLower(strings.TrimSpace(input))
	if norm == "" {
		return Chain{}, clierr.New(clierr.CodeUsage, "chain is required")
	}
	if chain, ok := chainAliases[norm]; ok {
		return chain, nil
	}
	if eip155Pattern.MatchString(norm) {
		suffix := strings.TrimPrefix(norm, "eip155:")
		id, err := strconv.ParseInt(suffix, 10, 64)
		if err != nil {
			return Chain{}, clierr.Wrap(clierr.CodeUsage, "invalid chain", err)
		}
		return Chain{Name: fmt.Sprintf("EVM %d", id), Slug: suffix, CAIP2: norm, EVMChainID: id}, nil
	}
	if id, err := strconv.ParseInt(norm, 10, 64); err == nil {
		caip2 := fmt.Sprintf("eip155:%d", id)
		if chain, ok := chainAliases[caip2]; ok {
			return chain, nil
		}
		return Chain{Name: fmt.Sprintf("EVM %d", id), Slug: norm, CAIP2: caip2, EVMChainID: id}, nil
	}
	return Chain{}, clierr.New(clierr.CodeUnsupported, fmt.Sprintf("unsupported chain: %s", input))
}

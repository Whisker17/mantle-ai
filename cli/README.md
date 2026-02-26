# mantle

Agent-first Mantle retrieval CLI.

## Features

- Chain metadata and status
- Address balances (MNT + known tokens)
- Transaction lookup
- Contract read and call simulation
- Token info, symbol resolution, token balances
- Swap quotes (Agni, Merchant Moe, best-of routing)
- Lending markets and rates (Lendle, Aurelius)
- mETH staking info and quote
- Yield opportunities (Pendle + lending aggregation)
- Bridge quotes and status (official bridge + Across quote support)
- Machine-readable command schema and providers metadata

## Build and test

```bash
go build -o mantle ./cmd/mantle
go test ./...
go test -race ./...
go vet ./...
```

## Quick start

```bash
./mantle providers list --results-only
./mantle schema --results-only
./mantle chain info --results-only
./mantle chain status --results-only
./mantle swap quote --from USDC --to WMNT --amount 100 --provider best --results-only
./mantle lend markets --protocol all --results-only
./mantle stake info --results-only
./mantle yield opportunities --limit 10 --sort-by score --results-only
./mantle bridge quote --from-chain eip155:1 --to-chain eip155:5000 --asset USDC --amount 100 --provider best --results-only
```

## Configuration

Default config path: `${XDG_CONFIG_HOME:-~/.config}/mantle/config.yaml`

```yaml
network: mainnet
rpc_url: https://rpc.mantle.xyz
rpc_url_sepolia: https://rpc.sepolia.mantle.xyz

cache:
  enabled: true
  path: ~/.cache/mantle/cache.db
  max_stale: 5m

providers:
  across:
    enabled: true
    api_key_env: ACROSS_API_KEY
```

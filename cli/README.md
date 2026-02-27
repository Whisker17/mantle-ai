# mantle

Agent-first Mantle retrieval CLI.

## Features

- [x] Chain metadata and status
- [x] Address balances (MNT + known tokens)
- [x] Transaction lookup
- [x] Contract read and call simulation
- [x] Token info, symbol resolution, token balances
- [x] Swap quotes (Agni, Merchant Moe, best-of routing)
- [x] Lending markets and rates (Lendle, Aurelius, AAVE v3)
- [x] mETH staking info and quote
- [x] Yield opportunities (Pendle + lending aggregation)
- [x] Bridge quotes and status (official bridge + Across quote support)
- [x] Machine-readable command schema and providers metadata

## Regression validation rules

- Mark a feature as done only after end-to-end CLI regression verification passes.
- Run transfer/swap behavior regression on Mantle Sepolia (`network: sepolia`, `rpc_url_sepolia: https://rpc.sepolia.mantle.xyz`).

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

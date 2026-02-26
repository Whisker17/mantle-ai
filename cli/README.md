# mantle

Agent-first Mantle retrieval CLI.

## Features (Phase 1)

- Chain metadata and status
- Address balances (MNT + known tokens)
- Transaction lookup
- Contract read and call simulation
- Token info, symbol resolution, token balances
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

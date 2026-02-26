# AGENTS.md

Short guide for agents working on `mantle`.

## Project intent

`mantle` is an agent-first Mantle retrieval CLI. Core priorities:

- stable JSON envelope output
- deterministic exit codes
- deterministic command schema for agent discovery

## Core checks

```bash
go test ./...
go test -race ./...
go vet ./...
go build -o mantle ./cmd/mantle
```

## Commands (Phase 1)

- `chain info`, `chain status`
- `balance <address>`
- `tx <hash>`
- `contract read|call`
- `token info|resolve|balances`
- `providers list`, `schema`, `version`

# Mantle-AI Full Scaffold Design

> Design and implementation spec for a complete agent-friendly scaffold for Mantle Network.
> This document is the single source of truth for implementers. It covers architecture, interfaces, command schemas, tool schemas, and code skeletons.

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [CLI (Go)](#2-cli-go)
3. [MCP Server (TypeScript)](#3-mcp-server-typescript)
4. [Plugins and Skills](#4-plugins-and-skills)
5. [Shared Constants and Contracts](#5-shared-constants-and-contracts)
6. [Repository Structure](#6-repository-structure)
7. [Implementation Phases](#7-implementation-phases)

---

## 1. Architecture Overview

mantle-ai is a monorepo containing four independent but complementary pillars:

```
mantle-ai/
├── cli/                     # Go CLI binary (like defi-cli)
├── mcp-server/              # Standalone TypeScript MCP server
├── packages/plugins/        # Claude Code plugins (skills + agents)
└── docs/                    # Documentation and design specs
```

### How the Four Pillars Relate

```
                    ┌─────────────┐
                    │  AI Agent   │
                    └──────┬──────┘
                           │
            ┌──────────────┼──────────────┐
            │              │              │
            ▼              ▼              ▼
     ┌─────────────┐ ┌──────────┐ ┌──────────────┐
     │  CLI (Go)   │ │   MCP    │ │   Plugins    │
     │ shell tool  │ │  Server  │ │ skills/agents│
     └──────┬──────┘ └────┬─────┘ └──────────────┘
            │              │
            │     ┌────────┴────────┐
            │     │                 │
            ▼     ▼                 ▼
     ┌─────────────────┐    ┌──────────────┐
     │  Mantle RPC      │    │  DeFi APIs   │
     │  (viem / ethclient)│   │  (Agni, etc) │
     └─────────────────┘    └──────────────┘
```

- **CLI**: Agents invoke via `Bash(mantle:*)`. Returns structured JSON envelope. Agent-friendly via `--json`, `--results-only`, `--select`, and `schema` command.
- **MCP Server**: Agents invoke via MCP protocol (stdio). Returns structured JSON. Agent-friendly natively.
- **Plugins/Skills**: Agents read as markdown context. Provides knowledge about Mantle protocols, patterns, and contracts. No runtime component.

### Design Principles

1. **MNT-first**: Every component must account for MNT as native gas token (not ETH).
2. **Agent-agnostic**: CLI outputs structured JSON; MCP uses standard protocol; skills are plain markdown.
3. **Deterministic output**: CLI uses the defi-cli envelope contract. MCP tools return structured JSON. No ambiguous text.
4. **Fail-safe**: Typed errors with deterministic exit codes. Stale cache fallback. Partial results flagged.

---

## 2. CLI (Go)

A standalone Go binary following the defi-cli architecture. Single binary distribution via GoReleaser.

### 2.1 Directory Structure

```
cli/
├── cmd/mantle/
│   └── main.go                    # Entrypoint
├── internal/
│   ├── app/
│   │   └── runner.go              # Cobra root command, wiring, envelope emission
│   ├── config/
│   │   └── config.go              # YAML/env/flags config loading
│   ├── cache/
│   │   └── cache.go               # SQLite WAL cache
│   ├── providers/
│   │   ├── types.go               # Provider interfaces
│   │   ├── agni/                   # Agni Finance V3 (swap)
│   │   │   └── agni.go
│   │   ├── merchantmoe/           # Merchant Moe (swap)
│   │   │   └── merchantmoe.go
│   │   ├── lendle/                 # Lendle (lending)
│   │   │   └── lendle.go
│   │   ├── aurelius/               # Aurelius Finance (lending)
│   │   │   └── aurelius.go
│   │   ├── meth/                   # mETH LSP (staking)
│   │   │   └── meth.go
│   │   ├── mantlebridge/           # Official Mantle Bridge
│   │   │   └── bridge.go
│   │   ├── across/                 # Across Protocol (third-party bridge)
│   │   │   └── across.go
│   │   ├── pendle/                 # Pendle (yield)
│   │   │   └── pendle.go
│   │   ├── rpc/                    # Direct RPC provider (chain ops)
│   │   │   └── rpc.go
│   │   └── defillama/              # DefiLlama (market data)
│   │       └── defillama.go
│   ├── id/
│   │   ├── chain.go               # Chain ID parsing (CAIP-2)
│   │   ├── asset.go               # Asset ID parsing (CAIP-19)
│   │   └── tokens.go              # Mantle token registry
│   ├── model/
│   │   └── types.go               # Envelope + all domain types
│   ├── out/
│   │   └── render.go              # JSON/plain output, --select, --results-only
│   ├── errors/
│   │   └── errors.go              # Typed errors -> exit codes
│   ├── schema/
│   │   └── schema.go              # Machine-readable command schema
│   ├── policy/
│   │   └── policy.go              # Command allowlist
│   └── httpx/
│       └── client.go              # Shared HTTP client with retries
├── go.mod
├── go.sum
├── Makefile
├── .goreleaser.yml
├── AGENTS.md
└── README.md
```

### 2.2 Command Tree

```
mantle
├── chain
│   ├── info                        # Chain config (ID, RPC, gas token, explorer)
│   └── status                      # Current block number, gas price, sync status
├── balance <address>               # MNT + top token balances for an address
├── tx <hash>                       # Transaction details + receipt
├── contract
│   ├── read <addr> <func> [args]   # Call view/pure function
│   └── call <addr> <func> [args]   # Simulate write function (estimateGas)
├── token
│   ├── info <address>              # Name, symbol, decimals, totalSupply
│   ├── resolve <symbol>            # Symbol -> address resolution
│   └── balances <address>          # All known token balances
├── swap
│   └── quote                       # DEX swap quote (Agni, Merchant Moe)
├── lend
│   ├── markets                     # Lending markets (Lendle, Aurelius)
│   └── rates                       # Supply/borrow APY rates
├── stake
│   ├── info                        # mETH exchange rate, TVL, APY
│   └── quote                       # Stake/unstake quote (ETH -> mETH)
├── yield
│   └── opportunities               # Ranked yield opps (Pendle, LP, lending)
├── bridge
│   ├── quote                       # Bridge quote (official + third-party)
│   └── status <tx_hash>            # Bridge transaction status
├── providers
│   └── list                        # Available providers and auth status
├── schema [command_path]           # Machine-readable command schema
└── version                         # CLI version
```

### 2.3 Global Flags

```
--json              Force JSON envelope output (default for piped stdout)
--plain             Force plain-text table output
--select <fields>   Project output to selected fields (dot-path, comma-separated)
--results-only      Emit only the data payload, omit envelope wrapper
--strict            Fail on partial results instead of returning with warnings
--timeout <dur>     Per-provider timeout (default: 10s)
--retries <n>       Retry count on transient failure (default: 2)
--no-cache          Skip cache entirely
--no-stale          Reject stale cache hits
--max-stale <dur>   Maximum acceptable stale age (default: per-command)
--config <path>     Config file path (default: ~/.config/mantle/config.yaml)
--network <net>     Target network: mainnet (default) or sepolia
--rpc-url <url>     Override RPC endpoint
--enable-commands   Command allowlist (comma-separated, for sandboxed agents)
```

### 2.4 Provider Interfaces

```go
package providers

import (
    "context"
    "github.com/mantle/mantle-ai/cli/internal/id"
    "github.com/mantle/mantle-ai/cli/internal/model"
)

// Every provider implements this base.
type Provider interface {
    Info() model.ProviderInfo
}

// Direct RPC operations (balance, tx, block, contract).
type ChainProvider interface {
    Provider
    ChainInfo(ctx context.Context) (model.ChainInfo, error)
    ChainStatus(ctx context.Context) (model.ChainStatus, error)
    GetBalance(ctx context.Context, address string) (model.AddressBalance, error)
    GetTransaction(ctx context.Context, hash string) (model.TransactionInfo, error)
    ReadContract(ctx context.Context, req ContractReadRequest) (model.ContractResult, error)
    SimulateCall(ctx context.Context, req ContractCallRequest) (model.CallSimulation, error)
    GetTokenInfo(ctx context.Context, address string) (model.TokenInfo, error)
    ResolveToken(ctx context.Context, symbol string) (model.TokenResolution, error)
    GetTokenBalances(ctx context.Context, address string) ([]model.TokenBalance, error)
}

type ContractReadRequest struct {
    Address  string
    Function string   // Solidity signature: "balanceOf(address)"
    Args     []string
}

type ContractCallRequest struct {
    From     string
    To       string
    Function string
    Args     []string
    Value    string // MNT in ether units
}

// DEX swap quotes.
type SwapProvider interface {
    Provider
    QuoteSwap(ctx context.Context, req SwapQuoteRequest) (model.SwapQuote, error)
}

type SwapQuoteRequest struct {
    FromAsset       id.Asset
    ToAsset         id.Asset
    AmountBaseUnits string
    AmountDecimal   string
    FeeTier         int // 500, 3000, 10000 (for V3 pools)
}

// Lending protocol data.
type LendingProvider interface {
    Provider
    LendMarkets(ctx context.Context, asset id.Asset) ([]model.LendMarket, error)
    LendRates(ctx context.Context, asset id.Asset) ([]model.LendRate, error)
}

// mETH staking operations.
type StakingProvider interface {
    Provider
    StakeInfo(ctx context.Context) (model.StakeInfo, error)
    StakeQuote(ctx context.Context, req StakeQuoteRequest) (model.StakeQuote, error)
}

type StakeQuoteRequest struct {
    Action          string // "stake" or "unstake"
    AmountDecimal   string
}

// Yield aggregation.
type YieldProvider interface {
    Provider
    YieldOpportunities(ctx context.Context, req YieldRequest) ([]model.YieldOpportunity, error)
}

type YieldRequest struct {
    Asset             id.Asset
    Limit             int
    MinTVLUSD         float64
    MinAPY            float64
    SortBy            string // "apy", "tvl", "score"
}

// Cross-chain bridge.
type BridgeProvider interface {
    Provider
    QuoteBridge(ctx context.Context, req BridgeQuoteRequest) (model.BridgeQuote, error)
}

type BridgeQuoteRequest struct {
    FromChain       id.Chain // eip155:1 (L1) or eip155:5000 (L2)
    ToChain         id.Chain
    Asset           id.Asset
    AmountDecimal   string
}

// Bridge status tracking.
type BridgeStatusProvider interface {
    Provider
    BridgeStatus(ctx context.Context, txHash string) (model.BridgeStatus, error)
}
```

### 2.5 Domain Models

```go
package model

import "time"

const EnvelopeVersion = "v1"

// --- Output Envelope (same contract as defi-cli) ---

type Envelope struct {
    Version  string       `json:"version"`
    Success  bool         `json:"success"`
    Data     any          `json:"data,omitempty"`
    Error    *ErrorBody   `json:"error"`
    Warnings []string     `json:"warnings,omitempty"`
    Meta     EnvelopeMeta `json:"meta"`
}

type ErrorBody struct {
    Code    int    `json:"code"`
    Type    string `json:"type"`
    Message string `json:"message"`
}

type EnvelopeMeta struct {
    RequestID string           `json:"request_id"`
    Timestamp time.Time        `json:"timestamp"`
    Command   string           `json:"command"`
    Providers []ProviderStatus `json:"providers,omitempty"`
    Cache     CacheStatus      `json:"cache"`
    Partial   bool             `json:"partial"`
}

// --- Chain Operations ---

type ChainInfo struct {
    ChainID     int    `json:"chain_id"`
    Name        string `json:"name"`
    NativeToken string `json:"native_token"` // "MNT"
    RPCURL      string `json:"rpc_url"`
    ExplorerURL string `json:"explorer_url"`
    BridgeURL   string `json:"bridge_url"`
    DALayer     string `json:"da_layer"` // "EigenDA"
}

type ChainStatus struct {
    ChainID       int    `json:"chain_id"`
    BlockNumber   uint64 `json:"block_number"`
    GasPrice      string `json:"gas_price"`      // wei
    GasPriceMNT   string `json:"gas_price_mnt"`  // formatted
    L1DataFee     string `json:"l1_data_fee"`     // estimated L1 data posting cost
    SyncStatus    string `json:"sync_status"`
    PeerCount     int    `json:"peer_count"`
}

type AddressBalance struct {
    Address    string         `json:"address"`
    MNTBalance AmountInfo     `json:"mnt_balance"`
    Tokens     []TokenBalance `json:"tokens,omitempty"`
}

type TokenBalance struct {
    Symbol   string     `json:"symbol"`
    Address  string     `json:"address"`
    Balance  AmountInfo `json:"balance"`
}

type AmountInfo struct {
    AmountBaseUnits string `json:"amount_base_units"`
    AmountDecimal   string `json:"amount_decimal"`
    Decimals        int    `json:"decimals"`
}

type TransactionInfo struct {
    Hash            string     `json:"hash"`
    From            string     `json:"from"`
    To              string     `json:"to"`
    Value           AmountInfo `json:"value"` // MNT
    GasUsed         string     `json:"gas_used"`
    GasPrice        string     `json:"gas_price"`
    FeeMNT          string     `json:"fee_mnt"`
    BlockNumber     uint64     `json:"block_number"`
    Timestamp       time.Time  `json:"timestamp"`
    Status          string     `json:"status"` // "success" | "reverted"
    Input           string     `json:"input,omitempty"`
    ExplorerURL     string     `json:"explorer_url"`
}

type ContractResult struct {
    Address  string `json:"address"`
    Function string `json:"function"`
    Result   any    `json:"result"`
}

type CallSimulation struct {
    Success     bool   `json:"success"`
    GasEstimate string `json:"gas_estimate"`
    FeeMNT      string `json:"fee_mnt"`
    ReturnData  any    `json:"return_data,omitempty"`
    Error       string `json:"error,omitempty"`
}

type TokenInfo struct {
    Address     string `json:"address"`
    Name        string `json:"name"`
    Symbol      string `json:"symbol"`
    Decimals    int    `json:"decimals"`
    TotalSupply string `json:"total_supply"`
    ExplorerURL string `json:"explorer_url"`
}

type TokenResolution struct {
    Input       string `json:"input"`
    Symbol      string `json:"symbol"`
    Address     string `json:"address"`
    Decimals    int    `json:"decimals"`
    Unambiguous bool   `json:"unambiguous"`
}

// --- DeFi: Swap ---

type SwapQuote struct {
    Provider        string     `json:"provider"` // "agni", "merchant_moe"
    FromAsset       string     `json:"from_asset"`
    ToAsset         string     `json:"to_asset"`
    InputAmount     AmountInfo `json:"input_amount"`
    EstimatedOut    AmountInfo `json:"estimated_out"`
    PriceImpactPct  float64    `json:"price_impact_pct"`
    EstimatedGasMNT string     `json:"estimated_gas_mnt"`
    Route           string     `json:"route"`
    RouterAddress   string     `json:"router_address"`
    FetchedAt       string     `json:"fetched_at"`
}

// --- DeFi: Lending ---

type LendMarket struct {
    Protocol  string  `json:"protocol"` // "lendle", "aurelius"
    Asset     string  `json:"asset"`
    SupplyAPY float64 `json:"supply_apy"`
    BorrowAPY float64 `json:"borrow_apy"`
    TVLUSD    float64 `json:"tvl_usd"`
    FetchedAt string  `json:"fetched_at"`
}

type LendRate struct {
    Protocol    string  `json:"protocol"`
    Asset       string  `json:"asset"`
    SupplyAPY   float64 `json:"supply_apy"`
    BorrowAPY   float64 `json:"borrow_apy"`
    Utilization float64 `json:"utilization"`
    FetchedAt   string  `json:"fetched_at"`
}

// --- DeFi: Staking ---

type StakeInfo struct {
    Protocol      string  `json:"protocol"` // "meth_lsp"
    METHToETH     string  `json:"meth_to_eth"`     // exchange rate
    ETHToMETH     string  `json:"eth_to_meth"`
    APY           float64 `json:"apy"`
    TotalStaked   string  `json:"total_staked"`     // ETH
    TotalMETH     string  `json:"total_meth"`
    UnstakeDelay  string  `json:"unstake_delay"`    // e.g., "~7 days"
    FetchedAt     string  `json:"fetched_at"`
}

type StakeQuote struct {
    Action          string     `json:"action"` // "stake" or "unstake"
    InputAmount     AmountInfo `json:"input_amount"`
    EstimatedOut    AmountInfo `json:"estimated_out"`
    ExchangeRate    string     `json:"exchange_rate"`
    MinOutput       string     `json:"min_output"` // with 1% slippage
    FetchedAt       string     `json:"fetched_at"`
}

// --- DeFi: Yield ---

type YieldOpportunity struct {
    Protocol  string  `json:"protocol"` // "pendle", "lendle", "agni_lp"
    Asset     string  `json:"asset"`
    Type      string  `json:"type"`     // "pt", "yt", "supply", "lp"
    APYBase   float64 `json:"apy_base"`
    APYReward float64 `json:"apy_reward"`
    APYTotal  float64 `json:"apy_total"`
    TVLUSD    float64 `json:"tvl_usd"`
    RiskLevel string  `json:"risk_level"` // "low", "medium", "high"
    Score     float64 `json:"score"`
    FetchedAt string  `json:"fetched_at"`
}

// --- Bridge ---

type BridgeQuote struct {
    Provider        string     `json:"provider"` // "mantle_bridge", "across", "stargate"
    FromChain       string     `json:"from_chain"`
    ToChain         string     `json:"to_chain"`
    Asset           string     `json:"asset"`
    InputAmount     AmountInfo `json:"input_amount"`
    EstimatedOut    AmountInfo `json:"estimated_out"`
    EstimatedFeeUSD float64   `json:"estimated_fee_usd"`
    EstimatedTimeS  int64     `json:"estimated_time_s"`
    Route           string     `json:"route"`
    FetchedAt       string     `json:"fetched_at"`
}

type BridgeStatus struct {
    TxHash          string `json:"tx_hash"`
    Direction       string `json:"direction"` // "deposit" or "withdrawal"
    Status          string `json:"status"`    // "pending", "proven", "finalized", "completed"
    L1TxHash        string `json:"l1_tx_hash,omitempty"`
    L2TxHash        string `json:"l2_tx_hash,omitempty"`
    ChallengeEnd    string `json:"challenge_end,omitempty"` // ISO 8601
    RemainingTime   string `json:"remaining_time,omitempty"`
    ExplorerURL     string `json:"explorer_url"`
}
```

### 2.6 Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Internal error |
| 2 | Usage / validation error |
| 10 | Auth required / failed |
| 11 | Rate limited |
| 12 | Provider unavailable |
| 13 | Unsupported input / provider |
| 14 | Stale data beyond SLA |
| 15 | Partial results in strict mode |
| 16 | Blocked by command allowlist |

### 2.7 Schema System

Same as defi-cli. `mantle schema` emits machine-readable JSON describing all commands, flags, and their types. Agents use this to auto-discover available operations.

```go
type CommandSchema struct {
    Path        string          `json:"path"`
    Use         string          `json:"use"`
    Short       string          `json:"short"`
    Aliases     []string        `json:"aliases,omitempty"`
    Flags       []FlagSchema    `json:"flags,omitempty"`
    Subcommands []CommandSchema `json:"subcommands,omitempty"`
}

type FlagSchema struct {
    Name      string `json:"name"`
    Shorthand string `json:"shorthand,omitempty"`
    Type      string `json:"type"`
    Usage     string `json:"usage"`
    Default   string `json:"default,omitempty"`
}
```

Example: `mantle schema "swap quote" --results-only` returns the flag spec for the swap quote command.

### 2.8 Config File

```yaml
# ~/.config/mantle/config.yaml
network: mainnet
rpc_url: https://rpc.mantle.xyz
rpc_url_sepolia: https://rpc.sepolia.mantle.xyz

cache:
  enabled: true
  path: ~/.cache/mantle/cache.db
  max_stale: 5m

providers:
  agni:
    enabled: true
  merchant_moe:
    enabled: true
  lendle:
    enabled: true
  aurelius:
    enabled: true
  meth:
    enabled: true
  mantle_bridge:
    enabled: true
  across:
    enabled: true
    api_key_env: ACROSS_API_KEY
  pendle:
    enabled: true
  defillama:
    enabled: true
```

### 2.9 Cache Strategy

Same as defi-cli: SQLite WAL with file lock and busy-timeout.

| Command | TTL | Max Stale |
|---------|-----|-----------|
| chain info | 1h | 24h |
| chain status | 15s | 1m |
| balance | 30s | 5m |
| tx | Infinite (immutable) | N/A |
| swap quote | 15s | 1m |
| lend markets | 5m | 30m |
| lend rates | 30s | 5m |
| stake info | 1m | 10m |
| yield | 5m | 30m |
| bridge quote | 30s | 5m |
| bridge status | 30s | 5m |

### 2.10 Distribution

GoReleaser config for cross-platform single-binary distribution:
- Linux amd64, arm64
- macOS amd64, arm64 (universal)
- Windows amd64

Install script: `curl -fsSL https://raw.githubusercontent.com/mantle/mantle-ai/main/cli/scripts/install.sh | sh`

---

## 3. MCP Server (TypeScript)

A standalone TypeScript MCP server using `@modelcontextprotocol/sdk` and `viem`. Lives as its own package in the monorepo, independent of the plugin system.

### 3.1 Directory Structure

```
mcp-server/
├── src/
│   ├── server.ts                  # MCP server entrypoint (stdio transport)
│   ├── tools/
│   │   ├── index.ts               # Tool registration
│   │   ├── chain.ts               # Chain info and status tools
│   │   ├── balance.ts             # Balance query tools
│   │   ├── transaction.ts         # Transaction lookup tools
│   │   ├── contract.ts            # Contract read/simulate tools
│   │   ├── token.ts               # Token info and resolution tools
│   │   ├── swap.ts                # DEX swap quote tools
│   │   ├── lending.ts             # Lending market/rate tools
│   │   ├── staking.ts             # mETH staking tools
│   │   ├── yield.ts               # Yield opportunity tools
│   │   ├── bridge.ts              # Bridge quote and status tools
│   │   └── explorer.ts            # Explorer URL generation
│   ├── resources/
│   │   ├── index.ts               # Resource registration
│   │   ├── chain-config.ts        # mantle://chain/* resources
│   │   ├── token-registry.ts      # mantle://tokens/* resources
│   │   └── protocol-registry.ts   # mantle://protocols/* resources
│   ├── providers/
│   │   ├── rpc.ts                 # Direct viem RPC calls
│   │   ├── agni.ts                # Agni Finance quoter
│   │   ├── lendle.ts              # Lendle lending pool reads
│   │   ├── meth.ts                # mETH staking manager reads
│   │   └── bridge.ts              # Mantle Bridge status
│   ├── config/
│   │   ├── chains.ts              # Chain definitions (mainnet, sepolia)
│   │   ├── tokens.ts              # Token registry
│   │   └── protocols.ts           # Protocol contract addresses
│   └── utils/
│       ├── client.ts              # viem client factory (lazy singleton)
│       ├── format.ts              # BigInt serialization, amount formatting
│       └── errors.ts              # Typed error responses
├── package.json
├── tsconfig.json
├── AGENTS.md
└── README.md
```

### 3.2 Tool Catalog

#### 3.2.1 Chain Tools

**`mantle_chainInfo`**
```typescript
{
  name: "mantle_chainInfo",
  description: "Get Mantle Network chain configuration and status",
  inputSchema: {
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  },
  // Returns: ChainInfo (chainId, name, nativeToken, rpcUrl, explorerUrl, daLayer)
}
```

**`mantle_chainStatus`**
```typescript
{
  name: "mantle_chainStatus",
  description: "Get current block number, gas price in MNT, sync status",
  inputSchema: {
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  },
  // Returns: { blockNumber, gasPrice, gasPriceGwei, gasPriceMNT }
}
```

#### 3.2.2 Balance Tools

**`mantle_getBalance`**
```typescript
{
  name: "mantle_getBalance",
  description: "Get native MNT balance and optionally ERC-20 token balance",
  inputSchema: {
    address: z.string().describe("Wallet address (0x...)"),
    tokenAddress: z.string().optional().describe("ERC-20 address; omit for native MNT"),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  },
  // Returns: { address, token, symbol, decimals, balance, formatted }
}
```

**`mantle_getTokenBalances`**
```typescript
{
  name: "mantle_getTokenBalances",
  description: "Get all known token balances for an address on Mantle",
  inputSchema: {
    address: z.string(),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  },
  // Returns: { address, mntBalance, tokens: [{symbol, address, balance, formatted}] }
}
```

#### 3.2.3 Transaction Tools

**`mantle_getTransaction`**
```typescript
{
  name: "mantle_getTransaction",
  description: "Get transaction details and receipt",
  inputSchema: {
    hash: z.string().describe("Transaction hash (0x...)"),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  },
  // Returns: { hash, from, to, value, valueMNT, gasUsed, feeMNT, blockNumber, status }
}
```

**`mantle_getBlock`**
```typescript
{
  name: "mantle_getBlock",
  description: "Get block data by number or latest",
  inputSchema: {
    blockNumber: z.string().optional(),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  }
}
```

#### 3.2.4 Contract Tools

**`mantle_readContract`**
```typescript
{
  name: "mantle_readContract",
  description: "Call a view/pure function on a Mantle contract",
  inputSchema: {
    address: z.string(),
    functionSignature: z.string().describe('e.g. "function balanceOf(address) view returns (uint256)"'),
    args: z.array(z.string()).default([]),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  }
}
```

**`mantle_simulateCall`**
```typescript
{
  name: "mantle_simulateCall",
  description: "Simulate a contract write call (dry-run, does not send)",
  inputSchema: {
    address: z.string(),
    functionSignature: z.string(),
    args: z.array(z.string()).default([]),
    value: z.string().optional().describe("MNT value in ether units"),
    from: z.string().describe("Sender address for simulation"),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  }
}
```

**`mantle_encodeFunction`**
```typescript
{
  name: "mantle_encodeFunction",
  description: "ABI-encode a function call for transaction data",
  inputSchema: {
    functionSignature: z.string(),
    args: z.array(z.string())
  }
}
```

**`mantle_estimateGas`**
```typescript
{
  name: "mantle_estimateGas",
  description: "Estimate gas cost for a transaction in MNT",
  inputSchema: {
    from: z.string(),
    to: z.string(),
    value: z.string().optional(),
    data: z.string().optional(),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  }
  // Returns: { gasEstimate, gasPrice, gasPriceGwei, totalFee, totalFeeMNT }
}
```

#### 3.2.5 Token Tools

**`mantle_getTokenInfo`**
```typescript
{
  name: "mantle_getTokenInfo",
  description: "Get ERC-20 token metadata (name, symbol, decimals, totalSupply)",
  inputSchema: {
    tokenAddress: z.string(),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  }
}
```

**`mantle_resolveToken`**
```typescript
{
  name: "mantle_resolveToken",
  description: "Resolve token symbol to address on Mantle (e.g. USDC -> 0x09Bc...)",
  inputSchema: {
    symbol: z.string().describe("Token symbol (USDC, WMNT, mETH, etc.)")
  }
  // Returns: { symbol, address, decimals }
}
```

#### 3.2.6 DeFi: Swap Tools

**`mantle_getSwapQuote`**
```typescript
{
  name: "mantle_getSwapQuote",
  description: "Get DEX swap quote on Mantle (Agni Finance V3, Merchant Moe)",
  inputSchema: {
    tokenIn: z.string().describe("Input token address or symbol"),
    tokenOut: z.string().describe("Output token address or symbol"),
    amountIn: z.string().describe("Amount in human-readable units"),
    feeTier: z.number().default(3000).describe("V3 fee tier: 500, 3000, 10000"),
    provider: z.enum(["agni", "merchant_moe", "best"]).default("best")
  }
  // Returns: { provider, tokenIn, tokenOut, amountIn, amountOut, priceImpact, router }
}
```

#### 3.2.7 DeFi: Lending Tools

**`mantle_getLendingMarkets`**
```typescript
{
  name: "mantle_getLendingMarkets",
  description: "Get lending markets on Mantle (Lendle, Aurelius)",
  inputSchema: {
    asset: z.string().optional().describe("Filter by asset symbol or address"),
    protocol: z.enum(["lendle", "aurelius", "all"]).default("all")
  }
  // Returns: [{ protocol, asset, supplyAPY, borrowAPY, tvlUSD }]
}
```

**`mantle_getLendingRates`**
```typescript
{
  name: "mantle_getLendingRates",
  description: "Get supply and borrow APY rates for a specific asset",
  inputSchema: {
    asset: z.string().describe("Asset symbol or address"),
    protocol: z.enum(["lendle", "aurelius", "all"]).default("all")
  }
}
```

#### 3.2.8 DeFi: Staking Tools

**`mantle_getStakingInfo`**
```typescript
{
  name: "mantle_getStakingInfo",
  description: "Get mETH liquid staking info: exchange rate, APY, total staked",
  inputSchema: {}
  // Returns: { methToEth, ethToMeth, apy, totalStaked, unstakeDelay }
}
```

**`mantle_getStakeQuote`**
```typescript
{
  name: "mantle_getStakeQuote",
  description: "Get quote for staking ETH -> mETH or unstaking mETH -> ETH",
  inputSchema: {
    action: z.enum(["stake", "unstake"]),
    amount: z.string().describe("Amount in ether units")
  }
  // Returns: { action, inputAmount, estimatedOutput, exchangeRate, minOutput }
}
```

#### 3.2.9 DeFi: Yield Tools

**`mantle_getYieldOpportunities`**
```typescript
{
  name: "mantle_getYieldOpportunities",
  description: "Get ranked yield opportunities on Mantle (Pendle, LP, lending)",
  inputSchema: {
    asset: z.string().optional(),
    minAPY: z.number().optional(),
    limit: z.number().default(20),
    sortBy: z.enum(["apy", "tvl", "score"]).default("score")
  }
  // Returns: [{ protocol, asset, type, apyBase, apyReward, apyTotal, tvlUSD, riskLevel, score }]
}
```

#### 3.2.10 Bridge Tools

**`mantle_getBridgeQuote`**
```typescript
{
  name: "mantle_getBridgeQuote",
  description: "Get bridge quote for L1<->L2 transfer (official + third-party)",
  inputSchema: {
    token: z.string().describe("Token to bridge (ETH, MNT, USDC, etc.)"),
    amount: z.string().describe("Amount in human-readable units"),
    direction: z.enum(["deposit", "withdrawal"]).describe("deposit = L1->L2, withdrawal = L2->L1"),
    provider: z.enum(["official", "across", "stargate", "best"]).default("best")
  }
  // Returns: { provider, token, amount, estimatedOutput, estimatedFeeUSD, estimatedTimeS }
}
```

**`mantle_getBridgeStatus`**
```typescript
{
  name: "mantle_getBridgeStatus",
  description: "Check status of a bridge transaction (deposit or withdrawal)",
  inputSchema: {
    txHash: z.string(),
    direction: z.enum(["deposit", "withdrawal"])
  }
  // Returns: { status, l1TxHash, l2TxHash, challengeEnd, remainingTime }
}
```

#### 3.2.11 Explorer Tools

**`mantle_explorerLookup`**
```typescript
{
  name: "mantle_explorerLookup",
  description: "Generate Mantle Explorer URL for an address, tx, or block",
  inputSchema: {
    query: z.string().describe("Address, tx hash, or block number"),
    network: z.enum(["mainnet", "sepolia"]).default("mainnet")
  }
  // Returns: { url, apiUrl }
}
```

### 3.3 MCP Resources

Resources provide static/semi-static data that agents can read as context.

```typescript
// mantle://chain/mainnet
{
  uri: "mantle://chain/mainnet",
  name: "Mantle Mainnet Configuration",
  mimeType: "application/json",
  // Returns: { chainId: 5000, nativeToken: "MNT", rpcUrl, explorerUrl, ... }
}

// mantle://chain/sepolia
{
  uri: "mantle://chain/sepolia",
  name: "Mantle Sepolia Testnet Configuration",
  mimeType: "application/json",
}

// mantle://tokens/registry
{
  uri: "mantle://tokens/registry",
  name: "Mantle Token Registry",
  mimeType: "application/json",
  // Returns: { WMNT: { address, decimals }, USDC: { ... }, ... }
}

// mantle://protocols/list
{
  uri: "mantle://protocols/list",
  name: "Mantle DeFi Protocol Registry",
  mimeType: "application/json",
  // Returns: [{ name, type, contracts: { router, factory, ... } }, ...]
}

// mantle://contracts/bridge
{
  uri: "mantle://contracts/bridge",
  name: "Mantle Bridge Contract Addresses",
  mimeType: "application/json",
  // Returns: { l1: { standardBridge, crossDomainMessenger }, l2: { ... } }
}
```

### 3.4 MCP Configuration

```json
{
  "mcpServers": {
    "mantle-onchain": {
      "type": "stdio",
      "command": "npx",
      "args": ["tsx", "mcp-server/src/server.ts"],
      "env": {
        "MANTLE_RPC_URL": "https://rpc.mantle.xyz",
        "MANTLE_SEPOLIA_RPC_URL": "https://rpc.sepolia.mantle.xyz",
        "MANTLE_NETWORK": "mainnet"
      }
    }
  }
}
```

### 3.5 Error Handling

Every tool returns a consistent shape. On error:

```typescript
{
  content: [{
    type: "text",
    text: JSON.stringify({
      error: true,
      code: "PROVIDER_UNAVAILABLE",
      message: "Agni quoter contract reverted: pool not found",
      suggestion: "Try a different fee tier (500, 3000, 10000) or check token addresses"
    })
  }]
}
```

Error codes:
- `INVALID_ADDRESS` -- Malformed address
- `INVALID_ARGS` -- Bad function arguments
- `CONTRACT_REVERT` -- Contract call reverted
- `PROVIDER_UNAVAILABLE` -- External API/contract unavailable
- `TOKEN_NOT_FOUND` -- Symbol not in registry
- `RPC_ERROR` -- RPC communication failure
- `UNSUPPORTED_NETWORK` -- Network not supported

---

## 4. Plugins and Skills

Already implemented in `packages/plugins/`. Five plugins with 12 skills and 5 expert agents. These provide knowledge context for agents, not runtime functionality.

### 4.1 Plugin Summary

| Plugin | Skills | Agent | Purpose |
|--------|--------|-------|---------|
| mantle-core | mantle-network, mantle-viem, contract-deployment, token-operations | mantle-developer-expert | Network fundamentals, dev tooling |
| mantle-lsp | meth-staking, cmeth-cross-chain | lsp-expert | mETH liquid staking |
| mantle-defi | swap-guide, lending-guide, yield-farming | defi-expert | DeFi protocol integrations |
| mantle-bridge | mantle-bridge-guide, deposit-withdrawal | bridge-expert | L1-L2 bridging |
| mantle-mcp | mcp-usage-guide | (none) | MCP server documentation |

### 4.2 Skill Structure (per skill)

```
skills/<skill-name>/
├── SKILL.md              # YAML frontmatter + guide content
└── references/           # Deep-dive reference docs
    ├── <topic>.md
    └── ...
```

Required SKILL.md frontmatter:

```yaml
---
name: <skill-name>            # Must match directory name
description: <trigger phrases> # When to activate this skill
allowed-tools: <tool list>     # Tools the skill may use
model: sonnet                  # Recommended model
license: MIT
metadata:
  author: mantle
  version: '0.1.0'
---
```

### 4.3 What Needs Review in Existing Plugins

The existing plugin files were created as implementation, not spec. Implementers should review and potentially update:

1. **Contract addresses** -- Verify all addresses are current and correct against on-chain data
2. **ABI signatures** -- Validate function signatures match deployed contracts
3. **Protocol details** -- Confirm DeFi protocol specifics (fee tiers, pool addresses) are accurate
4. **Testnet data** -- Fill in any missing Sepolia testnet addresses

---

## 5. Shared Constants and Contracts

All three runtime components (CLI, MCP, plugins) reference the same Mantle ecosystem data. These constants should be sourced from a single reference.

### 5.1 Chain Configuration

| Property | Mainnet | Sepolia |
|----------|---------|---------|
| Chain ID | 5000 | 5003 |
| Native Token | MNT (18 decimals) | MNT (18 decimals) |
| RPC | `https://rpc.mantle.xyz` | `https://rpc.sepolia.mantle.xyz` |
| WebSocket | `wss://ws.mantle.xyz` | `wss://ws.sepolia.mantle.xyz` |
| Explorer | `https://explorer.mantle.xyz` | `https://explorer.sepolia.mantle.xyz` |
| Bridge UI | `https://bridge.mantle.xyz` | `https://bridge.sepolia.mantle.xyz` |
| DA Layer | EigenDA | EigenDA |
| L1 Settlement | Ethereum Mainnet | Ethereum Sepolia |

### 5.2 Token Registry (Mainnet)

| Symbol | Address | Decimals |
|--------|---------|----------|
| WMNT | `0x78c1b0C915c4FAA5FffA6CAbf0219DA63d7f4cb` | 18 |
| WETH | `0xdEAddEaDdeadDEadDEADDEAddEADDEAddead1111` | 18 |
| USDC | `0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9` | 6 |
| USDT | `0x201EBa5CC46D216Ce6DC03F6a759e8E766e956aE` | 6 |
| DAI | `0xd7183F311AF7DDD312C1E7D147c989E2A508405e` | 18 |
| mETH | `0xcDA86A272531e8640cD7F1a92c01839911B90bb0` | 18 |
| cmETH | `0xE6829d9a7eE3040e1276Fa75293Bde931859e8fA` | 18 |
| COOK | `0x31dCcD8774b8b07E4370c9E39F80884E3F77D0f0` | 18 |

### 5.3 Protocol Contracts (Mainnet)

| Protocol | Contract | Address |
|----------|----------|---------|
| Agni Finance | SwapRouter | `0x319B69888b0d11cEC22caA5034e25FfFBDc88421` |
| Agni Finance | Factory | `0x25780dc8Fc3cfBD75F33bFDAB65e969b603b2035` |
| Agni Finance | QuoterV2 | `0xc4aaDc921E1cdb66c5300Bc158a313292923C0cb` |
| Agni Finance | NFT Position Mgr | `0x218bf598D1453383e2F4AA7b14fFB9BfB102D637` |
| Merchant Moe | Router | `0xeaEE7EE68874218c3558b40063c42B82D3E7232a` |
| Merchant Moe | Factory | `0x8597db3ba8dE68DE4aD5F5De69F9b5d4834800E0` |
| Lendle | LendingPool | `0xCFa5aE7c2CE8Fadc6426C1ff872cA45378Fb7cF3` |
| Lendle | DataProvider | `0x552b9e4bae485C4B7F540777d7D25614CdB84773` |
| Pendle | Router | `0x888888888889758F76e7103c6CbF23ABbF58F946` |
| mETH LSP | Staking Mgr (L1) | `0xe3cBd06D7dadB3F4e6557bAb7EdD924CD1489E8f` |
| mETH LSP | mETH Token (L1) | `0xd5F7838F5C461fefF7FE49ea5ebaF7728bB0ADfa` |
| Permit2 | Universal | `0x000000000022D473030F116dDEE9F6B43aC78BA3` |
| Multicall3 | Universal | `0xcA11bde05977b3631167028862bE2a173976CA11` |

### 5.4 Bridge Contracts

| Contract | L1 (Ethereum) Address | L2 (Mantle) Address |
|----------|----------------------|---------------------|
| Standard Bridge | `0x95fC37A27a2f68e3A647CDc081F0A89bb47c3012` | `0x4200000000000000000000000000000000000010` |
| Cross Domain Messenger | `0x676A795fe6E43C17c668de16730c3F690FEB7120` | `0x4200000000000000000000000000000000000007` |
| L2 To L1 Message Passer | N/A | `0x4200000000000000000000000000000000000016` |
| MNT Token (L1) | `0x3c3a81e81dc49A522A592e7622A7E711c06bf354` | Native |

---

## 6. Repository Structure

```
mantle-ai/
├── cli/                                # Go CLI binary
│   ├── cmd/mantle/main.go
│   ├── internal/
│   │   ├── app/runner.go
│   │   ├── config/config.go
│   │   ├── cache/cache.go
│   │   ├── providers/
│   │   │   ├── types.go
│   │   │   ├── rpc/rpc.go
│   │   │   ├── agni/agni.go
│   │   │   ├── merchantmoe/merchantmoe.go
│   │   │   ├── lendle/lendle.go
│   │   │   ├── aurelius/aurelius.go
│   │   │   ├── meth/meth.go
│   │   │   ├── mantlebridge/bridge.go
│   │   │   ├── across/across.go
│   │   │   ├── pendle/pendle.go
│   │   │   └── defillama/defillama.go
│   │   ├── id/
│   │   ├── model/types.go
│   │   ├── out/render.go
│   │   ├── errors/errors.go
│   │   ├── schema/schema.go
│   │   ├── policy/policy.go
│   │   └── httpx/client.go
│   ├── go.mod
│   ├── Makefile
│   ├── .goreleaser.yml
│   ├── AGENTS.md
│   └── README.md
│
├── mcp-server/                         # Standalone TypeScript MCP server
│   ├── src/
│   │   ├── server.ts
│   │   ├── tools/
│   │   │   ├── index.ts
│   │   │   ├── chain.ts
│   │   │   ├── balance.ts
│   │   │   ├── transaction.ts
│   │   │   ├── contract.ts
│   │   │   ├── token.ts
│   │   │   ├── swap.ts
│   │   │   ├── lending.ts
│   │   │   ├── staking.ts
│   │   │   ├── yield.ts
│   │   │   ├── bridge.ts
│   │   │   └── explorer.ts
│   │   ├── resources/
│   │   │   ├── index.ts
│   │   │   ├── chain-config.ts
│   │   │   ├── token-registry.ts
│   │   │   └── protocol-registry.ts
│   │   ├── providers/
│   │   │   ├── rpc.ts
│   │   │   ├── agni.ts
│   │   │   ├── lendle.ts
│   │   │   ├── meth.ts
│   │   │   └── bridge.ts
│   │   ├── config/
│   │   │   ├── chains.ts
│   │   │   ├── tokens.ts
│   │   │   └── protocols.ts
│   │   └── utils/
│   │       ├── client.ts
│   │       ├── format.ts
│   │       └── errors.ts
│   ├── package.json
│   ├── tsconfig.json
│   ├── AGENTS.md
│   └── README.md
│
├── packages/plugins/                   # Claude Code plugins (already exists)
│   ├── mantle-core/
│   ├── mantle-lsp/
│   ├── mantle-defi/
│   ├── mantle-bridge/
│   └── mantle-mcp/                     # Plugin wrapper for MCP server docs
│
├── docs/
│   └── plans/
│       └── 2026-02-26-mantle-ai-full-scaffold-design.md  # This file
│
├── .claude-plugin/marketplace.json
├── scripts/validate-plugin.cjs
├── scripts/validate-all.sh
├── package.json                        # Root workspace (Nx)
├── nx.json
├── tsconfig.base.json
├── CLAUDE.md
├── AGENTS.md -> CLAUDE.md
├── LICENSE
├── README.md
└── .gitignore
```

---

## 7. Implementation Phases

### Phase 1: CLI Foundation
- Go module scaffolding (`go mod init`, Cobra setup)
- Output envelope and render system (copy defi-cli patterns)
- Schema system
- `ChainProvider` (rpc): `chain info`, `chain status`, `balance`, `tx`, `contract read`, `token info`
- Config and cache
- Install script and GoReleaser

### Phase 2: CLI DeFi Providers
- `SwapProvider`: Agni (on-chain quoter), Merchant Moe
- `LendingProvider`: Lendle (on-chain reads)
- `StakingProvider`: mETH (L1 reads)
- `YieldProvider`: aggregated from lending + Pendle
- `BridgeProvider`: Mantle Bridge, Across
- `BridgeStatusProvider`: withdrawal status tracking

### Phase 3: MCP Server
- MCP server scaffolding (`@modelcontextprotocol/sdk`, viem)
- Chain, balance, transaction, contract tools (replicating CLI chain commands as MCP tools)
- Token tools (info, resolve, balances)
- DeFi tools (swap, lending, staking, yield, bridge)
- MCP resources (chain config, token registry, protocol registry)
- Error handling and formatting

### Phase 4: Plugin/Skill Review
- Audit existing plugin content for accuracy (addresses, ABIs, protocol details)
- Update mantle-mcp plugin to document the standalone MCP server
- Add missing testnet data

### Phase 5: Polish
- Evals framework (promptfoo, per-skill suites)
- VitePress documentation site
- CI/CD workflows
- Marketplace publishing

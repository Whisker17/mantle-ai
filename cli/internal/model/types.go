package model

import "time"

const EnvelopeVersion = "v1"

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

type ProviderStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
}

type CacheStatus struct {
	Status string `json:"status"`
	AgeMS  int64  `json:"age_ms"`
	Stale  bool   `json:"stale"`
}

type ProviderInfo struct {
	Name           string                   `json:"name"`
	Type           string                   `json:"type"`
	Enabled        bool                     `json:"enabled"`
	RequiresKey    bool                     `json:"requires_key"`
	AuthConfigured bool                     `json:"auth_configured"`
	Capabilities   []string                 `json:"capabilities"`
	KeyEnvVarName  string                   `json:"key_env_var,omitempty"`
	CapabilityAuth []ProviderCapabilityAuth `json:"capability_auth,omitempty"`
}

type ProviderCapabilityAuth struct {
	Capability  string `json:"capability"`
	KeyEnvVar   string `json:"key_env_var"`
	Description string `json:"description,omitempty"`
}

type ProviderDoctorStatus struct {
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	Available     bool   `json:"available"`
	Status        string `json:"status"`
	LatencyMS     int64  `json:"latency_ms"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	LastFailureAt string `json:"last_failure_at,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	CheckedAt     string `json:"checked_at"`
}

type ChainInfo struct {
	ChainID     int    `json:"chain_id"`
	Name        string `json:"name"`
	NativeToken string `json:"native_token"`
	RPCURL      string `json:"rpc_url"`
	ExplorerURL string `json:"explorer_url"`
	BridgeURL   string `json:"bridge_url"`
	DALayer     string `json:"da_layer"`
	Source      string `json:"source,omitempty"`
}

type ChainStatus struct {
	ChainID     int    `json:"chain_id"`
	BlockNumber uint64 `json:"block_number"`
	GasPrice    string `json:"gas_price"`
	GasPriceMNT string `json:"gas_price_mnt"`
	L1DataFee   string `json:"l1_data_fee"`
	SyncStatus  string `json:"sync_status"`
	PeerCount   int    `json:"peer_count"`
}

type AddressBalance struct {
	Address    string         `json:"address"`
	MNTBalance AmountInfo     `json:"mnt_balance"`
	Tokens     []TokenBalance `json:"tokens,omitempty"`
}

type TokenBalance struct {
	Symbol  string     `json:"symbol"`
	Address string     `json:"address"`
	Balance AmountInfo `json:"balance"`
}

type AmountInfo struct {
	AmountBaseUnits string `json:"amount_base_units"`
	AmountDecimal   string `json:"amount_decimal"`
	Decimals        int    `json:"decimals"`
}

type TransactionInfo struct {
	Hash        string     `json:"hash"`
	From        string     `json:"from"`
	To          string     `json:"to"`
	Value       AmountInfo `json:"value"`
	GasUsed     string     `json:"gas_used"`
	GasPrice    string     `json:"gas_price"`
	FeeMNT      string     `json:"fee_mnt"`
	BlockNumber uint64     `json:"block_number"`
	Timestamp   time.Time  `json:"timestamp"`
	Status      string     `json:"status"`
	Input       string     `json:"input,omitempty"`
	ExplorerURL string     `json:"explorer_url"`
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

type TransferSimulation struct {
	Kind         string     `json:"kind"`
	From         string     `json:"from"`
	To           string     `json:"to"`
	Asset        string     `json:"asset"`
	TokenAddress string     `json:"token_address,omitempty"`
	Amount       AmountInfo `json:"amount"`
	GasEstimate  string     `json:"gas_estimate"`
	FeeMNT       string     `json:"fee_mnt"`
	Success      bool       `json:"success"`
	Error        string     `json:"error,omitempty"`
	Source       string     `json:"source,omitempty"`
	FetchedAt    string     `json:"fetched_at"`
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

type SwapQuote struct {
	Provider        string     `json:"provider"`
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

type LendMarket struct {
	Protocol  string  `json:"protocol"`
	Asset     string  `json:"asset"`
	SupplyAPY float64 `json:"supply_apy"`
	BorrowAPY float64 `json:"borrow_apy"`
	TVLUSD    float64 `json:"tvl_usd"`
	FetchedAt string  `json:"fetched_at"`
	Source    string  `json:"source,omitempty"`
}

type LendRate struct {
	Protocol    string  `json:"protocol"`
	Asset       string  `json:"asset"`
	SupplyAPY   float64 `json:"supply_apy"`
	BorrowAPY   float64 `json:"borrow_apy"`
	Utilization float64 `json:"utilization"`
	FetchedAt   string  `json:"fetched_at"`
	Source      string  `json:"source,omitempty"`
}

type StakeInfo struct {
	Protocol     string  `json:"protocol"`
	METHToETH    string  `json:"meth_to_eth"`
	ETHToMETH    string  `json:"eth_to_meth"`
	APY          float64 `json:"apy"`
	TotalStaked  string  `json:"total_staked"`
	TotalMETH    string  `json:"total_meth"`
	UnstakeDelay string  `json:"unstake_delay"`
	FetchedAt    string  `json:"fetched_at"`
	Source       string  `json:"source,omitempty"`
}

type StakeQuote struct {
	Action       string     `json:"action"`
	InputAmount  AmountInfo `json:"input_amount"`
	EstimatedOut AmountInfo `json:"estimated_out"`
	ExchangeRate string     `json:"exchange_rate"`
	MinOutput    string     `json:"min_output"`
	FetchedAt    string     `json:"fetched_at"`
}

type YieldOpportunity struct {
	Protocol  string  `json:"protocol"`
	Asset     string  `json:"asset"`
	Type      string  `json:"type"`
	APYBase   float64 `json:"apy_base"`
	APYReward float64 `json:"apy_reward"`
	APYTotal  float64 `json:"apy_total"`
	TVLUSD    float64 `json:"tvl_usd"`
	RiskLevel string  `json:"risk_level"`
	Score     float64 `json:"score"`
	FetchedAt string  `json:"fetched_at"`
	Source    string  `json:"source,omitempty"`
}

type BridgeQuote struct {
	Provider        string     `json:"provider"`
	FromChain       string     `json:"from_chain"`
	ToChain         string     `json:"to_chain"`
	Asset           string     `json:"asset"`
	InputAmount     AmountInfo `json:"input_amount"`
	EstimatedOut    AmountInfo `json:"estimated_out"`
	EstimatedFeeUSD float64    `json:"estimated_fee_usd"`
	EstimatedTimeS  int64      `json:"estimated_time_s"`
	Route           string     `json:"route"`
	FetchedAt       string     `json:"fetched_at"`
}

type BridgeStatus struct {
	TxHash        string `json:"tx_hash"`
	Direction     string `json:"direction"`
	Status        string `json:"status"`
	L1TxHash      string `json:"l1_tx_hash,omitempty"`
	L2TxHash      string `json:"l2_tx_hash,omitempty"`
	ChallengeEnd  string `json:"challenge_end,omitempty"`
	RemainingTime string `json:"remaining_time,omitempty"`
	ExplorerURL   string `json:"explorer_url"`
}

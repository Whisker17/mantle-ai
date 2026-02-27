package id

import "strings"

type Token struct {
	Symbol   string
	Address  string
	Decimals int
}

var tokenRegistry = map[string][]Token{
	"eip155:5000": {
		{Symbol: "WMNT", Address: "0x78c1b0C915c4FAA5FffA6CAbf0219DA63d7f4cb8", Decimals: 18},
		{Symbol: "WETH", Address: "0xdEAddEaDdeadDEadDEADDEAddEADDEAddead1111", Decimals: 18},
		{Symbol: "USDC", Address: "0x09Bc4E0D864854c6aFB6eB9A9cdF58aC190D0dF9", Decimals: 6},
		{Symbol: "USDT", Address: "0x201EBa5CC46D216Ce6DC03F6a759e8E766e956aE", Decimals: 6},
		{Symbol: "DAI", Address: "0xd7183F311AF7DDD312C1E7D147c989E2A508405e", Decimals: 18},
		{Symbol: "METH", Address: "0xcDA86A272531e8640cD7F1a92c01839911B90bb0", Decimals: 18},
		{Symbol: "CMETH", Address: "0xE6829d9a7eE3040e1276Fa75293Bde931859e8fA", Decimals: 18},
		{Symbol: "COOK", Address: "0x31dCcD8774b8b07E4370c9E39F80884E3F77D0f0", Decimals: 18},
	},
	"eip155:5003": {
		{Symbol: "WMNT", Address: "0x67A1f4A939b477A6b7c5BF94D97E45dE87E608eF", Decimals: 18},
		{Symbol: "USDC", Address: "0xaCab8129e2ce587fd203FD770EC9ecafa2c88080", Decimals: 6},
		{Symbol: "USDT", Address: "0xcC4Ac915857532adA58D69493554C6d869932fE6", Decimals: 6},
	},
}

func TokensForChain(chainID string) []Token {
	items := tokenRegistry[strings.ToLower(strings.TrimSpace(chainID))]
	out := make([]Token, 0, len(items))
	out = append(out, items...)
	return out
}

func ResolveTokenSymbol(chainID, symbol string) (Token, bool) {
	normChain := strings.ToLower(strings.TrimSpace(chainID))
	normSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	for _, token := range tokenRegistry[normChain] {
		if token.Symbol == normSymbol {
			return token, true
		}
	}
	return Token{}, false
}

func ResolveTokenAddress(chainID, address string) (Token, bool) {
	normChain := strings.ToLower(strings.TrimSpace(chainID))
	normAddress := strings.ToLower(strings.TrimSpace(address))
	for _, token := range tokenRegistry[normChain] {
		if strings.EqualFold(token.Address, normAddress) {
			return token, true
		}
	}
	return Token{}, false
}

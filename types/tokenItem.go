package types

// TokenItem 表示每个 token 的数据结构（只挑选了常用字段）
type TokenItem struct {
	ID                 int     `json:"id"`
	Chain              string  `json:"chain"`
	Address            string  `json:"address"`
	Symbol             string  `json:"symbol"`
	Price              float64 `json:"price"`
	PriceChangePercent float64 `json:"price_change_percent"`
	Volume             float64 `json:"volume"`
	Liquidity          float64 `json:"liquidity"`
	MarketCap          float64 `json:"market_cap"`
	HolderCount        int     `json:"holder_count"`
	Buys               int     `json:"buys"`
	Sells              int     `json:"sells"`
	PoolTypeStr        string  `json:"pool_type_str"`
	TwitterUsername    string  `json:"twitter_username"`
	Website            string  `json:"website"`
	SmartDegenCount    int     `json:"smart_degen_count"`
	RenownedCount      int     `json:"renowned_count"`
	PoolAddress        string  // 将在初始化时填充
}

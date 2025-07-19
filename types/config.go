package types

// Config 程序配置
type Config struct {
	DataDir          string `json:"data_dir"`
	Interval         int    `json:"interval"`
	Proxy            string `json:"proxy"`
	RSIPeriod        int    `json:"rsi_period"`
	StochRSI         int    `json:"stoch_rsi_period"`
	KPeriod          int    `json:"k_period"`
	DPeriod          int    `json:"d_period"`
	BotToken         string `json:"botToken"`
	ChatID           string `json:"chatId"`
	Url              string `json:"url"`
	Timeframe        string `json:"timeframe"`
	OneAggregate     string `json:"1_aggregate"`
	FiveAggregate    string `json:"5_aggregate"`
	FifteenAggregate string `json:"15_aggregate"`
}

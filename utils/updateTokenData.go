package utils

import (
	"fmt"
	"onchain-energe-SRSI/geckoterminal"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
	"time"
)

// updateTokenData 更新代币数据
func UpdateTokenData(id string, data *types.TokenData, config *types.Config) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()

	tokenItem := data.TokenItem

	EMA25 := GetEMA25FromDB(model.DB, tokenItem.Symbol)
	EMA50 := GetEMA50FromDB(model.DB, tokenItem.Symbol)
	PriceGT_EMA25 := GetPriceGT_EMA25FromDB(model.DB, tokenItem.Symbol)

	// 构建查询参数
	options := map[string]string{
		"aggregate":               config.FiveAggregate,
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}

	// 1.获取OHLCV数据
	var ohlcvData []geckoterminal.OHLCV
	var meta *geckoterminal.MetaData
	var err error

	// 循环尝试获取数据，直到成功或达到最大重试次数
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ohlcvData, meta, err = geckoterminal.GetOHLCV(tokenItem.Chain, tokenItem.PoolAddress, config.Timeframe, options, config.Proxy)
		if err == nil {
			// 获取成功，退出循环
			break
		}
		time.Sleep(2 * time.Second) // 等待2秒后重试
	}

	// 如果最终仍然失败
	if err != nil {
		fmt.Printf("[%s] 多次尝试后获取OHLCV数据失败: %v\n", id, err)
		return
	}

	if len(ohlcvData) == 0 {
		fmt.Printf("[%s] 未找到OHLCV数据\n", id)
		return
	}

	data.Data = ohlcvData

	// 显示代币信息
	if meta != nil && data.LastUpdated.IsZero() {
		fmt.Printf("[%s] 代币信息: %s (%s) / %s (%s)\n",
			id,
			meta.Base.Name, meta.Base.Symbol,
			meta.Quote.Name, meta.Quote.Symbol)
	}

	var closes []float64
	for _, k := range data.Data {
		closes = append(closes, k.Close)
	}

	// 计算SRSI
	_, k, _ := StochRSIFromClose(closes, config.RSIPeriod, config.StochRSI, config.KPeriod, config.DPeriod)

	// 更新时间
	data.LastUpdated = time.Now()

	// 获取最新的SRSI值
	if len(k) > 0 {
		latestRSI := k[len(k)-1]
		if latestRSI < 20 && EMA25 > EMA50 {
			message := fmt.Sprintf("🚀[%s] SRSI: %.2f (%s)",
				id, latestRSI, geckoterminal.FormatTimestamp(data.Data[len(data.Data)-1].Timestamp))
			if PriceGT_EMA25 {
				message += "🚀GT"
			}
			telegram.SendMessage(config.BotToken, config.ChatID, message)
		}
	}
}

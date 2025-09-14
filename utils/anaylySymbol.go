package utils

import (
	"fmt"
	"onchain-energe-SRSI/geckoterminal"
	"onchain-energe-SRSI/types"
	"time"
)

// AnaylySymbol 更新代币数据并执行交易模型分析
func AnaylySymbol(data *types.TokenData, config *types.Config, resultsChan chan types.TokenItem) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()

	tokenItem := data.TokenItem

	PreCheck := EMA25PreCheck(tokenItem.Symbol, data, config)
	if !PreCheck {
		return
	}

	// 构建查询参数
	options := map[string]string{
		"aggregate":               config.FifteenAggregate,
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}

	// 1.获取OHLCV数据
	var ohlcvData []geckoterminal.OHLCV
	var err error

	// 循环尝试获取数据，直到成功或达到最大重试次数
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ohlcvData, _, err = geckoterminal.GetOHLCV(tokenItem.Chain, tokenItem.PoolAddress, config.Timeframe, options, config.Proxy)
		if err == nil {
			// 获取成功，退出循环
			break
		}
		time.Sleep(2 * time.Second) // 等待2秒后重试
	}

	// 如果最终仍然失败
	if err != nil || len(ohlcvData) == 0 {
		fmt.Printf("[%s] 多次尝试后获取OHLCV数据失败: %v\n", tokenItem.Symbol, err)
		return
	} else {
		for i, j := 0, len(ohlcvData)-1; i < j; i, j = i+1, j-1 {
			ohlcvData[i], ohlcvData[j] = ohlcvData[j], ohlcvData[i]
		}
	}

	var closesM15 []float64
	for _, k := range ohlcvData {
		closesM15 = append(closesM15, k.Close)
	}
	pricePre := closesM15[len(closesM15)-2]
	pricePre2 := closesM15[len(closesM15)-3]
	_, EMA25M15NOW := CalculateEMA(closesM15, 25)

	if pricePre > EMA25M15NOW || pricePre2 > EMA25M15NOW {
		tokenItem.Emoje = "🟢"
		resultsChan <- tokenItem
		return

	}
}

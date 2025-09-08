package utils

import (
	"fmt"
	"time"

	"onchain-energe-SRSI/geckoterminal"
	"onchain-energe-SRSI/types"
)

func EMA25PreCheck(symbol string, data *types.TokenData, config *types.Config) (result bool) {

	tokenItem := data.TokenItem

	// 构建查询参数
	optionsD1 := map[string]string{
		"aggregate":               "1",
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}

	// 1.获取OHLCV数据
	var ohlcvDataD1, ohlcvDataH4, ohlcvDataH1 []geckoterminal.OHLCV
	var err error

	// 循环尝试获取数据，直到成功或达到最大重试次数
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ohlcvDataD1, _, err = geckoterminal.GetOHLCV(tokenItem.Chain, tokenItem.PoolAddress, "day", optionsD1, config.Proxy)
		if err == nil {
			// 获取成功，退出循环
			break
		}
		time.Sleep(2 * time.Second) // 等待2秒后重试
	}

	// 如果最终仍然失败
	if err != nil || len(ohlcvDataD1) == 0 {
		fmt.Printf("[%s] 多次尝试后获取OHLCV数据失败: %v\n", symbol, err)
		return false
	} else {
		for i, j := 0, len(ohlcvDataD1)-1; i < j; i, j = i+1, j-1 {
			ohlcvDataD1[i], ohlcvDataD1[j] = ohlcvDataD1[j], ohlcvDataD1[i]
		}
	}

	var closesD1 []float64
	for _, k := range ohlcvDataD1 {
		closesD1 = append(closesD1, k.Close)
	}

	priceBIG := closesD1[len(closesD1)-1]
	_, EMA25D1 := CalculateEMA(closesD1, 25)
	if priceBIG > EMA25D1 {
		// 构建查询参数
		optionsH4 := map[string]string{
			"aggregate":               "4",
			"limit":                   "200", // 只获取最新的几条数据即可
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		// 循环尝试获取数据，直到成功或达到最大重试次数
		maxRetries := 3
		for i := 0; i < maxRetries; i++ {
			ohlcvDataH4, _, err = geckoterminal.GetOHLCV(tokenItem.Chain, tokenItem.PoolAddress, "hour", optionsH4, config.Proxy)
			if err == nil {
				// 获取成功，退出循环
				break
			}
			time.Sleep(2 * time.Second) // 等待2秒后重试
		}

		// 如果最终仍然失败
		if err != nil || len(ohlcvDataH4) == 0 {
			fmt.Printf("[%s] 多次尝试后获取OHLCV数据失败: %v\n", symbol, err)
			return false
		} else {
			for i, j := 0, len(ohlcvDataH4)-1; i < j; i, j = i+1, j-1 {
				ohlcvDataH4[i], ohlcvDataH4[j] = ohlcvDataH4[j], ohlcvDataH4[i]
			}
		}

		var closesH4 []float64
		for _, k := range ohlcvDataH4 {
			closesH4 = append(closesH4, k.Close)
		}
		_, EMA25H4 := CalculateEMA(closesH4, 25)
		if priceBIG > EMA25H4 {
			// 构建查询参数
			optionsH1 := map[string]string{
				"aggregate":               "1",
				"limit":                   "200", // 只获取最新的几条数据即可
				"token":                   "base",
				"currency":                "usd",
				"include_empty_intervals": "true",
			}
			// 循环尝试获取数据，直到成功或达到最大重试次数
			maxRetries := 3
			for i := 0; i < maxRetries; i++ {
				ohlcvDataH1, _, err = geckoterminal.GetOHLCV(tokenItem.Chain, tokenItem.PoolAddress, "hour", optionsH1, config.Proxy)
				if err == nil {
					// 获取成功，退出循环
					break
				}
				time.Sleep(2 * time.Second) // 等待2秒后重试
			}

			// 如果最终仍然失败
			if err != nil || len(ohlcvDataH1) == 0 {
				fmt.Printf("[%s] 多次尝试后获取OHLCV数据失败: %v\n", symbol, err)
				return false
			} else {
				for i, j := 0, len(ohlcvDataH1)-1; i < j; i, j = i+1, j-1 {
					ohlcvDataH1[i], ohlcvDataH1[j] = ohlcvDataH1[j], ohlcvDataH1[i]
				}
			}

			var closesH1 []float64
			for _, k := range ohlcvDataH1 {
				closesH1 = append(closesH1, k.Close)
			}
			_, EMA25H1 := CalculateEMA(closesH1, 25)
			if priceBIG < EMA25H1 {
				return false
			}
		} else {
			return false
		}
	} else {
		return false
	}

	return true
}

package utils

import (
	"fmt"
	"time"

	"onchain-energe-SRSI/geckoterminal"
	"onchain-energe-SRSI/types"
)

func EMA25PreCheck(symbol string, data *types.TokenData, config *types.Config) (result bool) {

	tokenItem := data.TokenItem
	// 1.获取OHLCV数据
	var ohlcvDataH1 []geckoterminal.OHLCV
	var err error

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
	priceBIG := closesH1[len(closesH1)-1]
	_, EMA25H1 := CalculateEMA(closesH1, 25)
	MA60H1 := CalculateMA(closesH1, 60)
	DIFH1 := IsDIFUP(closesH1, 6, 13, 5)
	if priceBIG > EMA25H1 && priceBIG > MA60H1 && DIFH1 { //1H : EMA25 + DIF
		return true
	}
	return false
}

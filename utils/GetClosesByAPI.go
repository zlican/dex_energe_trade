package utils

import (
	"fmt"
	"onchain-energe-SRSI/geckoterminal"
	"onchain-energe-SRSI/types"
	"time"
)

func GetClosesByAPI(tokenItem types.TokenItem, config *types.Config, options map[string]string, TF string) (closes []float64, err error) {
	var ohlcvData []geckoterminal.OHLCV
	// 循环尝试获取数据，直到成功或达到最大重试次数
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ohlcvData, _, err = geckoterminal.GetOHLCV(tokenItem.Chain, tokenItem.PoolAddress, TF, options, config.Proxy)
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

	for _, k := range ohlcvData {
		closes = append(closes, k.Close)
	}

	return closes, err
}

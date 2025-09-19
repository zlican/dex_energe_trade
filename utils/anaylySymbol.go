package utils

import (
	"fmt"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
)

// AnaylySymbol  一次性检查1h, 15m,5m,1m
func AnaylySymbol(data *types.TokenData, config *types.Config, resultsChan chan types.TokenItem, wait_sucess_token, chatID string) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			progressLogger.Printf("[analyseSymbolForSignal] panic recovered %s : %v\n", data.TokenItem.Symbol, r)
		}
	}()

	tokenItem := data.TokenItem
	validMACD := "BUYMACD"

	//1小时检查
	optionsH1 := map[string]string{
		"aggregate":               "1",
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}
	closesH1, err := GetClosesByAPI(tokenItem, config, optionsH1, "hour")
	if err != nil {
		fmt.Println(err)
	}
	priceBIG := closesH1[len(closesH1)-2]
	_, EMA25H1 := CalculateEMA(closesH1, 25)
	MA60H1 := CalculateMA(closesH1, 60)
	DIFH1 := IsDIFUP(closesH1, 6, 13, 5)
	MACDH1 := "RANGE"
	if priceBIG > EMA25H1 && priceBIG > MA60H1 && DIFH1 { //1H : EMA25 + DIF
		MACDH1 = "BUYMACD"
	}
	if MACDH1 != validMACD {
		return
	}

	//15分钟检查
	options := map[string]string{
		"aggregate":               config.FifteenAggregate,
		"limit":                   "200",
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}

	closesM15, err := GetClosesByAPI(tokenItem, config, options, config.Timeframe)
	if err != nil {
		fmt.Println(err)
	}
	price := closesM15[len(closesM15)-2]
	_, EMA25M15NOW := CalculateEMA(closesM15, 25)
	golden := IsGolden(closesM15, 6, 13, 5)
	DIFM15UP := IsDIFUP(closesM15, 6, 13, 5)

	MACDM15 := "RANGE"
	if price > EMA25M15NOW && golden && DIFM15UP {
		MACDM15 = "BUYMACD"
	}
	if MACDM15 != validMACD {
		return
	}

	//5分钟检查
	optionsM5 := map[string]string{
		"aggregate":               config.FiveAggregate,
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}
	closesM5, err := GetClosesByAPI(tokenItem, config, optionsM5, config.Timeframe)
	if err != nil {
		fmt.Println(err)
	}
	priceM5 := closesM5[len(closesM5)-2]
	ma60M5 := CalculateMA(closesM5, 60)
	_, ema25M5Now := CalculateEMA(closesM5, 25)
	MACDSmallUP := IsSmallTFUP(closesM5, 6, 13, 5)

	MACDM5 := "RANGE"
	if priceM5 > ema25M5Now && priceM5 > ma60M5 && MACDSmallUP {
		MACDM5 = "BUYMACD"
	}
	if MACDM5 != validMACD {
		return
	}

	//1分钟检查
	optionsM1 := map[string]string{
		"aggregate":               config.OneAggregate,
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}
	closesM1, err := GetClosesByAPI(tokenItem, config, optionsM1, config.Timeframe)
	if err != nil {
		fmt.Println(err)
	}
	priceM1 := closesM1[len(closesM1)-2]
	ma60M1 := CalculateMA(closesM1, 60)
	XSTRONGUPM1 := XSTRONGUP(closesM1, 6, 13, 5)

	MACDM1 := ""
	if priceM1 > ma60M1 && XSTRONGUPM1 {
		MACDM1 = "XBUY"
	}

	if MACDH1 == validMACD && MACDM15 == validMACD && MACDM5 == validMACD && MACDM1 == "XBUY" {
		msg := fmt.Sprintf("%s%s\n📬 `%s`", tokenItem.Emoje, tokenItem.Symbol, tokenItem.Address)
		// 错误注释：Telegram 发送失败依赖其内置重试机制，失败后跳过状态更新
		if err := telegram.SendMarkdownMessage(wait_sucess_token, chatID, msg); err != nil {
			fmt.Printf("发送 Telegram 买入消息失败 (%s): %v\n", tokenItem.Symbol, err)
			return
		}
	}
}

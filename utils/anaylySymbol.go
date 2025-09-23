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

	//4小时检查
	optionsH4 := map[string]string{
		"aggregate":               "4",
		"limit":                   "200",
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}

	closesH4, err := GetClosesByAPI(tokenItem, config, optionsH4, "hour")
	if err != nil {
		fmt.Println(err)
	}
	price := closesH4[len(closesH4)-2]
	_, EMA25H4NOW := CalculateEMA(closesH4, 25)
	ColANDDIFUPH4 := ColANDDIFUP(closesH4, 6, 13, 5)

	MACDH4 := "RANGE"
	if price > EMA25H4NOW && ColANDDIFUPH4 {
		MACDH4 = "BUYMACD"
	}
	if MACDH4 != validMACD {
		return
	}

	//1小时检查
	optionsH1 := map[string]string{
		"aggregate":               "1",
		"limit":                   "200",
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}
	closesH1, err := GetClosesByAPI(tokenItem, config, optionsH1, "hour")
	if err != nil {
		fmt.Println(err)
	}
	DIFH1 := IsDIFUP(closesH1, 6, 13, 5)
	MACDH1 := "RANGE"
	if DIFH1 { //1H :DIF
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
	price = closesM15[len(closesM15)-2]
	_, EMA25M15NOW := CalculateEMA(closesM15, 25)
	ColANDDIFUPM15 := ColANDDIFUP(closesM15, 6, 13, 5)
	DIFM15UP := IsDIFUP(closesM15, 6, 13, 5)

	MACDM15 := "RANGE"
	if price > EMA25M15NOW && DIFM15UP && ColANDDIFUPM15 {
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
	DIFUPM5 := IsDIFUP(closesM5, 6, 13, 5)

	MACDM5 := "RANGE"
	if priceM5 > ma60M5 && DIFUPM5 {
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
	DIFUPM1 := IsDIFUP(closesM1, 6, 13, 5)
	ColANDDIFUPM1 := ColANDDIFUPMicro(closesM1, 6, 13, 5)

	MACDM1 := ""
	if priceM1 > ma60M1 && DIFUPM1 && ColANDDIFUPM1 {
		MACDM1 = "XBUY"
	}

	if MACDH4 == validMACD && MACDH1 == validMACD && MACDM15 == validMACD && MACDM5 == validMACD && MACDM1 == "XBUY" {
		msg := fmt.Sprintf("%s%s\n📬 `%s`", tokenItem.Emoje, tokenItem.Symbol, tokenItem.Address)
		// 错误注释：Telegram 发送失败依赖其内置重试机制，失败后跳过状态更新
		if err := telegram.SendMarkdownMessage(wait_sucess_token, chatID, msg); err != nil {
			fmt.Printf("发送 Telegram 买入消息失败 (%s): %v\n", tokenItem.Symbol, err)
			return
		}
	}
}

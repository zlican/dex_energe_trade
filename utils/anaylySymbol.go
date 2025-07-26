package utils

import (
	"fmt"
	"log"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
)

// updateTokenData 更新代币数据
func AnaylySymbol(data *types.TokenData, config *types.Config, resultsChan chan types.TokenItem) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()

	tokenItem := data.TokenItem

	//获取Closes
	options := map[string]string{
		"aggregate":               config.FiveAggregate,
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}
	closes, err := GetClosesByAPI(tokenItem, config, options)
	if err != nil {
		return
	}
	EMA25M5, EMA50M5, _ := Get5MEMAFromDB(model.DB, tokenItem.Symbol)
	EMA25M15, EMA50M15 := Get15MEMAFromDB(model.DB, tokenItem.Symbol)
	SRSIM5 := Get5SRSIFromDB(model.DB, tokenItem.Symbol)

	UpMACD := IsAboutToGoldenCross(closes, 6, 13, 5)

	var MainTrend bool
	MainTrend = EMA25M5 > EMA50M5

	var up bool
	up = EMA25M15 > EMA50M15 && MainTrend

	buyCond := SRSIM5 < 25

	switch {
	case up && buyCond:
		optionsM1 := map[string]string{
			"aggregate":               config.OneAggregate,
			"limit":                   "200", // 只获取最新的几条数据即可
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM1, err := GetClosesByAPI(tokenItem, config, optionsM1)
		if err != nil {
			return
		}
		EMA25M1 := CalculateEMA(closesM1, 25)
		EMA50M1 := CalculateEMA(closesM1, 50)

		if EMA25M1[len(EMA25M1)-1] > EMA50M1[len(EMA50M1)-1] && UpMACD {
			// 完全满足，直接推送
			msg := fmt.Sprintf("🟢%s\n📬 Address:\n`%s`", data.Symbol, data.TokenItem.Address)
			err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg)
			if err != nil {
				log.Println("发送失败:", err)
			}
		} else {
			// 不满足但接近，加入等待区
			resultsChan <- tokenItem
		}

	}
}

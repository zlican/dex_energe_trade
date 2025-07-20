package utils

import (
	"fmt"
	"log"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
)

// updateTokenData 更新代币数据
func UpdateTokenData(data *types.TokenData, config *types.Config) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()

	tokenItem := data.TokenItem

	// 构建查询参数
	options := map[string]string{
		"aggregate":               config.OneAggregate,
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}

	closes, err := GetClosesByAPI(tokenItem, config, options)
	if err != nil {
		return
	}
	price := closes[len(closes)-1]
	EMA25M1 := CalculateEMA(closes, 25)
	EMA50M1 := CalculateEMA(closes, 50)
	EMA25M5, EMA50M5, EMA169M5 := Get5MEMAFromDB(model.DB, tokenItem.Symbol)
	EMA25M15, EMA50M15 := Get15MEMAFromDB(model.DB, tokenItem.Symbol)
	PriceGT_EMA25 := GetPriceGT_EMA25FromDB(model.DB, tokenItem.Symbol)
	SRSIM15 := Get15MSRSIFromDB(model.DB, tokenItem.Symbol)
	SRSIM5 := Get5SRSIFromDB(model.DB, tokenItem.Symbol)

	var up, longUp bool
	up = PriceGT_EMA25 && EMA25M5 > EMA50M5
	longUp = EMA25M15 > EMA50M15 && price > EMA169M5

	buyCond := SRSIM5 < 25
	longBuyCond := SRSIM15 < 20 && SRSIM5 < 25

	var status string
	switch {
	case up && buyCond:
		if EMA25M1[len(EMA25M1)-1] > EMA50M1[len(EMA50M1)-1] && price > EMA25M5 {
			status = "Soon"
		} else {
			status = "Wait"
		}
		msg := fmt.Sprintf(
			"🟢%s (%s)\n📬 Address:\n`%s`",
			data.Symbol, status, data.TokenItem.Address,
		)

		err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg)
		if err != nil {
			log.Println("发送失败:", err)
		}
	case longUp && longBuyCond:
		msg := fmt.Sprintf(
			"🟢%s (longBuy)\n📬 Address:\n`%s`",
			data.Symbol, data.TokenItem.Address,
		)

		err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg)
		if err != nil {
			log.Println("发送失败:", err)
		}
	}

}

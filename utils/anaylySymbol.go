package utils

import (
	"fmt"
	"log"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
)

// updateTokenData 更新代币数据
func AnaylySymbol(data *types.TokenData, config *types.Config) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()

	tokenItem := data.TokenItem

	EMA25M5, EMA50M5, _ := Get5MEMAFromDB(model.DB, tokenItem.Symbol)
	EMA25M15, EMA50M15 := Get15MEMAFromDB(model.DB, tokenItem.Symbol)
	SRSIM5 := Get5SRSIFromDB(model.DB, tokenItem.Symbol)

	var MainTrend bool
	MainTrend = EMA25M5 > EMA50M5

	var up bool
	up = EMA25M15 > EMA50M15 && MainTrend

	buyCond := SRSIM5 < 25

	switch {
	case up && buyCond:
		msg := fmt.Sprintf(
			"🟢%s\n📬 Address:\n`%s`",
			data.Symbol, data.TokenItem.Address,
		)
		err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg)
		if err != nil {
			log.Println("发送失败:", err)
		}

	}

}

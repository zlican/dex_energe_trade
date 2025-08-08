package utils

import (
	"fmt"
	"log"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
)

// AnaylySymbol 更新代币数据并执行交易模型分析
func AnaylySymbol(data *types.TokenData, config *types.Config, resultsChan chan types.TokenItem) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()

	tokenItem := data.TokenItem

	// ===== 获取5分钟数据 =====
	optionsM5 := map[string]string{
		"aggregate":               config.FiveAggregate,
		"limit":                   "200",
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}
	closesM5, err := GetClosesByAPI(tokenItem, config, optionsM5)
	if err != nil || len(closesM5) < 2 {
		return
	}
	price := closesM5[len(closesM5)-1]

	EMA25M5, EMA50M5, _ := Get5MEMAFromDB(model.DB, tokenItem.Symbol)
	EMA25M15, _ := Get15MEMAFromDB(model.DB, tokenItem.Symbol)
	SRSIM5 := Get5SRSIFromDB(model.DB, tokenItem.Symbol)

	UpMACDM5 := IsAboutToGoldenCross(closesM5, 6, 13, 5)

	up := price > EMA25M15 && EMA25M5 > EMA50M5
	buyCond := SRSIM5 < 35

	// ===== 获取1分钟EMA（仅当需要时调用，减少API消耗） =====
	get1MData := func() ([]float64, []float64, []float64, bool) {
		optionsM1 := map[string]string{
			"aggregate":               config.OneAggregate,
			"limit":                   "200",
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM1, err := GetClosesByAPI(tokenItem, config, optionsM1)
		if err != nil || len(closesM1) < 2 {
			return nil, nil, nil, false
		}
		EMA25M1 := CalculateEMA(closesM1, 25)
		EMA50M1 := CalculateEMA(closesM1, 50)
		return closesM1, EMA25M1, EMA50M1, true
	}
	closesM1, EMA25M1, EMA50M1, ok := get1MData()
	if !ok {
		return
	}
	UpMACDM1 := IsAboutToGoldenCross(closesM1, 6, 13, 5)

	// ===== 模型1（优先级最高） =====
	if up && buyCond {
		if EMA25M1[len(EMA25M1)-1] > EMA50M1[len(EMA50M1)-1] && UpMACDM5 && UpMACDM1 {
			// 完全满足，直接推送
			msg := fmt.Sprintf("🟢%s\n📬 `%s`", data.Symbol, data.TokenItem.Address)
			if err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg); err != nil {
				log.Println("发送失败:", err)
			}
		} else {
			// 不满足但接近，加入等待区
			resultsChan <- tokenItem
		}
		return // 模型1触发后直接返回，不执行模型2
	}

	// ===== 模型2（仅模型1未触发时执行） =====

	if price > EMA25M15 && EMA25M5 > EMA50M5 && EMA25M1[len(EMA25M1)-1] > EMA50M1[len(EMA50M1)-1] && UpMACDM5 && UpMACDM1 {
		msg := fmt.Sprintf("🟣%s\n📬 `%s`", data.Symbol, data.TokenItem.Address)
		if err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg); err != nil {
			log.Println("发送失败:", err)
		}
	}
}

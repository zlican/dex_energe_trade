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

	// ===== 获取1分钟EMA（仅当需要时调用，减少API消耗） =====
	get1MData := func() ([]float64, []float64, []float64, float64, bool) {
		optionsM1 := map[string]string{
			"aggregate":               config.OneAggregate,
			"limit":                   "200",
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM1, err := GetClosesByAPI(tokenItem, config, optionsM1)
		if err != nil || len(closesM1) < 2 {
			return nil, nil, nil, 0, false
		}
		EMA25M1 := CalculateEMA(closesM1, 25)
		EMA50M1 := CalculateEMA(closesM1, 50)
		MA60 := CalculateMA(closesM1, 60)
		return closesM1, EMA25M1, EMA50M1, MA60, true
	}
	closesM1, EMA25M1, _, MA60M1, ok := get1MData()
	if !ok {
		return
	}

	price := closesM1[len(closesM1)-2]

	//MACD模型
	UpMACDM1 := IsGoldenCross(closesM1, 6, 13, 5)
	XUpMACDM1 := IsGolden(closesM1, 6, 13, 5)
	var MACDM1 string
	M1UPEMA := EMA25M1[len(EMA25M1)-1] > MA60M1
	M1DOWNEMA := EMA25M1[len(EMA25M1)-1] < MA60M1
	if M1UPEMA && UpMACDM1 && (price > MA60M1 || XUpMACDM1) { //金叉回调
		MACDM1 = "BUYMACD"
	} else if M1DOWNEMA && price > EMA25M1[len(EMA25M1)-1] && XUpMACDM1 && price > MA60M1 { //死叉反转
		MACDM1 = "BUYMACD"
	} else if price > MA60M1 {
		MACDM1 = "UPRANGE"
	}

	MACDM15 := Get15MStatusFromDB(model.DB, tokenItem.Symbol)
	MACDM5 := Get5MStatusFromDB(model.DB, tokenItem.Symbol)

	// ===== 模型1（优先级最高） =====
	if MACDM15 == "BUYMACD" {
		tokenItem.Emoje = "🟢"
		if (MACDM5 == "BUYMACD" || MACDM5 == "UPRANGE") && (MACDM1 == "BUYMACD" || MACDM1 == "UPRANGE") {
			// 完全满足，直接推送
			msg := fmt.Sprintf("🟢%s\n📬 `%s`", data.Symbol, data.TokenItem.Address)
			if err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg); err != nil {
				log.Println("发送失败:", err)
			}
			resultsChan <- tokenItem
		} else {
			// 不满足但接近，加入等待区
			resultsChan <- tokenItem
		}
		return // 模型1触发后直接返回，不执行模型2
	}
}

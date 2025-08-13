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

	price := closesM1[len(closesM1)-1]

	EMA25M5, EMA50M5, _ := Get5MEMAFromDB(model.DB, tokenItem.Symbol)
	EMA25M15, EMA50M15 := Get15MEMAFromDB(model.DB, tokenItem.Symbol)
	SRSIM5 := Get5SRSIFromDB(model.DB, tokenItem.Symbol)

	up := price > EMA25M15 && EMA25M15 > EMA50M15 && EMA25M5 > EMA50M5
	buyCond := SRSIM5 < 35

	//MACD模型
	_, XUpMACDM5 := GetMACDFromDB(model.DB, tokenItem.Symbol)
	UpMACDM1 := IsAboutToGoldenCrossM1(closesM1, 6, 13, 5) //防插针版
	XUpMACDM1 := IsGoldenM1(closesM1, 6, 13, 5)
	var BuyMACDM1 bool
	M1UPEMA := EMA25M1[len(EMA25M1)-1] > EMA50M1[len(EMA50M1)-1]
	M1DOWNEMA := EMA25M1[len(EMA25M1)-1] < EMA50M1[len(EMA50M1)-1]
	if M1UPEMA && price > EMA25M1[len(EMA25M1)-1] && UpMACDM1 { //金叉浅回调
		BuyMACDM1 = true
	} else if M1UPEMA && price < EMA25M1[len(EMA25M1)-1] && XUpMACDM1 { //金叉深回调
		BuyMACDM1 = true
	} else if M1DOWNEMA && price > EMA25M1[len(EMA25M1)-1] && XUpMACDM1 { //死叉反转
		BuyMACDM1 = true
	} else {
		BuyMACDM1 = false
	}

	/* 	Model3UP := price < EMA25M15 && EMA25M15 > EMA50M15 //15分钟随机漫步
	   	Model3BuyMACD := XUpMACDM1 && XUpMACDM5             //双重MACD看多 */

	// ===== 模型1（优先级最高） =====
	if up && buyCond {
		tokenItem.Emoje = "🟢"
		if XUpMACDM5 && BuyMACDM1 {
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
	tokenItem.Emoje = "🟣"
	if price > EMA25M15 && EMA25M5 > EMA50M5 && XUpMACDM5 && BuyMACDM1 {
		msg := fmt.Sprintf("🟣%s\n📬 `%s`", data.Symbol, data.TokenItem.Address)
		if err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg); err != nil {
			log.Println("发送失败:", err)
		}
	} else {
		resultsChan <- tokenItem
	}

	/* 	// ===== 模型3 反转模型 =====
	   	if Model3UP && Model3BuyMACD {
	   		msg := fmt.Sprintf("🟡%s\n📬 `%s`", data.Symbol, data.TokenItem.Address)
	   		if err := telegram.SendMarkdownMessage(config.BotToken, config.ChatID, msg); err != nil {
	   			log.Println("发送失败:", err)
	   		}
	   	} */
}

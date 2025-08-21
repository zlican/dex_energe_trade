package utils

import (
	"database/sql"
	"fmt"
	"log"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
	"strings"
	"sync"
	"time"
)

type waitToken struct {
	Symbol    string
	TokenItem types.TokenItem
	AddedAt   time.Time
}

var waitMu sync.Mutex
var waitList = make(map[string]waitToken)

// sendWaitListBroadcast 用于主动推送等待区列表
func sendWaitListBroadcast(now time.Time, waiting_token, chatID string) {
	waitMu.Lock()
	defer waitMu.Unlock()

	if len(waitList) == 0 {
		telegram.SendMarkdownMessageWaiting(waiting_token, chatID, "等待区为空")
		return
	}

	var msgBuilder strings.Builder

	var emoje string

	for _, token := range waitList {
		emoje = token.TokenItem.Emoje
		msgBuilder.WriteString(fmt.Sprintf("%s %-12s\n📬 `%s`\n", emoje, token.Symbol, token.TokenItem.Address))
	}
	msg := msgBuilder.String()
	log.Printf("📤 推送等待区更新列表，共 %d 个代币", len(waitList))
	telegram.SendMarkdownMessageWaiting(waiting_token, chatID, msg)
}

func waitUntilNext1Min() time.Duration {
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	return time.Until(next)
}

func WaitEnerge(resultsChan chan types.TokenItem, db *sql.DB, wait_sucess_token, chatID string, waiting_token string, config *types.Config) {
	go func() {
		// 首次对齐等待，直到下一个 1 分钟整点
		time.Sleep(waitUntilNext1Min())
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for now := range ticker.C {
			go func(now time.Time) {
				var changed bool // 是否发生了删除

				waitMu.Lock()
				waitCopy := make(map[string]waitToken)
				for k, v := range waitList {
					waitCopy[k] = v
				}
				waitMu.Unlock()

				for sym, token := range waitCopy {
					optionsM1 := map[string]string{
						"aggregate":               config.OneAggregate,
						"limit":                   "200", // 只获取最新的几条数据即可
						"token":                   "base",
						"currency":                "usd",
						"include_empty_intervals": "true",
					}
					closesM1, err := GetClosesByAPI(token.TokenItem, config, optionsM1)
					if err != nil {
						return
					}
					price := closesM1[len(closesM1)-2]
					EMA25M1 := CalculateEMA(closesM1, 25)
					MA60M1 := CalculateMA(closesM1, 60)

					//MACD模型
					UpMACDM1 := IsGoldenCross(closesM1, 6, 13, 5) //防插针版
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
					MACDM15 := Get15MStatusFromDB(model.DB, token.Symbol)
					MACDM5 := Get5MStatusFromDB(model.DB, token.Symbol)
					if MACDM15 == "BUYMACD" && (MACDM5 == "BUYMACD" || MACDM5 == "UPRANGE") && (MACDM1 == "BUYMACD" || MACDM1 == "UPRANGE") {
						msg := fmt.Sprintf("监控回响：%s%s\n📬 `%s`", token.TokenItem.Emoje, sym, token.TokenItem.Address)
						telegram.SendMarkdownMessage(wait_sucess_token, chatID, msg)
						log.Printf("🟢 等待成功 Buy : %s", sym)
					} else if MACDM15 != "BUYMACD" {
						log.Printf("❌ Wait失败 Buy : %s", sym)
						waitMu.Lock()
						delete(waitList, sym)
						waitMu.Unlock()
						changed = true
					}

					if now.Sub(token.AddedAt) > 3*time.Hour {
						log.Printf("⏰ Wait超时清理 : %s", sym)
						waitMu.Lock()
						delete(waitList, sym)
						waitMu.Unlock()
						changed = true
					}
				}
				if changed {
					sendWaitListBroadcast(now, waiting_token, chatID)
				}
			}(now)
		}
	}()

	// 接收新 results 并更新 waitList（逻辑不变）
	for coin := range resultsChan {
		var newAdded bool
		now := time.Now()

		waitMu.Lock()
		_, exists := waitList[coin.Symbol]
		if !exists {
			waitList[coin.Symbol] = waitToken{
				Symbol:    coin.Symbol,
				TokenItem: coin,
				AddedAt:   now,
			}
			log.Printf("✅ 添加或替换等待代币: %s", coin.Symbol)
			newAdded = true

		}

		waitMu.Unlock()

		if newAdded {
			sendWaitListBroadcast(now, waiting_token, chatID)
		}
	}
}

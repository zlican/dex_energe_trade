package utils

import (
	"database/sql"
	"fmt"
	"log"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
	"strings"
	"sync"
	"time"
)

type waitToken struct {
	Symbol              string
	TokenItem           types.TokenItem
	AddedAt             time.Time
	LastPushedOperation string // 新增字段：记录最后一次推送的操作
	LastInvalidPushed   bool   // 新增字段：是否已经推送过失效消息
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
func executeWaitCheck(db *sql.DB, wait_sucess_token, chatID, waiting_token string, config *types.Config, now time.Time) {
	var changed bool

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

		optionsM5 := map[string]string{
			"aggregate":               config.FiveAggregate,
			"limit":                   "200", // 只获取最新的几条数据即可
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM5, err := GetClosesByAPI(token.TokenItem, config, optionsM5)
		if err != nil {
			return
		}
		EMA25M5 := CalculateEMA(closesM5, 25)

		//MACD模型
		var MACDM1, MACDM5 string
		if price > MA60M1 && price > EMA25M1[len(EMA25M1)-1] {
			MACDM1 = "BUYMACD"
		}
		DEAUP := IsDEAUP(closesM5, 6, 13, 5)
		if price > EMA25M5[len(EMA25M5)-1] && DEAUP {
			MACDM5 = "BUYMACD"
		}

		if MACDM5 == "BUYMACD" && MACDM1 == "BUYMACD" {
			if token.LastPushedOperation != "BUY" {
				msg := fmt.Sprintf("%s%s\n📬 `%s`", token.TokenItem.Emoje, sym, token.TokenItem.Address)
				telegram.SendMarkdownMessage(wait_sucess_token, chatID, msg)
				waitMu.Lock()
				t := waitList[sym]
				t.LastPushedOperation = "BUY"
				t.LastInvalidPushed = false // 重置失效推送标志
				waitList[sym] = t
				waitMu.Unlock()
			}
		} else if MACDM5 != "BUYMACD" {
			waitMu.Lock()
			// 如果之前推送过买入信号，而且还没发过“失效”消息
			t := waitList[sym]
			if t.LastPushedOperation == "BUY" && !t.LastInvalidPushed {
				msg := fmt.Sprintf("⚠️信号失效：%s", sym)
				telegram.SendMessage(wait_sucess_token, chatID, msg)
				t.LastInvalidPushed = true
				waitList[sym] = t
			}
			delete(waitList, sym) // 删除
			waitMu.Unlock()
			changed = true
		} else {
			waitMu.Lock()
			t := waitList[sym]
			if t.LastPushedOperation == "BUY" && !t.LastInvalidPushed {
				msg := fmt.Sprintf("⚠️信号失效：%s", sym)
				telegram.SendMessage(wait_sucess_token, chatID, msg)
			}
			t.LastPushedOperation = "" // 清空，允许下次推送
			t.LastInvalidPushed = true
			waitList[sym] = t
			waitMu.Unlock()
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
}

func WaitEnerge(resultsChan chan types.TokenItem, db *sql.DB, wait_sucess_token, chatID string, waiting_token string, config *types.Config) {
	go func() {
		// 🚀 启动时立即执行一次
		now := time.Now()
		executeWaitCheck(db, wait_sucess_token, chatID, waiting_token, config, now)

		// 等到下一个 1 分钟整点
		time.Sleep(waitUntilNext1Min())

		// 每 1 分钟触发（分钟 %1==0）
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for now := range ticker.C {
			go executeWaitCheck(db, wait_sucess_token, chatID, waiting_token, config, now)
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

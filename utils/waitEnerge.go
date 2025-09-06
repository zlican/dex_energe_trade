package utils

import (
	"database/sql"
	"fmt"
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
		var MACDM1, MACDM5 string
		MACDM15 := Get15MStatusFromDB(db, sym)

		optionsM1 := map[string]string{
			"aggregate":               config.OneAggregate,
			"limit":                   "200", // 只获取最新的几条数据即可
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM1, err := GetClosesByAPI(token.TokenItem, config, optionsM1)
		if err != nil {
			continue
		}
		price := closesM1[len(closesM1)-2]
		MA60M1 := CalculateMA(closesM1, 60)
		XSTRONGM1 := XSTRONG(closesM1, 6, 13, 5)
		if price > MA60M1 && XSTRONGM1 {
			MACDM1 = "BUYMACD"
		}

		optionsM5 := map[string]string{
			"aggregate":               config.FiveAggregate,
			"limit":                   "200", // 只获取最新的几条数据即可
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM5, err := GetClosesByAPI(token.TokenItem, config, optionsM5)
		if err != nil {
			continue
		}

		MA60M5 := CalculateMA(closesM5, 60)
		EMA25M5 := CalculateEMA(closesM5, 25)
		EMA25M5NOW := EMA25M5[len(EMA25M5)-1]
		DIFUP := IsDIFUP(closesM5, 6, 13, 5)
		if price > EMA25M5NOW && price > MA60M5 && DIFUP {
			MACDM5 = "BUYMACD"
		}
		if XSTRONG(closesM5, 6, 13, 5) && price > MA60M5 {
			MACDM5 = "XBUYMID"
		}

		if MACDM15 == "BUYMACD" && ((MACDM5 == "BUYMACD" && MACDM1 == "BUYMACD") || MACDM5 == "XBUYMID") {
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
		} else if MACDM5 != "BUYMACD" && MACDM5 != "XBUYMID" {
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
		// 先消费一次已有消息，保证 waitList 不为空
		drainResults(resultsChan)

		// 🚀 启动时立即执行一次
		now := time.Now()
		executeWaitCheck(db, wait_sucess_token, chatID, waiting_token, config, now)

		// 等到下一个 1 分钟整点
		time.Sleep(waitUntilNext1Min())

		// 每 1 分钟触发
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for now := range ticker.C {
			go executeWaitCheck(db, wait_sucess_token, chatID, waiting_token, config, now)
		}
	}()

	// 常规消费
	for coin := range resultsChan {
		addToWaitList(coin, waiting_token, chatID)
	}
}

func drainResults(resultsChan chan types.TokenItem) {
	drain := true
	for drain {
		select {
		case coin := <-resultsChan:
			addToWaitList(coin, "", "")
		default:
			drain = false
		}
	}
}

func addToWaitList(coin types.TokenItem, waiting_token, chatID string) {
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
		newAdded = true
	}
	waitMu.Unlock()

	if newAdded && waiting_token != "" {
		sendWaitListBroadcast(now, waiting_token, chatID)
	}
}

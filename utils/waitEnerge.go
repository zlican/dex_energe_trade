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
	LastPushedOperation string // 记录最后一次推送的操作
	LastInvalidPushed   bool   // 是否已经推送过失效消息
}

var waitMu sync.Mutex
var waitList = make(map[string]waitToken)

// sendWaitListBroadcast 用于主动推送等待区列表
func sendWaitListBroadcast(now time.Time, waiting_token, chatID string) {
	if len(waitList) == 0 {
		// 错误注释：Telegram 发送失败依赖其内置指数退避重试机制
		telegram.SendMarkdownMessageWaiting(waiting_token, chatID, "等待区为空")
		return
	}

	var msgBuilder strings.Builder
	for _, token := range waitList {
		emoji := token.TokenItem.Emoje
		msgBuilder.WriteString(fmt.Sprintf("%s %-12s\n📬 `%s`\n", emoji, token.Symbol, token.TokenItem.Address))
	}
	msg := msgBuilder.String()
	// 错误注释：Telegram 发送失败依赖其内置重试机制，可能导致用户未收到等待区更新
	telegram.SendMarkdownMessageWaiting(waiting_token, chatID, msg)
}

// handleOperation 处理买入信号逻辑
// 返回值：bool 表示是否从 waitList 删除代币
func handleOperation(sym string, token waitToken, mid bool, MACDM1, MACDM5, MACDM15, wait_sucess_token, chatID string) bool {
	// 条件 1：信号有效，发送买入信号
	if MACDM15 == "BUYMACD" && ((MACDM5 == "BUYMACD" && MACDM1 == "XBUY") || MACDM5 == "XBUY") {
		if token.LastPushedOperation != "BUY" {
			msg := fmt.Sprintf("%s%s\n📬 `%s`", token.TokenItem.Emoje, sym, token.TokenItem.Address)
			// 错误注释：Telegram 发送失败依赖其内置重试机制，失败后跳过状态更新
			if err := telegram.SendMarkdownMessage(wait_sucess_token, chatID, msg); err != nil {
				fmt.Printf("发送 Telegram 买入消息失败 (%s): %v\n", sym, err)
				return false
			}
			t := waitList[sym]
			t.LastPushedOperation = "BUY"
			t.LastInvalidPushed = false // 重置失效推送标志
			waitList[sym] = t
		}
		return false
	}
	// 条件 2：5分钟信号失效，从 waitList 删除
	if !mid {
		t := waitList[sym]
		if t.LastPushedOperation == "BUY" && !t.LastInvalidPushed {
			msg := fmt.Sprintf("⚠️信号失效：%s", sym)
			// 错误注释：Telegram 发送失败依赖其内置重试机制，失败后仍删除代币以避免重复处理
			if err := telegram.SendMessage(wait_sucess_token, chatID, msg); err != nil {
				fmt.Printf("发送 Telegram 失效消息失败 (%s): %v\n", sym, err)
			} else {
				t.LastInvalidPushed = true
				waitList[sym] = t
			}
		}
		delete(waitList, sym)
		return true
	}

	// 条件 3：其他情况，发送失效消息并清空推送状态
	t := waitList[sym]
	if t.LastPushedOperation == "BUY" && !t.LastInvalidPushed {
		msg := fmt.Sprintf("⚠️信号失效：%s", sym)
		// 错误注释：Telegram 发送失败依赖其内置重试机制，失败后仍更新状态以避免重复发送
		if err := telegram.SendMessage(wait_sucess_token, chatID, msg); err != nil {
			fmt.Printf("发送 Telegram 失效消息失败 (%s): %v\n", sym, err)
		}
		t.LastInvalidPushed = true
	}
	t.LastPushedOperation = "" // 清空，允许下次推送
	waitList[sym] = t
	return false
}

func executeWaitCheck(db *sql.DB, wait_sucess_token, chatID, waiting_token string, config *types.Config, now time.Time) {
	// 使用 defer 捕获可能的 panic
	defer func() {
		if r := recover(); r != nil {
			// 错误注释：捕获 panic，避免程序崩溃，需记录详细日志以便调试
			fmt.Printf("[executeWaitCheck] Panic recovered: %v\n", r)
		}
	}()

	time.Sleep(10 * time.Second) // 保持原有延迟

	var changed bool // 是否发生了删除

	// 单次锁定，复制 waitList 以避免并发修改
	waitMu.Lock()
	waitCopy := make(map[string]waitToken)
	for k, v := range waitList {
		waitCopy[k] = v
	}
	waitMu.Unlock()

	// 单次锁定处理所有代币
	waitMu.Lock()
	defer waitMu.Unlock()

	for sym, token := range waitCopy {
		var MACDM1, MACDM5 string
		var mid bool
		// 错误注释：Get15MStatusFromDB 可能因数据库连接失败返回空值，需检查
		MACDM15 := Get15MStatusFromDB(db, sym)

		// 获取 1 分钟 K 线数据
		optionsM1 := map[string]string{
			"aggregate":               config.OneAggregate,
			"limit":                   "200",
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM1, err := GetClosesByAPI(token.TokenItem, config, optionsM1)
		if err != nil || len(closesM1) < 2 {
			// 错误注释：API 获取失败或数据不足，跳过以避免 panic
			fmt.Printf("获取 %s (1m) 数据失败或不足: %v\n", sym, err)
			continue
		}
		price := closesM1[len(closesM1)-2]
		MA60M1 := CalculateMA(closesM1, 60)
		XSTRONGM1 := XSTRONG(closesM1, 6, 13, 5)
		DIFM1 := IsDIFUP(closesM1, 6, 13, 5)
		if price > MA60M1 && XSTRONGM1 && DIFM1 {
			MACDM1 = "XBUY"
		}

		// 获取 5 分钟 K 线数据
		optionsM5 := map[string]string{
			"aggregate":               config.FiveAggregate,
			"limit":                   "200",
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM5, err := GetClosesByAPI(token.TokenItem, config, optionsM5)
		if err != nil || len(closesM5) == 0 {
			// 错误注释：API 获取失败或数据为空，跳过以避免 panic
			fmt.Printf("获取 %s (5m) 数据失败或为空: %v\n", sym, err)
			continue
		}
		MA60M5 := CalculateMA(closesM5, 60)
		EMA25M5 := CalculateEMA(closesM5, 25)
		if len(EMA25M5) == 0 {
			// 错误注释：EMA 计算失败（可能因数据不足），跳过以避免 panic
			fmt.Printf("计算 %s (5m) EMA25 失败: 空数组\n", sym)
			continue
		}
		EMA25M5NOW := EMA25M5[len(EMA25M5)-1]
		DIFUP := IsDIFUP(closesM5, 6, 13, 5)
		MACDM5 = "RANGE"
		if price > EMA25M5NOW && price > MA60M5 && DIFUP {
			MACDM5 = "BUYMACD"
		}
		if XSTRONG(closesM5, 6, 13, 5) && price > MA60M5 && DIFUP {
			MACDM5 = "XBUY"
		}
		mid = false
		if price > MA60M5 && DIFUP {
			mid = true
		}

		// 处理买入信号逻辑
		if handleOperation(sym, token, mid, MACDM1, MACDM5, MACDM15, wait_sucess_token, chatID) {
			changed = true
		}

		// 检查是否超时（3小时）
		if now.Sub(token.AddedAt) > 3*time.Hour {
			// 错误注释：超时删除代币，未通知用户，可能需添加 Telegram 通知
			delete(waitList, sym)
			changed = true
		}
	}

	if changed {
		// 错误注释：Telegram 发送失败依赖其内置重试机制
		sendWaitListBroadcast(now, waiting_token, chatID)
	}
}

func waitUntilNext1Min() time.Duration {
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	return time.Until(next)
}

func WaitEnerge(resultsChan chan types.TokenItem, db *sql.DB, wait_sucess_token, chatID string, waiting_token string, config *types.Config) {
	go func() {
		// 先消费一次已有消息，保证 waitList 不为空
		drainResults(resultsChan)

		// 启动时立即执行一次
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
	for {
		select {
		case coin := <-resultsChan:
			addToWaitList(coin, "", "")
		default:
			return
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
	if newAdded && waiting_token != "" {
		sendWaitListBroadcast(now, waiting_token, chatID)
	}
	waitMu.Unlock()
}

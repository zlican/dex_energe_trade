package utils

import (
	"database/sql"
	"fmt"
	"log"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
	"os"
	"strings"
	"sync"
	"time"
)

var minuteMonitorOnce sync.Once
var cfg *types.Config

type waitToken struct {
	Symbol              string
	TokenItem           types.TokenItem
	AddedAt             time.Time
	LastPushedOperation string // 记录最后一次推送的操作
	LastInvalidPushed   bool   // 是否已经推送过失效消息
}

// New: minuteMonitorToken for 1-minute monitoring
type minuteMonitorToken struct {
	Token   waitToken
	AddedAt time.Time
}

var waitMu sync.Mutex
var waitList = make(map[string]waitToken)
var minuteMonitorMu sync.Mutex
var minuteMonitorList = make(map[string]minuteMonitorToken)
var progressLogger = log.New(os.Stdout, "[Screener] ", log.LstdFlags)

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
} // New: sendMinuteMonitorBroadcast for 1-minute monitoring signals
func sendMinuteMonitorBroadcast(token waitToken, wait_sucess_token, chatID string) error {

	msg := fmt.Sprintf("%s%s\n📬 `%s`", token.TokenItem.Emoje, token.Symbol, token.TokenItem.Address)
	// 错误注释：Telegram 发送失败依赖其内置重试机制，失败后跳过状态更新
	if err := telegram.SendMarkdownMessage(wait_sucess_token, chatID, msg); err != nil {
		fmt.Printf("发送 Telegram 买入消息失败 (%s): %v\n", token.Symbol, err)
		return err
	}
	t := waitList[token.Symbol]
	t.LastInvalidPushed = false
	waitList[token.Symbol] = t
	return nil
}

// handleOperation 处理买入信号逻辑
// 返回值：bool 表示是否从 waitList 删除代币
func handleOperation(sym string, token waitToken, MACDM5, MACDM15, wait_sucess_token, chatID string) bool {
	// 条件 1：信号有效，发送买入信号
	if MACDM15 == "BUYMACD" && MACDM5 == "BUYMACD" {
		// Add to 1-minute monitoring pipeline
		minuteMonitorMu.Lock()
		if _, exists := minuteMonitorList[sym]; !exists {
			minuteMonitorList[sym] = minuteMonitorToken{
				Token:   token,
				AddedAt: time.Now(),
			}
		}
		minuteMonitorMu.Unlock()
		return false
	}
	// 条件 2：15分钟信号失效，从 waitList 删除
	if MACDM15 != "BUYMACD" {
		t := waitList[sym]
		if !t.LastInvalidPushed {
			msg := fmt.Sprintf("⚠️信号失效：%s", sym)
			// 错误注释：Telegram 发送失败依赖其内置重试机制，失败后仍删除代币以避免重复处理
			if err := telegram.SendMessage(wait_sucess_token, chatID, msg); err != nil {
				fmt.Printf("发送 Telegram 失效消息失败 (%s): %v\n", sym, err)
			} else {
				t.LastInvalidPushed = true
				waitList[sym] = t
			}
		}
		minuteMonitorMu.Lock()
		delete(minuteMonitorList, sym)
		minuteMonitorMu.Unlock()
		delete(waitList, sym)
		return true
	}

	// 条件 3：其他情况，发送失效消息并清空推送状态
	t := waitList[sym]
	if !t.LastInvalidPushed {
		msg := fmt.Sprintf("⚠️信号失效：%s", sym)
		// 错误注释：Telegram 发送失败依赖其内置重试机制，失败后仍更新状态以避免重复发送
		if err := telegram.SendMessage(wait_sucess_token, chatID, msg); err != nil {
			fmt.Printf("发送 Telegram 失效消息失败 (%s): %v\n", sym, err)
		}
		t.LastInvalidPushed = true
	}
	waitList[sym] = t
	return false
}

// New: executeMinuteMonitorCheck for 1-minute monitoring
func executeMinuteMonitorCheck(wait_sucess_token, chatID string, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			progressLogger.Printf("[executeMinuteMonitorCheck] Panic recovered: %v\n", r)
		}
	}()

	// small delay if needed (保持你原来的 10s 也可以)
	time.Sleep(10 * time.Second)

	// Copy list quickly under lock
	minuteMonitorMu.Lock()
	monitorCopy := make(map[string]minuteMonitorToken, len(minuteMonitorList))
	for k, v := range minuteMonitorList {
		monitorCopy[k] = v
	}
	minuteMonitorMu.Unlock()

	// collect changes
	toRemove := make([]string, 0)
	// messages to send (sym -> msg)
	msgsToSend := make([]struct{ token waitToken }, 0)

	for sym, token := range monitorCopy {
		// --- 获取 1m 数据（无锁） ---
		// 获取 1 分钟 K 线数据
		optionsM1 := map[string]string{
			"aggregate":               cfg.OneAggregate,
			"limit":                   "200",
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM1, err := GetClosesByAPI(token.Token.TokenItem, cfg, optionsM1)
		if err != nil || len(closesM1) < 2 {
			// 错误注释：API 获取失败或数据不足，跳过以避免 panic
			fmt.Printf("获取 %s (1m) 数据失败或不足: %v\n", sym, err)
			continue
		}
		price1 := closesM1[len(closesM1)-1]
		ma60M1 := CalculateMA(closesM1, 60)
		XSTRONGUPM1 := XSTRONGUP(closesM1, 6, 13, 5)
		XSTRONGDOWNM1 := XSTRONGDOWN(closesM1, 6, 13, 5)

		validX := "XBUY"
		validMACD := "BUYMACD"

		MACDM1 := ""
		if price1 > ma60M1 && XSTRONGUPM1 {
			MACDM1 = "XBUY"
		} else if price1 < ma60M1 && XSTRONGDOWNM1 {
			MACDM1 = "XSELL"
		}

		// --- 获取 15m 数据（无锁） ---
		optionsM15 := map[string]string{
			"aggregate":               cfg.FifteenAggregate,
			"limit":                   "200",
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM15, err := GetClosesByAPI(token.Token.TokenItem, cfg, optionsM15)
		if err != nil || len(closesM15) == 0 {
			// 错误注释：API 获取失败或数据为空，跳过以避免 panic
			fmt.Printf("获取 %s (15m) 数据失败或为空: %v\n", sym, err)
			continue
		}
		price := closesM15[len(closesM15)-1]
		isGolden := IsGolden(closesM15, 6, 13, 5)
		_, ema25M15now := CalculateEMA(closesM15, 25)
		DIFM15UP := IsDIFUP(closesM15, 6, 13, 5)
		MACDM15 := "RANGE"
		if price > ema25M15now && isGolden && DIFM15UP {
			MACDM15 = "BUYMACD"
		}

		// 触发
		if MACDM1 == validX && MACDM15 == validMACD {
			msgsToSend = append(msgsToSend, struct{ token waitToken }{token.Token})
			toRemove = append(toRemove, sym) //发送一次就删除了
		}

		if MACDM15 != validMACD {
			toRemove = append(toRemove, sym)
			progressLogger.Printf("Removed %s from 1-minute monitoring due to trend end\n", sym)
			continue
		}

		// timeout
		if now.Sub(token.AddedAt) > 1*time.Hour {
			toRemove = append(toRemove, sym)
			progressLogger.Printf("Removed %s from 1-minute monitoring due to timeout\n", sym)
			continue
		}
	}

	// APPLY  removals under lock
	if len(toRemove) > 0 {
		minuteMonitorMu.Lock()
		for _, sym := range toRemove {
			delete(minuteMonitorList, sym)
		}
		minuteMonitorMu.Unlock()
		progressLogger.Printf("1-minute monitor list updated, %d coins remaining\n", len(minuteMonitorList))
	}

	// SEND messages (outside lock)
	for _, m := range msgsToSend {
		if err := sendMinuteMonitorBroadcast(m.token, wait_sucess_token, chatID); err != nil {
			progressLogger.Printf("发送 1分钟消息失败: %s %v\n", m.token.Symbol, err)
		}
	}
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
		var MACDM5, MACDM15 string

		// 获取 15 分钟 K 线数据
		optionsM15 := map[string]string{
			"aggregate":               config.FifteenAggregate,
			"limit":                   "200",
			"token":                   "base",
			"currency":                "usd",
			"include_empty_intervals": "true",
		}
		closesM15, err := GetClosesByAPI(token.TokenItem, config, optionsM15)
		if err != nil || len(closesM15) < 2 {
			// 错误注释：API 获取失败或数据不足，跳过以避免 panic
			fmt.Printf("获取 %s (1m) 数据失败或不足: %v\n", sym, err)
			continue
		}
		price := closesM15[len(closesM15)-1]
		isGolden := IsGolden(closesM15, 6, 13, 5)
		ema25M15, ema25M15now := CalculateEMA(closesM15, 25)
		DIFM15UP := IsDIFUP(closesM15, 6, 13, 5)
		if len(ema25M15) == 0 {
			progressLogger.Printf("计算 %s (15m) EMA25 失败: 空数组\n", sym)
			continue
		}
		MACDM15 = "RANGE"
		if price > ema25M15now && isGolden && DIFM15UP {
			MACDM15 = "BUYMACD"
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
		ma60M5 := CalculateMA(closesM5, 60)
		_, ema25M5now := CalculateEMA(closesM5, 25)
		MACDSmallUP := IsSmallTFUP(closesM5, 6, 13, 5)
		MACDM5 = "RANGE"
		if price > ema25M5now && price > ma60M5 && MACDSmallUP {
			MACDM5 = "BUYMACD"
		}

		// 处理买入信号逻辑
		if handleOperation(sym, token, MACDM5, MACDM15, wait_sucess_token, chatID) {
			changed = true
		}

		// 检查是否超时（8小时）
		if now.Sub(token.AddedAt) > 8*time.Hour {
			// 错误注释：超时删除代币，未通知用户，可能需添加 Telegram 通知
			delete(waitList, sym)
			minuteMonitorMu.Lock()
			delete(minuteMonitorList, sym)
			minuteMonitorMu.Unlock()
			changed = true
		}
	}

	if changed {
		// 错误注释：Telegram 发送失败依赖其内置重试机制
		sendWaitListBroadcast(now, waiting_token, chatID)
	}
}

func waitUntilNext5Min() time.Duration {
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Duration(5-now.Minute()%5) * time.Minute)
	if next.Before(now) || next.Equal(now) {
		next = next.Add(5 * time.Minute)
	}
	return time.Until(next)
}

func WaitEnerge(resultsChan chan types.TokenItem, db *sql.DB, wait_sucess_token, chatID string, waiting_token string, config *types.Config) {
	cfg = config
	go func() {
		// 先消费一次已有消息，保证 waitList 不为空
		drainResults(resultsChan)

		// 启动时立即执行一次
		now := time.Now()
		executeWaitCheck(db, wait_sucess_token, chatID, waiting_token, config, now)

		// 等到下一个 5 分钟整点
		time.Sleep(waitUntilNext5Min())

		// 每 5 分钟触发
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for now := range ticker.C {
			go executeWaitCheck(db, wait_sucess_token, chatID, waiting_token, config, now)
		}
	}()
	go startMinuteMonitorLoop(wait_sucess_token, chatID)
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
			Symbol:            coin.Symbol,
			TokenItem:         coin,
			AddedAt:           now,
			LastInvalidPushed: true,
		}
		newAdded = true
	}
	if newAdded && waiting_token != "" {
		sendWaitListBroadcast(now, waiting_token, chatID)
	}
	waitMu.Unlock()
}
func startMinuteMonitorLoop(wait_sucess_token, chatID string) {
	minuteMonitorOnce.Do(func() {
		go func() {
			// 对齐到下一个整分钟
			time.Sleep(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)))
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for now := range ticker.C {
				// 每分钟并发执行一次检查（执行过程中不会持锁）
				go executeMinuteMonitorCheck(wait_sucess_token, chatID, now)
			}
		}()
	})
}

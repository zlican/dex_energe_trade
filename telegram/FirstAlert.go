package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	alertBotToken = "7573473925:AAE1IbVhFTgOmhvgV61IkD25Qr9kkbgBgQo"
	alertChatID   = "6074996357"
)

// sendRawMessage 与 SendMessage 类似，但**不会**把发送记录加入 savedMessages，避免循环调用
func sendRawMessage(botToken, chatID, text string) error {
	proxy := "http://127.0.0.1:10809"
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		return fmt.Errorf("解析代理地址失败: %w", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 10 * time.Second,
	}
	url := fmt.Sprintf("%s%s/sendMessage", telegramAPIURL, botToken)

	message := Message{ChatID: chatID, Text: text}
	jsonMessage, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	backoff := 1 * time.Second
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonMessage))
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if err != nil {
			lastErr = fmt.Errorf("发送失败 (尝试 %d): %w", attempt, err)
		} else {
			lastErr = fmt.Errorf("非 200 返回 (尝试 %d): %s", attempt, resp.Status)
			resp.Body.Close()
		}
		log.Printf("[Telegram raw] ❌ %v，等待 %v 后重试", lastErr, backoff)
		time.Sleep(backoff)
		backoff *= 2
	}
	return fmt.Errorf("最终发送失败: %w", lastErr)
}

// ------- 分析逻辑实现 -------

var (
	// 形式一：冒号后面跟符号，如 "消息: BTC"
	colonRegex = regexp.MustCompile(`[:：]\s*([A-Za-z0-9]+)`)
	// 形式二：可能带 ⚠️🟢 前缀、可能有 $ 前缀，如 "🟢 $SOL" 或 "DOGE"
	prefixRegex = regexp.MustCompile(`^[⚠️🟢]*\s*\$?([A-Za-z0-9]+)`)
)

const customLayout = "2006-01-02 15:04:05"

// ExtractSymbol 从消息文本中提取符号
func ExtractSymbol(text string) (string, bool) {
	if m := colonRegex.FindStringSubmatch(text); len(m) >= 2 {
		return strings.ToUpper(m[1]), true
	}
	if m := prefixRegex.FindStringSubmatch(text); len(m) >= 2 {
		return strings.ToUpper(m[1]), true
	}
	return "", false
}

// AnalyzeNewMessage 对刚刚添加的消息进行“首次警报”判定：
// 规则：
//  1. 如果该交易对在历史 savedMessages 中从未出现过 -> 首次警报
//  2. 否则如果该交易对上一次出现时间距本条消息 >= 30 分钟 -> 首次警报
//
// 如果判定为首次警报，会调用配置好的 alert bot 发送一条通知（不会把该通知再写入 savedMessages）
func AnalyzeNewMessage(msg SavedMessage) {
	symbol, ok := ExtractSymbol(msg.Text)
	if !ok {
		// 无法提取交易对，忽略
		return
	}

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// 复制一份 messages（在短锁内完成拷贝），避免在分析时持有锁或读到并发修改的不稳定状态
	savedMessages.RLock()
	copied := make([]SavedMessage, len(savedMessages.messages))
	copy(copied, savedMessages.messages)
	savedMessages.RUnlock()

	// 寻找除最后一条（刚加入的那条）之外的最近一次该交易对出现时间
	var lastPrev *SavedMessage
	if len(copied) >= 2 {
		for i := len(copied) - 2; i >= 0; i-- {
			if s, ok := ExtractSymbol(copied[i].Text); ok && s == symbol {
				lastPrev = &copied[i]
				break
			}
		}
	}

	isFirst := false
	reason := ""
	if lastPrev == nil {
		isFirst = true
		reason = "never_seen_before"
	} else {
		// 如果上一次出现距离当前消息 >= 30 分钟，则也视为首次警报
		if msg.Timestamp.Sub(lastPrev.Timestamp) >= 30*time.Minute {
			isFirst = true
			reason = "no_same_within_30m"
		}
	}

	if !isFirst {
		// 不是首次警报，结束
		return
	}

	// 配置检查
	if alertBotToken == "" || alertChatID == "" {
		log.Printf("[Alert] 首次警报 (%s) 被判定, 但 alert bot 未配置，消息: %s", symbol, msg.Text)
		return
	}

	// 构造告警文本（简洁）
	alertText := fmt.Sprintf("🔔 <短线>\n消息: %s\n时间: %s\n原因: %s", msg.Text, msg.Timestamp.Format(customLayout), reason)

	if err := sendRawMessage(alertBotToken, alertChatID, alertText); err != nil {
		log.Printf("[Alert] 发送首次警报失败: %v", err)
	} else {
		// 保存到 API，category 可选 "短线" 或 "中线"
		saveAlertToAPI("短线", msg.Text, reason)
	}
}

// 把 alert 数据保存到 /alert/add API
func saveAlertToAPI(category, text, reason string) {
	url := "http://127.0.0.1:9001/alert/add"
	body := map[string]string{
		"category": category, // 短线 / 中线
		"text":     text,
		"reason":   reason,
	}

	data, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("[Alert] 保存到API失败: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Alert] 保存到API失败, 状态码: %d", resp.StatusCode)
	}
}

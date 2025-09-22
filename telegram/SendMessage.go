package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const telegramAPIURL = "https://api.telegram.org/bot"

type Message struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// SavedMessage 代表保存的已发送消息（含时间）
type SavedMessage struct {
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

var (
	savedMessages = struct {
		sync.RWMutex
		messages []SavedMessage
		maxSize  int
	}{
		messages: make([]SavedMessage, 0, 100),
		maxSize:  100,
	}
)

// SendMessage 发送普通文本 Telegram 消息，包含指数退避重试
func SendMessage(botToken, chatID, text string) error {
	proxy := "http://127.0.0.1:10809"
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		// 错误注释：代理地址解析失败，通常由于配置错误
		return fmt.Errorf("解析代理地址失败: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second, // 每条请求超时 10 秒
	}
	url := fmt.Sprintf("%s%s/sendMessage", telegramAPIURL, botToken)

	message := Message{
		ChatID: chatID,
		Text:   text,
	}

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		// 错误注释：JSON 序列化失败，通常由于消息结构不合法
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	const maxRetries = 3
	const baseDelay = 1 * time.Second // 初始延迟 100ms
	const maxDelay = 10 * time.Second // 最大延迟 1000ms
	const jitterFactor = 0.1          // 抖动因子 ±10%

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonMessage))
		if err != nil {
			// 错误注释：网络请求失败（例如网络中断或超时）
			lastErr = fmt.Errorf("failed to send message (attempt %d/%d): %w", attempt, maxRetries, err)
		} else {
			// 检查 HTTP 状态码
			if resp.StatusCode != http.StatusOK {
				// 错误注释：非 200 状态码，通常由于 Telegram API 限流或参数错误
				lastErr = fmt.Errorf("received non-200 response (attempt %d/%d): %s", attempt, maxRetries, resp.Status)
			} else {
				// 成功发送消息，保存并返回
				AddMessage(SavedMessage{
					Text:      text,
					Timestamp: time.Now(),
				})
				resp.Body.Close()
				return nil
			}
			resp.Body.Close()
		}

		// 计算指数退避延迟
		// 延迟 = baseDelay * 2^(attempt-1) + 随机抖动
		delay := baseDelay * (1 << (attempt - 1))
		if delay > maxDelay {
			delay = maxDelay
		}
		// 添加 ±10% 抖动
		jitter := float64(delay) * jitterFactor * (rand.Float64() - 0.5) * 2
		totalDelay := time.Duration(float64(delay) + jitter)

		// 错误注释：等待指数退避延迟后重试，避免频繁请求导致限流
		fmt.Printf("Telegram 发送失败 (%s)，将在 %v 后重试 (尝试 %d/%d): %v\n", text, totalDelay, attempt, maxRetries, err)
		time.Sleep(totalDelay)
	}

	// 错误注释：重试耗尽后返回最后一次错误，需检查 Telegram API 状态或网络
	return fmt.Errorf("多次发送失败: %w", lastErr)
}

type MarkdownMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// SendMarkdownMessage 发送 Markdown 格式的 Telegram 消息，包含指数退避重试
func SendMarkdownMessage(botToken, chatID, text string) error {
	proxy := "http://127.0.0.1:10809"
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		// 错误注释：代理地址解析失败，通常由于配置错误
		return fmt.Errorf("解析代理地址失败: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second, // 每条请求超时 10 秒
	}
	url := fmt.Sprintf("%s%s/sendMessage", telegramAPIURL, botToken)

	message := MarkdownMessage{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "Markdown",
	}

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		// 错误注释：JSON 序列化失败，通常由于消息结构不合法
		return fmt.Errorf("failed to marshal markdown message: %w", err)
	}

	const maxRetries = 3
	const baseDelay = 1 * time.Second // 初始延迟 100ms
	const maxDelay = 10 * time.Second // 最大延迟 1000ms
	const jitterFactor = 0.1          // 抖动因子 ±10%

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonMessage))
		if err != nil {
			// 错误注释：网络请求失败（例如网络中断或超时）
			lastErr = fmt.Errorf("failed to send markdown message (attempt %d/%d): %w", attempt, maxRetries, err)
		} else {
			// 检查 HTTP 状态码
			if resp.StatusCode != http.StatusOK {
				// 错误注释：非 200 状态码，通常由于 Telegram API 限流或参数错误
				lastErr = fmt.Errorf("received non-200 response (attempt %d/%d): %s", attempt, maxRetries, resp.Status)
			} else {
				// 成功发送消息，保存并返回
				AddMessage(SavedMessage{
					Text:      text,
					Timestamp: time.Now(),
				})
				resp.Body.Close()
				return nil
			}
			resp.Body.Close()
		}

		// 计算指数退避延迟
		// 延迟 = baseDelay * 2^(attempt-1) + 随机抖动
		delay := baseDelay * (1 << (attempt - 1))
		if delay > maxDelay {
			delay = maxDelay
		}
		// 添加 ±10% 抖动
		jitter := float64(delay) * jitterFactor * (rand.Float64() - 0.5) * 2
		totalDelay := time.Duration(float64(delay) + jitter)

		// 错误注释：等待指数退避延迟后重试，避免频繁请求导致限流
		fmt.Printf("Telegram 发送失败 (%s)，将在 %v 后重试 (尝试 %d/%d)\n", text, totalDelay, attempt, maxRetries)
		time.Sleep(totalDelay)
	}

	// 错误注释：重试耗尽后返回最后一次错误，需检查 Telegram API 状态或网络
	return fmt.Errorf("多次发送失败: %w", lastErr)
}

// AddMessage 添加一条消息，超出maxSize自动删除最早的
func AddMessage(msg SavedMessage) {
	savedMessages.Lock()
	defer savedMessages.Unlock()

	if len(savedMessages.messages) >= savedMessages.maxSize {
		// 删除最早的一条，保持长度不变
		savedMessages.messages = savedMessages.messages[1:]
	}
	savedMessages.messages = append(savedMessages.messages, msg)
	go AnalyzeNewMessage(msg)
}

// GetLatestMessages 返回最新n条，倒序
func GetLatestMessages(n int) []SavedMessage {
	savedMessages.RLock()
	defer savedMessages.RUnlock()

	total := len(savedMessages.messages)
	if total == 0 {
		return nil
	}

	if n > total {
		n = total
	}

	res := make([]SavedMessage, n)
	for i := 0; i < n; i++ {
		res[i] = savedMessages.messages[total-1-i]
	}
	return res
}

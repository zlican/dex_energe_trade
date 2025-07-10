package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"rsi/geckoterminal"
	"rsi/telegram"
	"rsi/utils"
	"strings"
	"sync"
	"syscall"
	"time"
)

// TokenConfig 代币配置
type TokenConfig struct {
	Network      string `json:"network"`
	TokenAddress string `json:"token_address"`
	Timeframe    string `json:"timeframe"`
	Aggregate    string `json:"aggregate"`
	Description  string `json:"description"`
	PoolAddress  string // 将在初始化时填充
}

// Config 程序配置
type Config struct {
	DataDir   string        `json:"data_dir"`
	Interval  int           `json:"interval"`
	Proxy     string        `json:"proxy"`
	RSIPeriod int           `json:"rsi_period"`
	Tokens    []TokenConfig `json:"tokens"`
	BotToken  string        `json:"botToken"`
	ChatID    string        `json:"chatId"`
}

// TokenData 代币数据
type TokenData struct {
	Config      TokenConfig
	LatestData  []geckoterminal.OHLCV // 保存最新数据
	RSIData     []map[string]interface{}
	LastUpdated time.Time
	FilePath    string
	Mutex       sync.Mutex
}

// 全局变量
var (
	wg     sync.WaitGroup
	config *Config
)

func main() {

	// 命令行参数
	configFilePtr := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	// 读取配置文件
	var err error
	config, err = loadConfig(*configFilePtr)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	// 确保数据目录存在
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		fmt.Printf("创建数据目录失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化代币数据
	tokenDataMap := make(map[string]*TokenData)
	for _, token := range config.Tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 获取交易池地址
			var poolAddress string
			var err error
			for retries := 0; retries < 3; retries++ {
				poolAddress, err = utils.GetPoolAddress(token.Network, token.TokenAddress, config.Proxy)
				if err == nil {
					break
				}
				time.Sleep(2 * time.Second)
			}
			if err != nil {
				fmt.Printf("获取交易池地址失败，已达到最大重试次数: %v\n", err)
				return
			}

			tokenConfig := token
			tokenConfig.PoolAddress = poolAddress

			// 创建固定的文件路径 - 使用代币地址的最后部分作为文件名
			var tokenName string
			if token.Description != "" {
				// 使用描述作为文件名的一部分
				tokenName = sanitizeFileName(token.Description)
			} else {
				// 使用地址的最后部分
				tokenName = token.TokenAddress
				if len(tokenName) > 8 {
					tokenName = tokenName[len(tokenName)-8:]
				}
			}

			tokenID := fmt.Sprintf("%s_%s", token.Network, tokenName)
			filePath := filepath.Join(config.DataDir, fmt.Sprintf("%s_%s.csv", tokenID, token.Timeframe))

			tokenDataMap[tokenID] = &TokenData{
				Config:   tokenConfig,
				FilePath: filePath,
			}

			fmt.Printf("添加代币: %s (%s) - 交易池: %s\n",
				token.Description, token.TokenAddress, poolAddress)
		}()
	}
	wg.Wait()

	if len(tokenDataMap) == 0 {
		fmt.Println("没有找到有效的代币配置，程序退出")
		os.Exit(1)
	}

	// 设置定时器
	ticker := time.NewTicker(time.Duration(config.Interval) * time.Second)
	defer ticker.Stop()

	// 创建一个结束信号通道
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("RSI监控程序已启动，监控 %d 个代币，每 %d 秒更新一次，按Ctrl+C退出...\n",
		len(tokenDataMap), config.Interval)

	// 立即执行一次更新
	for id, data := range tokenDataMap {
		updateTokenData(id, data, config.Proxy, config.RSIPeriod)
	}

	// 循环执行
	for {
		select {
		case <-done:
			fmt.Println("接收到退出信号，程序退出...")
			return
		case <-ticker.C:
			// 对每个代币更新数据
			for id, data := range tokenDataMap {
				go updateTokenData(id, data, config.Proxy, config.RSIPeriod)
			}
		}
	}
}

// loadConfig 从文件加载配置
func loadConfig(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %v", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	if config.DataDir == "" {
		config.DataDir = "data"
	}
	if config.Interval <= 0 {
		config.Interval = 60
	}
	if config.RSIPeriod <= 0 {
		config.RSIPeriod = 14
	}

	return &config, nil
}

// sanitizeFileName 清理文件名
func sanitizeFileName(name string) string {
	// 替换不允许在文件名中使用的字符
	invalid := []rune{'<', '>', ':', '"', '/', '\\', '|', '?', '*'}
	for _, r := range invalid {
		name = strings.ReplaceAll(name, string(r), "_")
	}
	return name
}

// updateTokenData 更新代币数据
func updateTokenData(id string, data *TokenData, proxyURL string, rsiPeriod int) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()

	tokenConfig := data.Config

	// 构建查询参数
	options := map[string]string{
		"aggregate":               tokenConfig.Aggregate,
		"limit":                   "20", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}

	// 1.获取OHLCV数据
	var ohlcvData []geckoterminal.OHLCV
	var meta *geckoterminal.MetaData
	var err error

	// 循环尝试获取数据，直到成功或达到最大重试次数
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ohlcvData, meta, err = geckoterminal.GetOHLCV(tokenConfig.Network, tokenConfig.PoolAddress, tokenConfig.Timeframe, options, proxyURL)
		if err == nil {
			// 获取成功，退出循环
			break
		}
		time.Sleep(2 * time.Second) // 等待2秒后重试
	}

	// 如果最终仍然失败
	if err != nil {
		fmt.Printf("[%s] 多次尝试后获取OHLCV数据失败: %v\n", id, err)
		return
	}

	if len(ohlcvData) == 0 {
		fmt.Printf("[%s] 未找到OHLCV数据\n", id)
		return
	}

	// 显示代币信息
	if meta != nil && data.LastUpdated.IsZero() {
		fmt.Printf("[%s] 代币信息: %s (%s) / %s (%s)\n",
			id,
			meta.Base.Name, meta.Base.Symbol,
			meta.Quote.Name, meta.Quote.Symbol)
	}

	// 2. 去除未来数据（第一个通常是未完成的未来 K 线）
	if len(ohlcvData) > 0 {
		ohlcvData = ohlcvData[1:]
	}

	// 首次运行或太久没更新，重新获取更多历史数据
	var newData []geckoterminal.OHLCV
	if len(data.LatestData) == 0 || time.Since(data.LastUpdated) > 10*time.Minute {
		// 获取更多历史数据以计算准确的RSI
		options["limit"] = "300" // 获取足够多的历史数据
		ohlcvData, _, err = geckoterminal.GetOHLCV(tokenConfig.Network, tokenConfig.PoolAddress, tokenConfig.Timeframe, options, proxyURL)
		if err != nil {
			fmt.Printf("[%s] 获取历史OHLCV数据失败: %v\n", id, err)
			return
		}

		// 2. 去除未来数据（最后一个通常是未完成的未来 K 线）
		if len(ohlcvData) > 0 {
			ohlcvData = ohlcvData[1:]
		}

		// 全部数据都是新数据
		newData = ohlcvData
		data.LatestData = ohlcvData
	} else {
		// 合并新数据与现有数据
		// 检查最新数据是否已经包含在现有数据中
		existingTimestamps := make(map[int64]bool)
		for _, candle := range data.LatestData {
			existingTimestamps[candle.Timestamp] = true
		}

		// 找出新数据
		for i := 0; i < len(ohlcvData); i++ {
			candle := ohlcvData[i]
			if existingTimestamps[candle.Timestamp] {
				newData = ohlcvData[:i]
				break
			}
		}
		// 合并并保留最多300条
		data.LatestData = append(newData, data.LatestData...)
		if len(data.LatestData) > 300 {
			data.LatestData = data.LatestData[:300]
		}

		// 使用合并后的数据计算RSI
		ohlcvData = data.LatestData
	}

	// 计算RSI
	data.RSIData = geckoterminal.CalculateRSI(ohlcvData, rsiPeriod)

	// 更新时间
	data.LastUpdated = time.Now()

	// 创建RSI值的映射，以时间戳为键
	rsiMap := make(map[int64]float64)
	for _, rsi := range data.RSIData {
		timestamp := rsi["timestamp"].(int64)
		rsiMap[timestamp] = rsi["value"].(float64)
	}

	// 添加CSV标题行，只保留所需的列：时间、收盘价、交易量和RSI
	var csvData strings.Builder
	csvData.WriteString(fmt.Sprintf("时间,收盘价,交易量,RSI(%d)\n", rsiPeriod))
	for _, candle := range data.LatestData {
		timeStr := geckoterminal.FormatTimestamp(candle.Timestamp)
		rsiVal := ""
		if val, ok := rsiMap[candle.Timestamp]; ok {
			rsiVal = fmt.Sprintf("%.2f", val)
		}
		csvData.WriteString(fmt.Sprintf("%s,%.8f,%.8f,%s\n",
			timeStr, candle.Close, candle.Volume, rsiVal))
	}

	err = os.WriteFile(data.FilePath, []byte(csvData.String()), 0644)
	if err != nil {
		fmt.Printf("[%s] 写入文件失败: %v\n", id, err)
	}

	// 获取最新的RSI值
	if len(data.RSIData) > 0 {
		latestRSI := data.RSIData[len(data.RSIData)-1]
		if latestRSI["value"].(float64) < 30 {
			message := fmt.Sprintf("🚀🚀[%s] 最新RSI(%d)值: %.2f (时间: %s)🚀🚀",
				id, rsiPeriod,
				latestRSI["value"].(float64),
				geckoterminal.FormatTimestamp(latestRSI["timestamp"].(int64)))
			telegram.SendMessage(config.BotToken, config.ChatID, message)
		}
	}
}

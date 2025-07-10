package geckoterminal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// OHLCVResponse 是GeckoTerminal API返回的OHLCV数据结构
type OHLCVResponse struct {
	Data OHLCVData `json:"data"`
	Meta MetaData  `json:"meta"`
}

type OHLCVData struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Attributes Attributes `json:"attributes"`
}

type Attributes struct {
	OHLCVList [][]float64 `json:"ohlcv_list"`
}

type MetaData struct {
	Base  TokenInfo `json:"base"`
	Quote TokenInfo `json:"quote"`
}

type TokenInfo struct {
	Address         string `json:"address"`
	Name            string `json:"name"`
	Symbol          string `json:"symbol"`
	CoingeckoCoinID string `json:"coingecko_coin_id"`
}

// OHLCV 结构体表示一个K线数据
type OHLCV struct {
	Timestamp int64   // 时间戳（秒）
	Open      float64 // 开盘价
	High      float64 // 最高价
	Low       float64 // 最低价
	Close     float64 // 收盘价
	Volume    float64 // 交易量
}

// GetOHLCV 获取代币的K线数据
func GetOHLCV(network, poolAddress, timeframe string, options map[string]string, proxyURL string) ([]OHLCV, *MetaData, error) {
	// 构建请求URL
	baseURL := fmt.Sprintf("https://api.geckoterminal.com/api/v2/networks/%s/pools/%s/ohlcv/%s",
		network, poolAddress, timeframe)

	// 添加查询参数
	params := url.Values{}
	for key, value := range options {
		params.Add(key, value)
	}

	requestURL := baseURL
	if len(params) > 0 {
		requestURL += "?" + params.Encode()
	}

	fmt.Println(requestURL)
	// 创建HTTP客户端
	var client *http.Client

	if proxyURL != "" {
		// 使用代理
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return nil, nil, fmt.Errorf("解析代理URL失败: %v", err)
		}

		client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxy),
			},
			Timeout: 10 * time.Second,
		}
	} else {
		// 不使用代理
		client = &http.Client{
			Timeout: 10 * time.Second,
		}
	}

	// 创建HTTP请求
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 添加请求头
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "GeckoTerminalClient/1.0")

	// 发送HTTP请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("HTTP请求返回非成功状态码: %d", resp.StatusCode)
	}

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 解析JSON响应
	var response OHLCVResponse
	if err := json.Unmarshal(body, &response); err != nil {
		// 打印原始响应以便调试
		fmt.Printf("原始响应: %s\n", string(body))
		return nil, nil, fmt.Errorf("解析JSON失败: %v", err)
	}

	// 转换为OHLCV结构体
	var ohlcvData []OHLCV
	for _, item := range response.Data.Attributes.OHLCVList {
		if len(item) >= 6 {
			timestamp := int64(item[0])
			ohlcv := OHLCV{
				Timestamp: timestamp,
				Open:      item[1],
				High:      item[2],
				Low:       item[3],
				Close:     item[4],
				Volume:    item[5],
			}
			ohlcvData = append(ohlcvData, ohlcv)
		}
	}

	return ohlcvData, &response.Meta, nil
}

// FormatTimestamp 将时间戳格式化为可读的时间字符串
func FormatTimestamp(timestamp int64) string {
	t := time.Unix(timestamp, 0)
	return t.Format("2006-01-02 15:04:05")
}

// ParseTimestamp 将时间字符串解析为时间戳
func ParseTimestamp(timeStr string) (int64, error) {
	layout := "2006-01-02 15:04:05"
	t, err := time.Parse(layout, timeStr)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// GetSupportedNetworks 返回GeckoTerminal支持的网络列表
func GetSupportedNetworks() map[string]string {
	return map[string]string{
		"ethereum":   "eth",
		"binance":    "bsc",
		"polygon":    "polygon",
		"arbitrum":   "arbitrum",
		"avalanche":  "avalanche",
		"optimism":   "optimism",
		"fantom":     "ftm",
		"base":       "base",
		"solana":     "solana",
		"linea":      "linea",
		"blast":      "blast",
		"zksync":     "zksync_era",
		"mode":       "mode",
		"mantle":     "mantle",
		"celo":       "celo",
		"kava":       "kava",
		"metis":      "metis",
		"cronos":     "cronos",
		"gnosis":     "gnosis",
		"pulsechain": "pulsechain",
	}
}

// GetTimeframes 返回GeckoTerminal支持的时间周期
func GetTimeframes() []string {
	return []string{"minute", "hour", "day"}
}
func CalculateRSI(ohlcvData []OHLCV, period int) []map[string]interface{} {
	ohlcvData = reverseOHLCV(ohlcvData)

	if len(ohlcvData) <= period {
		return nil
	}

	var rsiValues []map[string]interface{}

	// 先计算初始涨跌列表
	var gains, losses []float64
	for i := 1; i <= period; i++ {
		change := ohlcvData[i].Close - ohlcvData[i-1].Close
		if change > 0 {
			gains = append(gains, change)
			losses = append(losses, 0)
		} else {
			gains = append(gains, 0)
			losses = append(losses, -change)
		}
	}

	// 初始平均 gain/loss
	avgGain := average(gains)
	avgLoss := average(losses)

	// 计算第一个 RSI
	var rs, rsi float64
	if avgLoss == 0 {
		rsi = 100
	} else {
		rs = avgGain / avgLoss
		rsi = 100 - (100 / (1 + rs))
	}

	rsiValues = append(rsiValues, map[string]interface{}{
		"timestamp": ohlcvData[period].Timestamp,
		"time":      strconv.FormatInt(ohlcvData[period].Timestamp, 10),
		"price":     ohlcvData[period].Close,
		"value":     rsi,
	})

	// 继续计算后面的 RSI 值
	for i := period + 1; i < len(ohlcvData); i++ {
		change := ohlcvData[i].Close - ohlcvData[i-1].Close
		var gain, loss float64
		if change > 0 {
			gain = change
			loss = 0
		} else {
			gain = 0
			loss = -change
		}

		avgGain = (avgGain*(float64(period)-1) + gain) / float64(period)
		avgLoss = (avgLoss*(float64(period)-1) + loss) / float64(period)

		if avgLoss == 0 {
			rsi = 100
		} else {
			rs = avgGain / avgLoss
			rsi = 100 - (100 / (1 + rs))
		}

		rsiValues = append(rsiValues, map[string]interface{}{
			"timestamp": ohlcvData[i].Timestamp,
			"time":      strconv.FormatInt(ohlcvData[i].Timestamp, 10),
			"price":     ohlcvData[i].Close,
			"value":     rsi,
		})
	}

	return rsiValues
}

// average returns the average of a slice of float64
func average(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}
func reverseOHLCV(data []OHLCV) []OHLCV {
	reversed := make([]OHLCV, len(data))
	for i := range data {
		reversed[i] = data[len(data)-1-i]
	}
	return reversed
}

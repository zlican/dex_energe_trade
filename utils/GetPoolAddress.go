package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PoolsResponse 是GeckoTerminal API返回的池数据结构
type PoolsResponse struct {
	Data []PoolData `json:"data"`
}

type PoolData struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Attributes Attributes `json:"attributes"`
}

type Attributes struct {
	Address            string            `json:"address"`
	Name               string            `json:"name"`
	ReserveInUSD       string            `json:"reserve_in_usd"`
	VolumeUSD          map[string]string `json:"volume_usd"`
	TokenPriceUSD      string            `json:"token_price_usd"`
	BaseTokenPriceUSD  string            `json:"base_token_price_usd"`
	QuoteTokenPriceUSD string            `json:"quote_token_price_usd"`
}

// GetPoolAddress 获取代币的最大交易池地址
func GetPoolAddress(network, tokenAddress, proxyURL string) (string, error) {
	// 构建请求URL
	baseURL := fmt.Sprintf("https://api.geckoterminal.com/api/v2/networks/%s/tokens/%s/pools?page=1&sort=h24_volume_usd_desc",
		network, tokenAddress)

	// 创建HTTP客户端
	var client *http.Client

	if proxyURL != "" {
		// 使用代理
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return "", fmt.Errorf("解析代理URL失败: %v", err)
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
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 添加请求头
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "GeckoTerminalClient/1.0")

	// 发送HTTP请求
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP请求返回非成功状态码: %d", resp.StatusCode)
	}

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	// 解析JSON响应
	var response PoolsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		// 打印原始响应以便调试
		fmt.Printf("原始响应: %s\n", string(body))
		return "", fmt.Errorf("解析JSON失败: %v", err)
	}

	// 检查是否有数据
	if len(response.Data) == 0 {
		return "", fmt.Errorf("未找到交易池数据")
	}

	// 获取第一个（交易量最大的）交易池的ID和地址
	poolID := response.Data[0].ID
	poolAddress := response.Data[0].Attributes.Address

	// 从ID中提取实际的地址（如果ID格式是 "network_address"）
	if strings.Contains(poolID, "_") {
		parts := strings.Split(poolID, "_")
		if len(parts) > 1 {
			poolID = parts[1]
		}
	}

	// 如果attributes中的address为空，使用ID
	if poolAddress == "" {
		poolAddress = poolID
	}

	return poolAddress, nil
}

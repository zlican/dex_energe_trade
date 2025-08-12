package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

// 内存缓存与并发去重结构
var (
	poolAddressCache = struct {
		sync.RWMutex
		m map[string]string
	}{m: make(map[string]string)}

	poolInflight = struct {
		sync.Mutex
		m map[string]chan struct{}
	}{m: make(map[string]chan struct{})}
)

// GetPoolAddress 获取代币的最大交易池地址
func GetPoolAddress(network, tokenAddress, proxyURL string) (string, error) {
	if tokenAddress == "" {
		return "", fmt.Errorf("tokenAddress is empty")
	}

	key := network + "|" + tokenAddress

	// 1) 读缓存
	poolAddressCache.RLock()
	if v, ok := poolAddressCache.m[key]; ok && v != "" {
		poolAddressCache.RUnlock()
		return v, nil
	}
	poolAddressCache.RUnlock()

	// 2) 并发去重
	poolInflight.Lock()
	if ch, ok := poolInflight.m[key]; ok {
		// 已有同键请求在进行，等待其完成
		poolInflight.Unlock()
		<-ch
		// 完成后再次从缓存读取
		poolAddressCache.RLock()
		v, ok := poolAddressCache.m[key]
		poolAddressCache.RUnlock()
		if ok && v != "" {
			return v, nil
		}
		return "", fmt.Errorf("并发请求结束但未命中缓存: %s", key)
	}
	// 本次请求成为leader
	ch := make(chan struct{})
	poolInflight.m[key] = ch
	poolInflight.Unlock()

	// 3) 远程获取
	addr, err := fetchPoolAddressRemote(network, tokenAddress, proxyURL)

	// 4) 写缓存并广播完成
	if err == nil && addr != "" {
		poolAddressCache.Lock()
		poolAddressCache.m[key] = addr
		poolAddressCache.Unlock()
	}
	poolInflight.Lock()
	close(ch)
	delete(poolInflight.m, key)
	poolInflight.Unlock()

	if err != nil {
		return "", err
	}
	return addr, nil
}

// 实际远程请求逻辑
func fetchPoolAddressRemote(network, tokenAddress, proxyURL string) (string, error) {
	// 构建请求URL
	baseURL := fmt.Sprintf("https://api.geckoterminal.com/api/v2/networks/%s/tokens/%s/pools?page=1&sort=h24_volume_usd_desc",
		network, tokenAddress)

	// 创建HTTP客户端
	var client *http.Client
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return "", fmt.Errorf("解析代理URL失败: %v", err)
		}
		client = &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(proxy)},
			Timeout:   10 * time.Second,
		}
	} else {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %v", err)
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "GeckoTerminalClient/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP请求返回非成功状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	var response PoolsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		fmt.Printf("原始响应: %s\n", string(body))
		return "", fmt.Errorf("解析JSON失败: %v", err)
	}
	if len(response.Data) == 0 {
		return "", fmt.Errorf("未找到交易池数据")
	}

	poolID := response.Data[0].ID
	poolAddress := response.Data[0].Attributes.Address
	if strings.Contains(poolID, "_") {
		parts := strings.Split(poolID, "_")
		if len(parts) > 1 {
			poolID = parts[1]
		}
	}
	if poolAddress == "" {
		poolAddress = poolID
	}
	return poolAddress, nil
}

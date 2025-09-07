package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"onchain-energe-SRSI/types"
	"time"
)

// AxiomToken 表示 Axiom API 返回的每个 token
type AxiomToken struct {
	PairAddress      string  `json:"pairAddress"`
	TokenAddress     string  `json:"tokenAddress"`
	TokenName        string  `json:"tokenName"`
	TokenTicker      string  `json:"tokenTicker"`
	TransactionCount int     `json:"transactionCount"`
	VolumeSol        float64 `json:"volumeSol"`
	MarketCapSol     float64 `json:"marketCapSol"`
	BuyCount         int     `json:"buyCount"`
	LiquiditySol     float64 `json:"liquiditySol"`
	LiquidityToken   float64 `json:"liquidityToken"`
	NumHolders       int     `json:"numHolders"`
	Top10Holders     float64 `json:"top10Holders"`
	Website          string  `json:"website"`
	Twitter          string  `json:"twitter"`
	Telegram         string  `json:"telegram"`
	Discord          string  `json:"discord"`
}

// FetchRankData 从 Axiom API 拉取数据并转换为 TokenItem
func FetchRankData(fetchURL string, proxy string) ([]*types.TokenItem, error) {
	if fetchURL == "" {
		return nil, fmt.Errorf("url is empty")
	}

	// 构建请求
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("User-Agent", GetRandomUserAgent())
	req.Header.Set("Accept", "application/json, text/plain, */*")

	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", "https://axiom.trade/")
	req.Header.Set("Origin", "https://axiom.trade")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cookie", `auth-refresh-token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyZWZyZXNoVG9rZW5JZCI6IjY3MmFiNmVjLTM2OWEtNDRmZi1hMDIzLTNmYzY2ZDNlYjA0OSIsImlhdCI6MTc1NjYwNDQyN30.sOBelCw8hB59gXIElPW8MEUB3HFRq28uj5wQjuGQaAQ; auth-access-token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdXRoZW50aWNhdGVkVXNlcklkIjoiMTA1ZTY4NGItODNmMi00ZGEyLTg0NTEtYjU3MzMxNjFhOTc2IiwiaWF0IjoxNzU2NjA0NDI3LCJleHAiOjE3NTY2MDUzODd9.8WwbpMCPKr7ncJhjTy-O6LMaM9mUthnlFiaSbOm8f94`)

	// 客户端（支持代理）
	client := &http.Client{Timeout: 10 * time.Second}
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("解析代理地址失败: %v", err)
		}
		client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码错误: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 解析 JSON 数组
	var axiomTokens []AxiomToken
	if err := json.Unmarshal(body, &axiomTokens); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %v", err)
	}

	// 转换成内部结构
	var tokenList []*types.TokenItem
	for _, at := range axiomTokens {
		// 市值 > 1000 SOL, 持币人数 > 1000, SOL流动性 > 250, TOP10 < 25, BUYCOUNT > 10
		if at.MarketCapSol < 1000 || at.NumHolders < 1000 || at.LiquiditySol < 250 || at.Top10Holders > 25 || at.BuyCount < 10 {
			continue
		}

		item := &types.TokenItem{
			Chain:           "solana",
			Address:         at.TokenAddress,
			Symbol:          at.TokenTicker,
			Price:           0, // Axiom 没有直接返回 USDT 价格，这里留 0
			Volume:          at.VolumeSol,
			Liquidity:       at.LiquiditySol,
			MarketCap:       at.MarketCapSol,
			HolderCount:     at.NumHolders,
			Top10Hoders:     at.Top10Holders,
			Buys:            at.BuyCount,
			Sells:           0,
			PoolAddress:     at.PairAddress,
			Website:         at.Website,
			TwitterUsername: at.Twitter,
		}
		tokenList = append(tokenList, item)
	}

	return tokenList, nil
}

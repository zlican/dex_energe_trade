package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"onchain-energe-SRSI/types"
	"sync"
	"time"
)

// RankResponse 表示完整的响应结构
type RankResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Rank []types.TokenItem `json:"rank"`
	} `json:"data"`
}

func FetchRankData(Fetchurl string, proxy string) ([]*types.TokenItem, error) {
	if Fetchurl == "" {
		return nil, fmt.Errorf("url is empty")
	}

	// 创建 HTTP 请求
	req, err := http.NewRequest("GET", Fetchurl, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 添加浏览器常见请求头，伪装成正常访问
	req.Header.Set("User-Agent", GetRandomUserAgent())
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", "https://gmgn.ai/")
	req.Header.Set("Origin", "https://gmgn.ai")
	req.Header.Set("Connection", "keep-alive")

	// 使用带超时的客户端
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		return nil, fmt.Errorf("解析代理地址失败: %v", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
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
		return nil, fmt.Errorf("读取响应体失败: %v", err)
	}

	var result RankResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %v", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API 返回错误: %s", result.Msg)
	}

	var tokenList []*types.TokenItem
	for _, item := range result.Data.Rank {
		if item.SmartDegenCount < 3 && item.RenownedCount < 3 {
			continue
		}
		item.Chain = "solana"
		tokenList = append(tokenList, &item)
	}

	//初步拿到代币之后，获取地址一并保存
	ch := make(chan interface{})
	tokenList = FetchPoolAddress(tokenList, proxy, ch)
	<-ch
	return tokenList, nil
}

func FetchPoolAddress(rankList []*types.TokenItem, proxy string, ch chan<- interface{}) []*types.TokenItem {
	wg := sync.WaitGroup{}
	for _, token := range rankList {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 获取交易池地址
			var poolAddress string
			var err error
			for retries := 0; retries < 3; retries++ {
				poolAddress, err = GetPoolAddress(token.Chain, token.Address, proxy)
				if err == nil {
					break
				}
				time.Sleep(2 * time.Second)
			}
			if err != nil {
				fmt.Printf("获取交易池地址失败，已达到最大重试次数: %v\n", err)
				return
			}

			token.PoolAddress = poolAddress
			/* fmt.Printf("添加代币: %s (%s) - 交易池: %s\n",
			token.Symbol, token.Address, poolAddress) */
		}()
	}
	wg.Wait()
	close(ch)
	return rankList
}

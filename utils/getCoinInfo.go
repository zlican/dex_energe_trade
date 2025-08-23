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
	req.Header.Set("Cookie", `GMGN_LOCALE=zh-CN; GMGN_THEME=dark; GMGN_CHAIN=sol; __cf_bm=2fZWEehEkb16k57bergBUIFkdSmYDt1Nrjos0Q86mUM-1752998680-1.0.1.1-Mb94_MpiihfvQqi5xtgHYoRqOvZpRRGVl5iDicBIN.v_c1cDFD3MS.f3RyRDRdO0sx7vdxp8ESiR4687B0N5FehoCl20Y1VFBFMrSdYNZrQ; sid=gmgn%7Cc2f113e7ee67f4b9342e7b8b206283c2; cf_clearance=.puB7RDod2jcP1dQdlYiMMVUhDUEoVjUEWJuqWbN2Hk-1752999160-1.2.1.1-QNnN28rkP2i1L37JGP97Cs5SxH9ArFibZFhUya5LkqY2Ushd9Ag5L60ln9fsVZWFobUfLCcVlCeTw44AwjlAy2PZc5_X6Ink1WJNMyT8pSmBYD4LtCzDUS7mDGwVG7OyGqL3N2R6RXjwFISsY1uri.7Ujpc4bIX_719zeYTfNu1rjb66vH0chm.PcKIr1Ey3Os8MzLnCP1Lca2mJNvN0TziTzwK4vpCTrXaqJQZyZtg`)

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
	seen := make(map[string]struct{}) // 去重: 按链+地址
	for _, item := range result.Data.Rank {
		if item.SmartDegenCount < 4 && item.RenownedCount < 4 {
			continue
		}
		item.Chain = "solana"
		key := item.Chain + "|" + item.Address
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
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
	sem := make(chan struct{}, 8) // 限制并发，避免瞬时打满API
	for _, token := range rankList {
		// 捕获循环变量
		t := token
		wg.Add(1)
		go func(tok *types.TokenItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// 获取交易池地址
			var poolAddress string
			var err error
			for retries := 0; retries < 3; retries++ {
				poolAddress, err = GetPoolAddress(tok.Chain, tok.Address, proxy)
				if err == nil {
					break
				}
				time.Sleep(2 * time.Second)
			}
			if err != nil {
				fmt.Printf("获取交易池地址失败，已达到最大重试次数: %v\n", err)
				return
			}

			tok.PoolAddress = poolAddress
			/* fmt.Printf("添加代币: %s (%s) - 交易池: %s\n",
			   tok.Symbol, tok.Address, poolAddress) */
		}(t)
	}
	wg.Wait()
	close(ch)
	return rankList
}

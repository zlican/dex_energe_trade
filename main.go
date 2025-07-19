package main

import (
	"flag"
	"fmt"
	"onchain-energe-SRSI/geckoterminal"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/types"
	"onchain-energe-SRSI/utils"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	wg             sync.WaitGroup
	config         *types.Config
	tokenDataMap   = make(map[string]*types.TokenData)
	tokenDataMutex sync.Mutex // 用于保护 tokenDataMap
)

func main() {
	model.InitDB()
	configFilePtr := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	var err error
	config, err = utils.LoadConfig(*configFilePtr)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	// 初始启动一次
	runScan()

	// 启动调度器，每5分钟执行一次 runScan
	go func() {
		for {
			now := time.Now()
			next := now.Truncate(time.Minute).Add(time.Duration(5-now.Minute()%5) * time.Minute)
			time.Sleep(time.Until(next))
			runScan()
		}
	}()

	fmt.Println("系统运行中，按 Ctrl+C 退出...")
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	fmt.Println("程序已退出")
}
func runScan() {
	fmt.Println("开始执行 runScan...")

	var (
		tokenList []*types.TokenItem
		err       error
		maxRetry  = 5
	)

	for i := 0; i < maxRetry; i++ {
		tokenList, err = utils.FetchRankData(config.Url, config.Proxy)
		if err == nil {
			break
		}

		// 判断是否是 403 错误
		if strings.Contains(err.Error(), "403") {
			fmt.Printf("第 %d 次尝试获取失败 (403)，重试中...\n", i+1)
			time.Sleep(2 * time.Second)
			continue
		} else {
			// 其他错误直接退出
			fmt.Printf("获取列表失败: %v\n", err)
			return
		}
	}

	if err != nil {
		fmt.Printf("多次尝试后仍获取失败: %v\n", err)
		return
	}
	var newTokens []string
	tokenDataMutex.Lock()
	for _, token := range tokenList {
		if _, exists := tokenDataMap[token.Symbol]; !exists {
			tokenDataMap[token.Symbol] = &types.TokenData{
				Symbol:      token.Symbol,
				TokenItem:   *token,
				LastUpdated: time.Time{},
				Data:        []geckoterminal.OHLCV{},
				Mutex:       sync.Mutex{},
			}
			newTokens = append(newTokens, token.Symbol)
		}
	}
	tokenDataMutex.Unlock()

	// 并发处理新 token 数据拉取 + EMA计算
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // 限制最大并发数
	for _, sym := range newTokens {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			data := tokenDataMap[symbol]
			utils.Update5minEMA25ToDB(model.DB, symbol, data, config)
			utils.Update15minEMA25ToDB(model.DB, symbol, data, config)
			utils.UpdateTokenData(data, config)
			fmt.Printf("监控并更新: %s\n", symbol)
		}(sym)
	}
	wg.Wait()
}

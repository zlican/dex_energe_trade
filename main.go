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

	go func() {
		// ✅ 首次立即执行
		fmt.Printf("[runScan] 首次立即执行: %s\n", time.Now().Format("15:04:05"))
		runScan()

		// ✅ 计算下一次 minute%5==0 的对齐时间
		now := time.Now()
		minutesToNext := 5 - (now.Minute() % 5)
		if minutesToNext == 0 {
			minutesToNext = 5
		}
		nextAligned := now.Truncate(time.Minute).Add(time.Duration(minutesToNext) * time.Minute)
		delay := time.Until(nextAligned)

		fmt.Printf("[runScan] 下一次对齐执行时间: %s（等待 %v）\n", nextAligned.Format("15:04:05"), delay)

		// ✅ 启动延迟后触发 runScan，然后定期执行
		go func() {
			time.Sleep(delay)

			fmt.Printf("[runScan] 对齐执行: %s\n", time.Now().Format("15:04:05"))
			runScan()

			ticker := time.NewTicker(5 * time.Minute)
			for t := range ticker.C {
				fmt.Printf("[runScan] 周期触发: %s\n", t.Format("15:04:05"))
				runScan()
			}
		}()
	}()

	// ✅ 优雅退出
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
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // 限制最大并发数

	for _, token := range tokenList {
		symbol := token.Symbol

		// 确保 tokenDataMap 里有对应结构（初始化一次）
		tokenDataMutex.Lock()
		if _, exists := tokenDataMap[symbol]; !exists {
			tokenDataMap[symbol] = &types.TokenData{
				Symbol:      symbol,
				TokenItem:   *token,
				LastUpdated: time.Time{},
				Data:        []geckoterminal.OHLCV{},
				Mutex:       sync.Mutex{},
			}
		} else {
			// 如果已存在，也更新 tokenItem（避免旧数据）
			tokenDataMap[symbol].TokenItem = *token
		}
		data := tokenDataMap[symbol]
		tokenDataMutex.Unlock()

		// 启动并发分析
		wg.Add(1)
		go func(symbol string, data *types.TokenData) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			utils.Update5minEMA25ToDB(model.DB, symbol, data, config)
			utils.Update15minEMA25ToDB(model.DB, symbol, data, config)
			utils.AnaylySymbol(data, config)

			fmt.Printf("监控并更新: %s\n", symbol)
		}(symbol, data)
	}

	wg.Wait()
}

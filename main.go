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

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		runScan()
		for {
			now := time.Now()
			next := now.Truncate(time.Minute).Add(time.Duration(15-now.Minute()%15) * time.Minute)
			time.Sleep(time.Until(next))
			go runScan() // 定时触发
		}
	}()

	fmt.Println("系统运行中，按 Ctrl+C 退出...")
	<-done
	fmt.Println("程序已退出")
}

func runScan() {
	fmt.Println("开始执行一次 runScan...")

	//1.请求API获取初步代币的Address
	// 获取最新代币列表
	tokenList, err := utils.FetchRankData(config.Url, config.Proxy)
	if err != nil {
		fmt.Printf("获取列表失败: %v\n", err)
		return
	}

	for _, token := range tokenList {
		tokenDataMutex.Lock()
		_, exists := tokenDataMap[token.Symbol]
		if !exists {
			// 创建新的 TokenData 并存入 map
			tokenDataMap[token.Symbol] = &types.TokenData{
				Symbol:      token.Symbol,
				Data:        []geckoterminal.OHLCV{},
				LastUpdated: time.Time{},
				Mutex:       sync.Mutex{},
				TokenItem:   *token,
			}
			utils.Update15minEMA25ToDB(model.DB, token.Symbol, tokenDataMap[token.Symbol], config)
			go monitorToken(token.Symbol, tokenDataMap[token.Symbol]) // 启动监控
			fmt.Printf("新增监控: %s\n", token.Symbol)
		}
		tokenDataMutex.Unlock()
	}
}
func monitorToken(id string, data *types.TokenData) {
	for {
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		time.Sleep(time.Until(next))

		minute := time.Now().Minute()

		if minute%5 == 0 {
			time.Sleep(5 * time.Second)
			utils.UpdateTokenData(id, data, config)
		}
	}
}

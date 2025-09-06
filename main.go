package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"onchain-energe-SRSI/geckoterminal"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/telegram"
	"onchain-energe-SRSI/types"
	"onchain-energe-SRSI/utils"
	"os"
	"os/signal"
	"strconv"
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

var (
	onchain_waiting_bot_token = "8389283907:AAFLshQSgAaiGKSISTg1N59DAgLp1OLs158"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/latest-tg-messages", latestMessagesHandler)
	mux.HandleFunc("/api/latest-tg-messages-waiting", latestMessagesWaitingHandler)

	go func() {
		if err := http.ListenAndServe(":8889", corsMiddleware(mux)); err != nil {
			log.Fatalf("HTTP服务器启动失败: %v", err)
		}
	}()

	model.InitDB()
	configFilePtr := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	var err error
	config, err = utils.LoadConfig(*configFilePtr)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}
	resultsChan := make(chan types.TokenItem, 100)
	// 启动等待区监控协程
	go utils.WaitEnerge(
		resultsChan,
		model.DB,
		config.BotToken, // 成功触发推送的 bot
		config.ChatID,
		onchain_waiting_bot_token, // 等待区列表推送的 bot
		config,
	)

	go func() {
		// ✅ 首次立即执行
		fmt.Printf("[runScan] 首次立即执行: %s\n", time.Now().Format("15:04:05"))
		runScan(resultsChan)

		// ✅ 计算下一次 minute%5==0 的对齐时间
		now := time.Now()
		minutesToNext := 5 - (now.Minute() % 5)
		if minutesToNext == 0 {
			minutesToNext = 5
		}
		nextAligned := now.Truncate(time.Minute).Add(time.Duration(minutesToNext) * time.Minute)
		delay := time.Until(nextAligned)

		// ✅ 启动延迟后触发 runScan，然后定期执行
		go func() {
			time.Sleep(delay)

			runScan(resultsChan)

			ticker := time.NewTicker(5 * time.Minute)
			for t := range ticker.C {
				fmt.Printf("[runScan] 周期触发: %s\n", t.Format("15:04:05"))
				runScan(resultsChan)
			}
		}()
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	fmt.Println("程序已退出")
}

func runScan(resultsChan chan types.TokenItem) {
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

			MACDM15 := utils.Update15minEMA25ToDB(model.DB, symbol, data, config)
			if !MACDM15 {
				return
			}

			MACDM5 := utils.Update5minEMA25ToDB(model.DB, symbol, data, config)
			if !MACDM5 {
				return
			}
			utils.AnaylySymbol(data, config, resultsChan)

		}(symbol, data)
	}

	wg.Wait()
}

func latestMessagesHandler(w http.ResponseWriter, r *http.Request) {
	// 参数limit，默认25
	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	msgs := telegram.GetLatestMessages(limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}
func latestMessagesWaitingHandler(w http.ResponseWriter, r *http.Request) {
	// 参数limit，默认1
	limit := 2
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	msgs := telegram.GetLatestMessagesWaiting(limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

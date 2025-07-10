package utils

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"onchain-energe-SRSI/geckoterminal"
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/types"
)

func Update15minEMA25ToDB(db *sql.DB, symbol string, data *types.TokenData, config *types.Config) {

	tokenItem := data.TokenItem

	// 构建查询参数
	options := map[string]string{
		"aggregate":               config.FifteenAggregate,
		"limit":                   "200", // 只获取最新的几条数据即可
		"token":                   "base",
		"currency":                "usd",
		"include_empty_intervals": "true",
	}

	// 1.获取OHLCV数据
	var ohlcvData []geckoterminal.OHLCV
	var err error

	// 循环尝试获取数据，直到成功或达到最大重试次数
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ohlcvData, _, err = geckoterminal.GetOHLCV(tokenItem.Chain, tokenItem.PoolAddress, config.Timeframe, options, config.Proxy)
		if err == nil {
			// 获取成功，退出循环
			break
		}
		time.Sleep(2 * time.Second) // 等待2秒后重试
	}

	// 如果最终仍然失败
	if err != nil {
		fmt.Printf("[%s] 多次尝试后获取OHLCV数据失败: %v\n", symbol, err)
		return
	}

	if len(ohlcvData) == 0 {
		fmt.Printf("[%s] 未找到OHLCV数据\n", symbol)
		return
	}

	var closes []float64
	for _, k := range ohlcvData {
		closes = append(closes, k.Close)
	}

	ema25 := CalculateEMA(closes, 25)
	ema50 := CalculateEMA(closes, 50)

	currentPrice := closes[len(closes)-1]
	lastEMA := ema25[len(ema25)-1]
	lastTime := ohlcvData[len(ohlcvData)-1].Timestamp

	// 写入数据库（UPSERT）
	_, err = model.DB.Exec(`
		INSERT INTO symbol_ema_15min (symbol, timestamp, ema25, ema50, price_gt_ema25)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		timestamp = VALUES(timestamp),
		ema25 = VALUES(ema25),
		ema50 = VALUES(ema50),
		price_gt_ema25 = VALUES(price_gt_ema25)
	`, symbol, lastTime, lastEMA, ema50[len(ema50)-1], currentPrice > lastEMA)
	if err != nil {
		log.Printf("写入出错 %s: %v", symbol, err)
	}

}

func GetEMA25FromDB(db *sql.DB, symbol string) float64 {
	var ema25 float64
	err := db.QueryRow("SELECT ema25 FROM symbol_ema_15min WHERE symbol = ?", symbol).Scan(&ema25)
	if err != nil {
		log.Printf("查询 EMA25 失败 %s: %v", symbol, err)
		return 0
	}
	return ema25
}

func GetEMA50FromDB(db *sql.DB, symbol string) float64 {
	var ema50 float64
	err := db.QueryRow("SELECT ema50 FROM symbol_ema_15min WHERE symbol = ?", symbol).Scan(&ema50)
	if err != nil {
		log.Printf("查询 EMA50 失败 %s: %v", symbol, err)
		return 0
	}
	return ema50
}

func GetPriceGT_EMA25FromDB(db *sql.DB, symbol string) bool {
	var priceGT_EMA25 bool
	err := db.QueryRow("SELECT price_gt_ema25 FROM symbol_ema_15min WHERE symbol = ?", symbol).Scan(&priceGT_EMA25)
	if err != nil {
		log.Printf("查询 PriceGT_EMA25 失败 %s: %v", symbol, err)
		return false
	}
	return priceGT_EMA25
}

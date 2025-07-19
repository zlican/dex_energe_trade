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
	if err != nil || len(ohlcvData) == 0 {
		fmt.Printf("[%s] 多次尝试后获取OHLCV数据失败: %v\n", symbol, err)
		return
	} else {
		for i, j := 0, len(ohlcvData)-1; i < j; i, j = i+1, j-1 {
			ohlcvData[i], ohlcvData[j] = ohlcvData[j], ohlcvData[i]
		}
	}

	var closes []float64
	for _, k := range ohlcvData {
		closes = append(closes, k.Close)
	}

	ema25 := CalculateEMA(closes, 25)
	ema50 := CalculateEMA(closes, 50)

	currentPrice := closes[len(closes)-1]
	lastEMA25 := ema25[len(ema25)-1]
	lastEMA50 := ema50[len(ema50)-1]
	lastTime := ohlcvData[len(ohlcvData)-1].Timestamp
	_, kLine, _ := StochRSIFromClose(closes, 14, 14, 3, 3)
	lastKLine := kLine[len(kLine)-1]

	// 写入数据库（UPSERT）
	_, err = model.DB.Exec(`
		INSERT INTO symbol_ema_15min (symbol, timestamp, ema25, ema50, srsi, price_gt_ema25)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		timestamp = VALUES(timestamp),
		ema25 = VALUES(ema25),
		ema50 = VALUES(ema50),
		srsi = VALUES(srsi),
		price_gt_ema25 = VALUES(price_gt_ema25)
	`, symbol, lastTime, lastEMA25, lastEMA50, lastKLine, currentPrice > lastEMA25)
	if err != nil {
		log.Printf("写入出错 %s: %v", symbol, err)
	}

}

func Get15MEMAFromDB(db *sql.DB, symbol string) (ema25, ema50 float64) {
	err := db.QueryRow("SELECT ema25, ema50 FROM symbol_ema_15min WHERE symbol = ?", symbol).Scan(&ema25, &ema50)
	if err != nil {
		log.Printf("查询 15MEMA 失败 %s: %v", symbol, err)
		return 0, 0
	}
	return ema25, ema50
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

func Get15MSRSIFromDB(db *sql.DB, symbol string) (srsi float64) {
	err := db.QueryRow("SELECT srsi FROM symbol_ema_15min WHERE symbol = ?", symbol).Scan(&srsi)
	if err != nil {
		log.Printf("查询 1HSRSIFromDB 失败 %s: %v", symbol, err)
		return 0
	}
	return srsi
}

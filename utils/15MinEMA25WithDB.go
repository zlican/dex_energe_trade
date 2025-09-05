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

func Update15minEMA25ToDB(db *sql.DB, symbol string, data *types.TokenData, config *types.Config) (result bool) {

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
		return false
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
	ma60 := CalculateMA(closes, 60)

	currentPrice := closes[len(closes)-1]
	lastEMA25 := ema25[len(ema25)-1]
	lastEMA50 := ema50[len(ema50)-1]
	lastTime := ohlcvData[len(ohlcvData)-1].Timestamp
	lastKLine := 0.0
	DEAUP := IsDEAUP(closes, 6, 13, 5)

	var status string
	if currentPrice > lastEMA25 && currentPrice > ma60 && DEAUP {
		status = "BUYMACD"
	} else {
		status = "RANGE"
	}

	// 写入数据库（UPSERT）
	_, err = model.DB.Exec(`
		INSERT INTO symbol_ema_15min (symbol, timestamp, ema25, ema50, srsi, status, price_gt_ema25)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		timestamp = VALUES(timestamp),
		ema25 = VALUES(ema25),
		ema50 = VALUES(ema50),
		srsi = VALUES(srsi),
		status = VALUES(status),
		price_gt_ema25 = VALUES(price_gt_ema25)
	`, symbol, lastTime, lastEMA25, lastEMA50, lastKLine, status, currentPrice > lastEMA25)
	if err != nil {
		log.Printf("写入出错 %s: %v", symbol, err)
	}

	if status == "BUYMACD" {
		return true
	}
	return false

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

func Get15MStatusFromDB(db *sql.DB, symbol string) (status string) {
	err := db.QueryRow("SELECT status FROM symbol_ema_15min WHERE symbol = ?", symbol).Scan(&status)
	if err != nil {
		log.Printf("查询 15MStatusFromDB 失败 %s: %v", symbol, err)
		return ""
	}
	return status
}

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
	ema25, _ := CalculateEMA(closes, 25)

	currentPrice := closes[len(closes)-1]
	lastEMA25 := ema25[len(ema25)-1]
	lastTime := ohlcvData[len(ohlcvData)-1].Timestamp
	golden := IsGolden(closes, 6, 13, 5)

	var status string
	if currentPrice > lastEMA25 && golden { //15M: EMA25 + 绿柱
		status = "BUYMACD"
	} else {
		status = "RANGE"
	}

	// 写入数据库（UPSERT）
	_, err = model.DB.Exec(`
		INSERT INTO symbol_ema_15min (symbol, timestamp, status)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
		timestamp = VALUES(timestamp),
		status = VALUES(status)
	`, symbol, lastTime, status)
	if err != nil {
		log.Printf("写入出错 %s: %v", symbol, err)
	}

	return status == "BUYMACD"

}

func Get15MStatusFromDB(db *sql.DB, symbol string) (status string) {
	err := db.QueryRow("SELECT status FROM symbol_ema_15min WHERE symbol = ?", symbol).Scan(&status)
	if err != nil {
		log.Printf("查询 15MStatusFromDB 失败 %s: %v", symbol, err)
		return ""
	}
	return status
}

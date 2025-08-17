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

func Update5minEMA25ToDB(db *sql.DB, symbol string, data *types.TokenData, config *types.Config) bool {

	tokenItem := data.TokenItem

	// 构建查询参数
	options := map[string]string{
		"aggregate":               config.FiveAggregate,
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
	price := closes[len(closes)-2]
	ema25 := CalculateEMA(closes, 25)
	ema50 := CalculateEMA(closes, 50)
	ema169 := CalculateEMA(closes, 169)
	ma60 := CalculateMA(closes, 60)
	UpMACD := IsAboutToGoldenCross(closes, 6, 13, 5)
	XUpMACD := IsGolden(closes, 6, 13, 5)

	lastEMA25 := ema25[len(ema25)-1]
	lastEMA50 := ema50[len(ema50)-1]
	lastEMA169 := ema169[len(ema169)-1]
	lastTime := ohlcvData[len(ohlcvData)-1].Timestamp
	_, kLine, _ := StochRSIFromClose(closes, 14, 14, 3, 3)
	lastKLine := kLine[len(kLine)-1]

	var status string
	if lastEMA25 > ma60 && UpMACD && price > ma60 {
		status = "BUYMACD"
	} else if lastEMA25 < ma60 && XUpMACD && price > lastEMA25 && price > ma60 {
		status = "BUYMACD"
	} else {
		status = "RANGE"
	}

	// 写入数据库（UPSERT）
	_, err = model.DB.Exec(`
		INSERT INTO symbol_ema_5min (symbol, timestamp, ema25, ema50, ema169, srsi, upmacd, xupmacd, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		timestamp = VALUES(timestamp),
		ema25 = VALUES(ema25),
		ema50 = VALUES(ema50),
		ema169 = VALUES(ema169),
		srsi = VALUES(srsi),
		upmacd = VALUES(upmacd),
		xupmacd = VALUES(xupmacd),
		status = VALUES(status)
	`, symbol, lastTime, lastEMA25, lastEMA50, lastEMA169, lastKLine, UpMACD, XUpMACD, status)
	if err != nil {
		log.Printf("写入出错 %s: %v", symbol, err)
	}

	GT := price > ma60
	return GT
}

func Get5MEMAFromDB(db *sql.DB, symbol string) (ema25, ema50, ema169 float64) {
	err := db.QueryRow("SELECT ema25, ema50, ema169 FROM symbol_ema_5min WHERE symbol = ?", symbol).Scan(&ema25, &ema50, &ema169)
	if err != nil {
		log.Printf("查询 5EMA 失败 %s: %v", symbol, err)
		return 0, 0, 0
	}
	return ema25, ema50, ema169
}

func Get5SRSIFromDB(db *sql.DB, symbol string) (srsi float64) {
	err := db.QueryRow("SELECT srsi FROM symbol_ema_5min WHERE symbol = ?", symbol).Scan(&srsi)
	if err != nil {
		log.Printf("查询 5MSRSIFromDB 失败 %s: %v", symbol, err)
		return 0
	}
	return srsi
}

func GetMACDFromDB(db *sql.DB, symbol string) (upmacd, xupmacd bool) {
	err := db.QueryRow("SELECT upmacd, xupmacd FROM symbol_ema_5min WHERE symbol = ?", symbol).Scan(&upmacd, &xupmacd)
	if err != nil {
		log.Printf("查询 5MMACD 失败 %s: %v", symbol, err)
		return false, false
	}
	return upmacd, xupmacd
}

func Get5MStatusFromDB(db *sql.DB, symbol string) (status string) {
	err := db.QueryRow("SELECT status FROM symbol_ema_5min WHERE symbol = ?", symbol).Scan(&status)
	if err != nil {
		log.Printf("查询 5MStatusFromDB 失败 %s: %v", symbol, err)
		return ""
	}
	return status
}

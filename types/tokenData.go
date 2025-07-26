package types

import (
	"onchain-energe-SRSI/geckoterminal"
	"sync"
	"time"
)

// TokenData 代币数据
type TokenData struct {
	Symbol      string
	TokenItem   TokenItem
	Data        []geckoterminal.OHLCV // 保存最新数据
	LastUpdated time.Time
	Mutex       sync.Mutex
}

type CoinIndicator struct {
	Symbol       string
	Price        float64
	TimeInternal string
	StochRSI     float64 // 只存最后一个值够用了
	Operation    string
	Status       string
}

package utils

import (
	"onchain-energe-SRSI/model"
	"onchain-energe-SRSI/types"
)

// AnaylySymbol 更新代币数据并执行交易模型分析
func AnaylySymbol(data *types.TokenData, config *types.Config, resultsChan chan types.TokenItem) {
	data.Mutex.Lock()
	defer data.Mutex.Unlock()

	tokenItem := data.TokenItem

	MACDM15 := Get15MStatusFromDB(model.DB, tokenItem.Symbol)
	MACDM5 := Get5MStatusFromDB(model.DB, tokenItem.Symbol)
	// ===== 模型1（优先级最高） =====
	if MACDM15 == "BUYMACD" && (MACDM5 == "BUYMACD" || MACDM15 == "XBUYMID") {
		tokenItem.Emoje = "🟢"
		resultsChan <- tokenItem
		return
	}
}

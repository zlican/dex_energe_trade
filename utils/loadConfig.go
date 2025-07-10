package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"onchain-energe-SRSI/types"
	"os"
)

// loadConfig 从文件加载配置
func LoadConfig(filePath string) (*types.Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %v", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config types.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	if config.DataDir == "" {
		config.DataDir = "data"
	}
	if config.Interval <= 0 {
		config.Interval = 60
	}
	if config.RSIPeriod <= 0 {
		config.RSIPeriod = 14
	}

	return &config, nil
}

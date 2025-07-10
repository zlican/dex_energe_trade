package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"rsi/telegram"
)

// Config 程序配置
type Config struct {
	DataDir   string `json:"data_dir"`
	Interval  int    `json:"interval"`
	Proxy     string `json:"proxy"`
	RSIPeriod int    `json:"rsi_period"`
	BotToken  string `json:"botToken"`
	ChatID    string `json:"chatId"`
}

func main() {

	// 命令行参数
	configFilePtr := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	// 读取配置文件
	config, err := loadConfig(*configFilePtr)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	botToken := config.BotToken
	chatID := config.ChatID
	message := "你好，这是来自 Go 语言的 Telegram 机器人消息！"

	err = telegram.SendMessage(botToken, chatID, message)
	if err != nil {
		fmt.Printf("发送消息时出错: %v\n", err)
	} else {
		fmt.Println("消息发送成功！")
	}
}

// loadConfig 从文件加载配置
func loadConfig(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %v", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
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

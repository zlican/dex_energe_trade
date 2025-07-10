package main

import (
	"flag"
	"fmt"
	"log"
	"onchain-energe-SRSI/types"
	"onchain-energe-SRSI/utils"
	"os"
)

var config *types.Config

func main() {
	configFilePtr := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	// 读取配置文件
	var err error
	config, err = utils.LoadConfig(*configFilePtr)

	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	rankList, err := utils.FetchRankData(config.Url, config.Proxy)
	if err != nil {
		log.Fatalf("获取排名数据失败: %v", err)
	}

	for _, item := range rankList {
		fmt.Println(item.Symbol, item.Address)
	}

}

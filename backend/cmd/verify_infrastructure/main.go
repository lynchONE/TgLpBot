package main

import (
	"TgLpBot/config"
	"TgLpBot/services"
	"context"
	"fmt"
)

func main() {
	fmt.Println("🔍 开始验证 [PoolM 数据源] ...")
	fmt.Println("==================================================")

	// 1. 加载配置
	if err := config.LoadConfig(); err != nil {
		fmt.Printf("❌ 配置加载失败: %v\n", err)
		return
	}

	// 2. 连接 ClickHouse
	fmt.Println("正在连接 ClickHouse...")
	ch, err := services.NewClickHouseService(
		config.AppConfig.ClickHouseAddr,
		config.AppConfig.ClickHouseDB,
		config.AppConfig.ClickHouseUser,
		config.AppConfig.ClickHousePassword,
		config.AppConfig.ClickHouseDebug,
	)
	if err != nil {
		fmt.Printf("❌ ClickHouse 连接失败: %v\n", err)
		// 如果这里失败，后续步骤无法进行
		return
	}
	fmt.Println("✅ ClickHouse 连接成功")

	// 3. 测试 PoolM 抓取
	fmt.Println("--------------------------------------------------")
	fmt.Println("📡 正在调用 PoolM API (Top 5, 15m)...")

	poolM := services.NewPoolMService(ch)
	err = poolM.FetchAndStore("bsc", "v3", 5, 15)
	if err != nil {
		fmt.Printf("❌ PoolM API 拉取失败: %v\n", err)
	} else {
		fmt.Println("✅ PoolM API 拉取并入库成功!")
	}

	// 4. 读取 ClickHouse 验证
	fmt.Println("--------------------------------------------------")
	fmt.Println("📖 读取数据库验证...")
	var count uint64
	// 简单统计 pools 表行数
	if err := ch.Conn.QueryRow(context.Background(), "SELECT count() FROM pools").Scan(&count); err != nil {
		fmt.Printf("⚠️ 读取失败: %v\n", err)
	} else {
		fmt.Printf("✅ pools 表当前记录数: %d\n", count)
	}

	fmt.Println("==================================================")
	fmt.Println("🎉 验证完成。请检查上述步骤是否全绿。")
}

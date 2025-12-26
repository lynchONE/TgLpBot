package main

import (
	"TgLpBot/config"
	"TgLpBot/services"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

func main() {
	fmt.Println("🧠 正在验证策略引擎 (Data -> Analysis -> Scenario)...")
	fmt.Println("==================================================")

	// 1. Config
	config.LoadConfig()

	// 2. ClickHouse
	ch, err := services.NewClickHouseService(
		config.AppConfig.ClickHouseAddr,
		config.AppConfig.ClickHouseDB,
		config.AppConfig.ClickHouseUser,
		config.AppConfig.ClickHousePassword,
		config.AppConfig.ClickHouseDebug,
	)
	if err != nil {
		log.Fatalf("ClickHouse connect failed: %v", err)
	}
	fmt.Println("✅ 数据库连接成功")

	// 3. Analysis Service & PoolM
	analyzer := services.NewAnalysisService(ch)
	poolM := services.NewPoolMService(ch)

	// 4. Debug: Insert a FAKE pool to verify read/write logic
	fakeAddr := "0xDebug"
	fmt.Println("🛠️ 插入测试数据 (0xDebug, Price=100.0)...")

	fakePool := services.PoolMData{
		Chain: "bsc", ProtocolVersion: "v3", PoolAddress: fakeAddr, TradingPair: "TEST/USDT",
		CurrentTokenPrice: 100.0,
	}
	// Call BatchInsert manually
	if err := poolM.BatchInsert([]services.PoolMData{fakePool}, 60); err != nil {
		fmt.Printf("❌ 插入测试数据失败: %v\n", err)
	}
	// Also need 15m for AnalyzePool (it fetches 15 and 60)
	if err := poolM.BatchInsert([]services.PoolMData{fakePool}, 15); err != nil {
		fmt.Printf("❌ 插入测试数据失败(15m): %v\n", err)
	}

	fmt.Println("⏳ 等待 ClickHouse 数据写入...")
	time.Sleep(2 * time.Second)

	// Check Count
	var count uint64
	err = ch.Conn.QueryRow(context.Background(), "SELECT count() FROM pools WHERE pool_address = ?", fakeAddr).Scan(&count)
	if err != nil {
		fmt.Printf("❌ Count Query Failed: %v\n", err)
	}
	fmt.Printf("🔍 DB Check: Count = %d\n", count)

	// Dump rows
	rows, _ := ch.Conn.Query(context.Background(), "SELECT chain, protocol_version, timeframe, current_token_price FROM pools WHERE pool_address = ?", fakeAddr)
	for rows.Next() {
		var c, p string
		var t uint32
		var pr float64
		rows.Scan(&c, &p, &t, &pr)
		fmt.Printf("📝 Row: Chain='%s' Proto='%s' TF=%d Price=%f\n", c, p, t, pr)
	}
	rows.Close()

	// 5. Analyze the FAKE Pool
	fmt.Printf("🎯 目标池子: %s (%s)\n", "TEST/USDT", fakeAddr)
	res, err := analyzer.AnalyzePool("bsc", "v3", fakeAddr)
	if err != nil {
		fmt.Printf("❌ Analyze Failed: %v\n", err)
	} else {
		// Output
		jsonBytes, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println("📊 分析结果(Fake):")
		fmt.Println(string(jsonBytes))
	}
}

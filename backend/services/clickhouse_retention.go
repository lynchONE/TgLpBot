package services

import (
	"context"
	"fmt"
	"log"
	"time"
)

const clickhouseRetentionHours = 24

func (s *ClickHouseService) StartDailyRetentionCleanup() {
	if s == nil || s.Conn == nil {
		return
	}
	go s.dailyRetentionCleanupLoop()
}

func (s *ClickHouseService) dailyRetentionCleanupLoop() {
	log.Printf("[ClickHouse] 保留策略清理任务已启动：每天 00:00 清理超过 %d 小时的数据", clickhouseRetentionHours)

	for {
		next := nextLocalMidnight(time.Now())
		wait := time.Until(next)
		if wait > 0 {
			time.Sleep(wait)
		}
		s.runRetentionCleanupOnce()
	}
}

func nextLocalMidnight(now time.Time) time.Time {
	loc := now.Location()
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, loc)
	if !now.Before(midnight) {
		midnight = midnight.Add(24 * time.Hour)
	}
	return midnight
}

func (s *ClickHouseService) runRetentionCleanupOnce() {
	if s == nil || s.Conn == nil {
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	type retentionTarget struct {
		table      string
		timeColumn string
	}
	targets := []retentionTarget{
		{table: "poolm_top_fees_raw", timeColumn: "ts"},
		{table: "auto_lp_analysis", timeColumn: "ts"},
		{table: "pools", timeColumn: "updated_at"},
	}

	for _, t := range targets {
		q := fmt.Sprintf("ALTER TABLE %s DELETE WHERE %s < now() - INTERVAL %d HOUR", t.table, t.timeColumn, clickhouseRetentionHours)
		if err := s.Conn.Exec(ctx, q); err != nil {
			log.Printf("[ClickHouse] 清理失败 table=%s err=%v", t.table, err)
			continue
		}
		log.Printf("[ClickHouse] 清理任务已提交(异步 mutation) table=%s", t.table)
	}

	log.Printf("[ClickHouse] 清理触发完成，用时=%s", time.Since(start).String())
}

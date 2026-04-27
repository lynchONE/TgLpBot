package pool_sync

import "strings"

type PoolDataSourceFieldCoverage struct {
	PoolCount             int `json:"pool_count"`
	MissingPoolIDCount    int `json:"missing_pool_id_count"`
	MissingTVLCount       int `json:"missing_tvl_count"`
	MissingActiveUSDCount int `json:"missing_active_liquidity_usd_count"`
	MissingFeeCount       int `json:"missing_fee_count"`
	MissingVolumeCount    int `json:"missing_volume_count"`
	MissingTokenCount     int `json:"missing_token_count"`
	V4PoolIDFallbackCount int `json:"v4_pool_id_fallback_count"`
}

func annotateSnapshotSource(snapshot *PoolMTopFeesResponse, source PoolDataSourceConfig) {
	if snapshot == nil {
		return
	}
	snapshot.PoolDataSourceID = source.ID
	snapshot.PoolDataSourceName = strings.TrimSpace(source.Name)
	snapshot.PoolDataSourceType = NormalizePoolDataSourceType(source.SourceType)
	snapshot.PoolDataSourceURL = strings.TrimSpace(source.BaseURL)
}

func poolDataSourceFieldCoverage(snapshot *PoolMTopFeesResponse) PoolDataSourceFieldCoverage {
	var out PoolDataSourceFieldCoverage
	if snapshot == nil {
		return out
	}
	out.PoolCount = len(snapshot.Data)
	for _, item := range snapshot.Data {
		protocol := normalizePoolMProtocolVersion(item, firstNonEmpty(item.PoolAddress, item.PoolID))
		if strings.TrimSpace(item.PoolAddress) == "" && strings.TrimSpace(item.PoolID) == "" {
			out.MissingPoolIDCount++
		}
		if protocol == "v4" && strings.TrimSpace(item.PoolAddress) == "" && strings.TrimSpace(item.PoolID) != "" {
			out.V4PoolIDFallbackCount++
		}
		if item.CurrentPoolValue <= 0 {
			out.MissingTVLCount++
		}
		if item.ActiveLiquidityUSD <= 0 {
			out.MissingActiveUSDCount++
		}
		if item.TotalFees <= 0 {
			out.MissingFeeCount++
		}
		if item.TotalVolume <= 0 {
			out.MissingVolumeCount++
		}
		if strings.TrimSpace(item.Token0Address) == "" || strings.TrimSpace(item.Token1Address) == "" {
			out.MissingTokenCount++
		}
	}
	return out
}

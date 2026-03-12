package web_server

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

type smartMoneyPoolMarkerPnLEstimate struct {
	HasPnLEstimate bool
	CostBasisUSD   float64
	PnLEstimateUSD float64
}

type smartMoneyPoolMarkerPositionKey struct {
	WalletAddress string
	ContractAddr  string
	TokenID       string
	TickLower     int32
	TickUpper     int32
}

func buildMarkerPositionKey(row smartMoneyPoolMarkerRow) smartMoneyPoolMarkerPositionKey {
	return smartMoneyPoolMarkerPositionKey{
		WalletAddress: strings.ToLower(strings.TrimSpace(row.WalletAddress)),
		ContractAddr:  strings.ToLower(strings.TrimSpace(row.ContractAddr)),
		TokenID:       strings.TrimSpace(row.TokenID),
		TickLower:     row.TickLower,
		TickUpper:     row.TickUpper,
	}
}

func (k smartMoneyPoolMarkerPositionKey) String() string {
	return strings.Join([]string{
		k.WalletAddress,
		k.ContractAddr,
		k.TokenID,
		fmt.Sprintf("%d", k.TickLower),
		fmt.Sprintf("%d", k.TickUpper),
	}, "|")
}

func estimateMarkerRowUSD(row smartMoneyPoolMarkerRow, dec0 int, dec1 int, p0 float64, p1 float64) float64 {
	amt0 := absFloat(amountToFloat(row.Amount0, dec0))
	amt1 := absFloat(amountToFloat(row.Amount1, dec1))
	usd0 := sanitizeFloat(amt0 * p0)
	usd1 := sanitizeFloat(amt1 * p1)
	return sanitizeFloat(usd0 + usd1)
}

func parseLiquidityAbs(raw string) *big.Int {
	v, ok := new(big.Int).SetString(strings.TrimSpace(raw), 10)
	if !ok || v == nil {
		return big.NewInt(0)
	}
	return v.Abs(v)
}

func markerLiquidityShare(part *big.Int, whole *big.Int) float64 {
	if part == nil || whole == nil || part.Sign() <= 0 || whole.Sign() <= 0 {
		return 0
	}
	nf := new(big.Float).SetPrec(256).SetInt(part)
	wf := new(big.Float).SetPrec(256).SetInt(whole)
	share, _ := new(big.Float).Quo(nf, wf).Float64()
	if share < 0 {
		share = -share
	}
	if share > 1 {
		return 1
	}
	return share
}

func querySmartMoneyPoolMarkerHistory(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, poolVersion string, poolID string, rows []smartMoneyPoolMarkerRow, end time.Time) ([]smartMoneyPoolMarkerRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}

	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if poolVersion == "" || poolID == "" {
		return []smartMoneyPoolMarkerRow{}, nil
	}
	if end.IsZero() {
		end = time.Now().UTC()
	}

	keys := make([]smartMoneyPoolMarkerPositionKey, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if strings.ToLower(strings.TrimSpace(row.Action)) != "remove" {
			continue
		}
		key := buildMarkerPositionKey(row)
		keyStr := key.String()
		if key.WalletAddress == "" || key.ContractAddr == "" {
			continue
		}
		if _, ok := seen[keyStr]; ok {
			continue
		}
		seen[keyStr] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return []smartMoneyPoolMarkerRow{}, nil
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	args := make([]any, 0, 4+len(keys)*5)
	args = append(args, poolVersion, poolID, end.UTC())
	chainFilter := ""
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	placeholders := make([]string, 0, len(keys))
	for _, key := range keys {
		placeholders = append(placeholders, "(?, ?, ?, ?, ?)")
		args = append(args, key.WalletAddress, key.ContractAddr, key.TokenID, key.TickLower, key.TickUpper)
	}

	q := fmt.Sprintf(`
		SELECT
			ts,
			event_seq,
			wallet_address,
			action,
			token_id,
			contract_address,
			toString(if(net_amount0 != '' AND net_amount0 != '0', net_amount0, amount0)) AS amount0_value,
			toString(if(net_amount1 != '' AND net_amount1 != '0', net_amount1, amount1)) AS amount1_value,
			liquidity_delta,
			tick_lower,
			tick_upper,
			tx_hash,
			block_number,
			log_index
		FROM smart_lp_events
		WHERE action IN ('add', 'remove')
			AND pool_version = ? AND pool_id = ?
			AND wallet_address != ''
			AND ts <= ?
			%s
			AND (wallet_address, contract_address, token_id, tick_lower, tick_upper) IN (%s)
		ORDER BY block_number ASC, log_index ASC
	`, chainFilter, strings.Join(placeholders, ","))

	historyRows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer historyRows.Close()

	out := make([]smartMoneyPoolMarkerRow, 0, len(rows)*2)
	for historyRows.Next() {
		var row smartMoneyPoolMarkerRow
		if err := historyRows.Scan(
			&row.Ts,
			&row.EventSeq,
			&row.WalletAddress,
			&row.Action,
			&row.TokenID,
			&row.ContractAddr,
			&row.Amount0,
			&row.Amount1,
			&row.LiquidityDelta,
			&row.TickLower,
			&row.TickUpper,
			&row.TxHash,
			&row.BlockNumber,
			&row.LogIndex,
		); err != nil {
			return nil, err
		}
		row.WalletAddress = strings.ToLower(strings.TrimSpace(row.WalletAddress))
		row.Action = strings.ToLower(strings.TrimSpace(row.Action))
		row.TxHash = strings.ToLower(strings.TrimSpace(row.TxHash))
		row.ContractAddr = strings.ToLower(strings.TrimSpace(row.ContractAddr))
		row.TokenID = strings.TrimSpace(row.TokenID)
		out = append(out, row)
	}
	if err := historyRows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func applyMarkerPnLEstimates(targetRows []smartMoneyPoolMarkerRow, historyRows []smartMoneyPoolMarkerRow, dec0 int, dec1 int, p0 float64, p1 float64) map[string]smartMoneyPoolMarkerPnLEstimate {
	out := make(map[string]smartMoneyPoolMarkerPnLEstimate)
	if len(targetRows) == 0 || len(historyRows) == 0 {
		return out
	}

	targets := make(map[string]struct{}, len(targetRows))
	for _, row := range targetRows {
		if strings.ToLower(strings.TrimSpace(row.Action)) != "remove" {
			continue
		}
		targets[buildMarkerEventID(row.TxHash, row.EventSeq, row.LogIndex)] = struct{}{}
	}
	if len(targets) == 0 {
		return out
	}

	type replayState struct {
		Liquidity *big.Int
		CostUSD   float64
	}

	states := make(map[string]*replayState)
	sort.Slice(historyRows, func(i, j int) bool {
		if historyRows[i].BlockNumber == historyRows[j].BlockNumber {
			if historyRows[i].LogIndex == historyRows[j].LogIndex {
				return historyRows[i].EventSeq < historyRows[j].EventSeq
			}
			return historyRows[i].LogIndex < historyRows[j].LogIndex
		}
		return historyRows[i].BlockNumber < historyRows[j].BlockNumber
	})

	for _, row := range historyRows {
		action := strings.ToLower(strings.TrimSpace(row.Action))
		if action != "add" && action != "remove" {
			continue
		}
		key := buildMarkerPositionKey(row).String()
		st := states[key]
		if st == nil {
			st = &replayState{Liquidity: big.NewInt(0)}
			states[key] = st
		}

		liqAbs := parseLiquidityAbs(row.LiquidityDelta)
		eventUSD := estimateMarkerRowUSD(row, dec0, dec1, p0, p1)

		if action == "add" {
			if liqAbs.Sign() <= 0 {
				continue
			}
			st.Liquidity.Add(st.Liquidity, liqAbs)
			st.CostUSD = sanitizeFloat(st.CostUSD + eventUSD)
			continue
		}

		if liqAbs.Sign() <= 0 || st.Liquidity.Sign() <= 0 {
			continue
		}

		share := markerLiquidityShare(liqAbs, st.Liquidity)
		if share <= 0 {
			continue
		}

		costUSD := sanitizeFloat(st.CostUSD * share)
		eventID := buildMarkerEventID(row.TxHash, row.EventSeq, row.LogIndex)
		if _, ok := targets[eventID]; ok {
			out[eventID] = smartMoneyPoolMarkerPnLEstimate{
				HasPnLEstimate: true,
				CostBasisUSD:   costUSD,
				PnLEstimateUSD: sanitizeFloat(eventUSD - costUSD),
			}
		}

		st.CostUSD = sanitizeFloat(st.CostUSD - costUSD)
		if st.CostUSD < 0 {
			st.CostUSD = 0
		}
		st.Liquidity.Sub(st.Liquidity, liqAbs)
		if st.Liquidity.Sign() <= 0 {
			st.Liquidity.SetInt64(0)
			st.CostUSD = 0
		}
	}

	return out
}

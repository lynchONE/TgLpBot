package web_server

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

type smartMoneyWalletPositionHistoryRow struct {
	Ts              time.Time
	EventSeq        uint64
	PoolVersion     string
	PoolID          string
	WalletAddress   string
	Action          string
	TokenID         string
	ContractAddress string
	Amount0         string
	Amount1         string
	LiquidityDelta  string
	TickLower       int32
	TickUpper       int32
	TxHash          string
	BlockNumber     uint64
	LogIndex        uint32
}

type smartMoneyWalletPositionReplayState struct {
	Liquidity *big.Int
	CostUSD   float64
	RunStart  *time.Time
	Complete  bool
}

func buildSmartMoneyWalletPositionTokenKey(poolVersion string, poolID string, contractAddr string, tokenID string, tickLower int, tickUpper int) string {
	pv := strings.ToLower(strings.TrimSpace(poolVersion))
	pid := strings.ToLower(strings.TrimSpace(poolID))
	contract := strings.ToLower(strings.TrimSpace(contractAddr))
	token := strings.TrimSpace(tokenID)
	if pv == "" || pid == "" || token == "" {
		return ""
	}
	switch pv {
	case "v3":
		if contract == "" {
			return ""
		}
		return fmt.Sprintf("v3|%s|%s|%s", pid, contract, token)
	case "v4":
		return fmt.Sprintf("v4|%s|%s|%d|%d", pid, token, tickLower, tickUpper)
	default:
		return fmt.Sprintf("%s|%s|%s", pv, pid, token)
	}
}

func buildSmartMoneyWalletPositionLegacyKey(poolVersion string, poolID string, contractAddr string, tickLower int, tickUpper int) string {
	pv := strings.ToLower(strings.TrimSpace(poolVersion))
	pid := strings.ToLower(strings.TrimSpace(poolID))
	contract := strings.ToLower(strings.TrimSpace(contractAddr))
	if pv == "" || pid == "" {
		return ""
	}
	if pv == "v4" {
		return fmt.Sprintf("v4|%s|legacy|%d|%d", pid, tickLower, tickUpper)
	}
	return fmt.Sprintf("%s|%s|legacy|%s|%d|%d", pv, pid, contract, tickLower, tickUpper)
}

func buildSmartMoneyWalletPositionHistoryAliases(position smartMoneyWalletLPPosition) []string {
	aliases := make([]string, 0, 2)
	poolVersion := strings.ToLower(strings.TrimSpace(position.PoolVersion))
	if tokenKey := buildSmartMoneyWalletPositionTokenKey(
		position.PoolVersion,
		position.PoolID,
		position.ContractAddress,
		position.PositionID,
		position.TickLower,
		position.TickUpper,
	); tokenKey != "" {
		aliases = append(aliases, tokenKey)
		if poolVersion == "v3" {
			return aliases
		}
	}
	if legacyKey := buildSmartMoneyWalletPositionLegacyKey(
		position.PoolVersion,
		position.PoolID,
		position.ContractAddress,
		position.TickLower,
		position.TickUpper,
	); legacyKey != "" {
		if len(aliases) == 0 || aliases[0] != legacyKey {
			aliases = append(aliases, legacyKey)
		}
	}
	return aliases
}

func buildSmartMoneyWalletPositionHistoryRowKey(row smartMoneyWalletPositionHistoryRow) string {
	if tokenKey := buildSmartMoneyWalletPositionTokenKey(
		row.PoolVersion,
		row.PoolID,
		row.ContractAddress,
		row.TokenID,
		int(row.TickLower),
		int(row.TickUpper),
	); tokenKey != "" {
		return tokenKey
	}
	return buildSmartMoneyWalletPositionLegacyKey(
		row.PoolVersion,
		row.PoolID,
		row.ContractAddress,
		int(row.TickLower),
		int(row.TickUpper),
	)
}

func buildSmartMoneyWalletPositionAliasTargets(positions []smartMoneyWalletLPPosition) (map[string]int, []string) {
	aliasTargets := make(map[string]int)
	ambiguous := make(map[string]struct{})

	for i := range positions {
		for _, alias := range buildSmartMoneyWalletPositionHistoryAliases(positions[i]) {
			if alias == "" {
				continue
			}
			if prev, ok := aliasTargets[alias]; ok && prev != i {
				ambiguous[alias] = struct{}{}
				continue
			}
			aliasTargets[alias] = i
		}
	}

	for alias := range ambiguous {
		delete(aliasTargets, alias)
	}

	queryKeys := make([]string, 0, len(aliasTargets))
	for alias := range aliasTargets {
		queryKeys = append(queryKeys, alias)
	}
	sort.Strings(queryKeys)
	return aliasTargets, queryKeys
}

func querySmartMoneyWalletPositionHistory(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, walletAddr string, positions []smartMoneyWalletLPPosition, end time.Time) ([]smartMoneyWalletPositionHistoryRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}

	wallet := strings.ToLower(strings.TrimSpace(walletAddr))
	if wallet == "" || len(positions) == 0 {
		return []smartMoneyWalletPositionHistoryRow{}, nil
	}
	if end.IsZero() {
		end = time.Now().UTC()
	}

	_, queryKeys := buildSmartMoneyWalletPositionAliasTargets(positions)
	if len(queryKeys) == 0 {
		return []smartMoneyWalletPositionHistoryRow{}, nil
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	args := make([]any, 0, 3+len(queryKeys))
	args = append(args, wallet, end.UTC())
	chainFilter := ""
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	placeholders := make([]string, 0, len(queryKeys))
	for _, key := range queryKeys {
		placeholders = append(placeholders, "?")
		args = append(args, key)
	}

	positionKeyExpr := `
		multiIf(
			lowerUTF8(pool_version) = 'v3' AND token_id != '' AND contract_address != '',
			concat('v3|', lowerUTF8(pool_id), '|', lowerUTF8(contract_address), '|', token_id),
			lowerUTF8(pool_version) = 'v4' AND token_id != '',
			concat('v4|', lowerUTF8(pool_id), '|', token_id, '|', toString(tick_lower), '|', toString(tick_upper)),
			lowerUTF8(pool_version) = 'v4',
			concat('v4|', lowerUTF8(pool_id), '|legacy|', toString(tick_lower), '|', toString(tick_upper)),
			concat(lowerUTF8(pool_version), '|', lowerUTF8(pool_id), '|legacy|', lowerUTF8(contract_address), '|', toString(tick_lower), '|', toString(tick_upper))
		)
	`

	q := fmt.Sprintf(`
		SELECT
			ts,
			event_seq,
			pool_version,
			pool_id,
			wallet_address,
			action,
			token_id,
			contract_address,
			amount0_value,
			amount1_value,
			liquidity_delta,
			tick_lower,
			tick_upper,
			tx_hash,
			block_number,
			log_index
		FROM (
			SELECT
				dedup_ts AS ts,
				dedup_event_seq AS event_seq,
				dedup_pool_version AS pool_version,
				dedup_pool_id AS pool_id,
				dedup_wallet_address AS wallet_address,
				dedup_action AS action,
				dedup_token_id AS token_id,
				dedup_contract_address AS contract_address,
				dedup_amount0_value AS amount0_value,
				dedup_amount1_value AS amount1_value,
				dedup_liquidity_delta AS liquidity_delta,
				dedup_tick_lower AS tick_lower,
				dedup_tick_upper AS tick_upper,
				tx_hash,
				dedup_block_number AS block_number,
				log_index
			FROM (
				SELECT
					argMax(ts, event_seq) AS dedup_ts,
					max(event_seq) AS dedup_event_seq,
					argMax(pool_version, event_seq) AS dedup_pool_version,
					argMax(pool_id, event_seq) AS dedup_pool_id,
					argMax(wallet_address, event_seq) AS dedup_wallet_address,
					argMax(action, event_seq) AS dedup_action,
					argMax(token_id, event_seq) AS dedup_token_id,
					argMax(contract_address, event_seq) AS dedup_contract_address,
					argMax(if(net_amount0 != '' AND net_amount0 != '0', net_amount0, amount0), event_seq) AS dedup_amount0_value,
					argMax(if(net_amount1 != '' AND net_amount1 != '0', net_amount1, amount1), event_seq) AS dedup_amount1_value,
					argMax(liquidity_delta, event_seq) AS dedup_liquidity_delta,
					argMax(tick_lower, event_seq) AS dedup_tick_lower,
					argMax(tick_upper, event_seq) AS dedup_tick_upper,
					tx_hash,
					argMax(block_number, event_seq) AS dedup_block_number,
					log_index
				FROM smart_lp_events
				WHERE lowerUTF8(wallet_address) = ?
					AND ts <= ?
					AND action IN ('add', 'remove')
					%s
					AND %s IN (%s)
				GROUP BY tx_hash, log_index
			)
		)
		ORDER BY block_number ASC, log_index ASC, event_seq ASC
	`, chainFilter, positionKeyExpr, strings.Join(placeholders, ","))

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyWalletPositionHistoryRow, 0, len(queryKeys)*4)
	for rows.Next() {
		var row smartMoneyWalletPositionHistoryRow
		if err := rows.Scan(
			&row.Ts,
			&row.EventSeq,
			&row.PoolVersion,
			&row.PoolID,
			&row.WalletAddress,
			&row.Action,
			&row.TokenID,
			&row.ContractAddress,
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
		row.PoolVersion = strings.ToLower(strings.TrimSpace(row.PoolVersion))
		row.PoolID = strings.ToLower(strings.TrimSpace(row.PoolID))
		row.WalletAddress = strings.ToLower(strings.TrimSpace(row.WalletAddress))
		row.Action = strings.ToLower(strings.TrimSpace(row.Action))
		row.TokenID = strings.TrimSpace(row.TokenID)
		row.ContractAddress = strings.ToLower(strings.TrimSpace(row.ContractAddress))
		row.TxHash = strings.ToLower(strings.TrimSpace(row.TxHash))
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func estimateSmartMoneyWalletPositionEventUSD(row smartMoneyWalletPositionHistoryRow, position smartMoneyWalletLPPosition, prices map[string]float64) float64 {
	p0 := prices[strings.ToLower(strings.TrimSpace(position.Token0))]
	p1 := prices[strings.ToLower(strings.TrimSpace(position.Token1))]
	amt0 := absFloat(amountToFloat(row.Amount0, position.Token0Dec))
	amt1 := absFloat(amountToFloat(row.Amount1, position.Token1Dec))
	return sanitizeFloat(amt0*p0 + amt1*p1)
}

func applySmartMoneyWalletPositionPnLEstimates(positions []smartMoneyWalletLPPosition, historyRows []smartMoneyWalletPositionHistoryRow, prices map[string]float64) {
	for i := range positions {
		positions[i].CurrentValueUSD = sanitizeFloat(positions[i].PositionUSD + positions[i].ClaimableFeesUSD)
		positions[i].HasPnL = false
		positions[i].AbsolutePnLUSD = 0
		positions[i].CostBasisUSD = 0
		positions[i].RunningSince = cloneUTCTimePtr(positions[i].RunningSince)
	}

	if len(positions) == 0 || len(historyRows) == 0 {
		return
	}

	aliasTargets, _ := buildSmartMoneyWalletPositionAliasTargets(positions)
	if len(aliasTargets) == 0 {
		return
	}

	sort.Slice(historyRows, func(i, j int) bool {
		if historyRows[i].BlockNumber == historyRows[j].BlockNumber {
			if historyRows[i].LogIndex == historyRows[j].LogIndex {
				return historyRows[i].EventSeq < historyRows[j].EventSeq
			}
			return historyRows[i].LogIndex < historyRows[j].LogIndex
		}
		return historyRows[i].BlockNumber < historyRows[j].BlockNumber
	})

	states := make([]smartMoneyWalletPositionReplayState, len(positions))
	for i := range states {
		states[i].Liquidity = big.NewInt(0)
		states[i].Complete = true
	}

	for _, row := range historyRows {
		idx, ok := aliasTargets[buildSmartMoneyWalletPositionHistoryRowKey(row)]
		if !ok || idx < 0 || idx >= len(positions) {
			continue
		}

		action := strings.ToLower(strings.TrimSpace(row.Action))
		if action != "add" && action != "remove" {
			continue
		}

		st := &states[idx]
		target := positions[idx]
		liqAbs := parseLiquidityAbs(row.LiquidityDelta)
		if liqAbs.Sign() <= 0 {
			continue
		}

		switch action {
		case "add":
			if st.Liquidity.Sign() == 0 {
				start := row.Ts.UTC()
				st.RunStart = &start
			}
			st.Liquidity.Add(st.Liquidity, liqAbs)
			st.CostUSD = sanitizeFloat(st.CostUSD + estimateSmartMoneyWalletPositionEventUSD(row, target, prices))
		case "remove":
			if st.Liquidity.Sign() <= 0 {
				st.Complete = false
				continue
			}
			if liqAbs.Cmp(st.Liquidity) > 0 {
				st.Complete = false
				st.Liquidity.SetInt64(0)
				st.CostUSD = 0
				st.RunStart = nil
				continue
			}
			share := markerLiquidityShare(liqAbs, st.Liquidity)
			if share <= 0 {
				st.Complete = false
				continue
			}
			removedCost := sanitizeFloat(st.CostUSD * share)
			st.CostUSD = sanitizeFloat(st.CostUSD - removedCost)
			if st.CostUSD < 0 {
				st.CostUSD = 0
			}
			st.Liquidity.Sub(st.Liquidity, liqAbs)
			if st.Liquidity.Sign() <= 0 {
				st.Liquidity.SetInt64(0)
				st.CostUSD = 0
				st.RunStart = nil
			}
		}
	}

	for i := range positions {
		currentLiquidity := parseLiquidityAbs(positions[i].Liquidity)
		if currentLiquidity.Sign() <= 0 {
			continue
		}
		st := states[i]
		if st.Liquidity == nil || !st.Complete || st.RunStart == nil {
			continue
		}
		if st.Liquidity.Cmp(currentLiquidity) != 0 {
			continue
		}

		start := st.RunStart.UTC()
		positions[i].RunningSince = &start
		positions[i].CostBasisUSD = sanitizeFloat(st.CostUSD)
		positions[i].AbsolutePnLUSD = sanitizeFloat(positions[i].CurrentValueUSD - positions[i].CostBasisUSD)
		positions[i].HasPnL = true
	}
}

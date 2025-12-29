package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var erc20TransferID = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

type SmartLPMonitor struct {
	ch *ClickHouseService

	stopChan chan struct{}
	ticker   *time.Ticker
}

type smartLPEvent struct {
	ts              time.Time
	eventSeq        uint64
	chain           string
	poolVersion     string
	poolID          string
	walletAddress   string
	action          string
	tokenID         string
	amount0         string
	amount1         string
	liquidityDelta  string
	txHash          string
	blockNumber     uint64
	logIndex        uint32
	contractAddress string
	source          string
}

func NewSmartLPMonitor(ch *ClickHouseService) *SmartLPMonitor {
	interval := 60 * time.Second
	if config.AppConfig != nil && config.AppConfig.SmartLPScanIntervalSeconds > 0 {
		interval = time.Duration(config.AppConfig.SmartLPScanIntervalSeconds) * time.Second
	}
	return &SmartLPMonitor{
		ch:       ch,
		stopChan: make(chan struct{}),
		ticker:   time.NewTicker(interval),
	}
}

func (s *SmartLPMonitor) Start() {
	if config.AppConfig == nil || !config.AppConfig.SmartLPEnabled {
		log.Println("[SmartLP] disabled (SMART_LP_ENABLED=0)")
		return
	}
	go s.runLoop()
}

func (s *SmartLPMonitor) Stop() {
	if s == nil {
		return
	}
	select {
	case <-s.stopChan:
	default:
		close(s.stopChan)
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
}

func (s *SmartLPMonitor) runLoop() {
	log.Println("[SmartLP] monitor started")
	for {
		select {
		case <-s.ticker.C:
			s.runOnce()
		case <-s.stopChan:
			log.Println("[SmartLP] monitor stopped")
			return
		}
	}
}

func (s *SmartLPMonitor) runOnce() {
	if config.AppConfig == nil || !config.AppConfig.SmartLPEnabled {
		return
	}
	if s.ch == nil || s.ch.Conn == nil {
		log.Println("[SmartLP] clickhouse not initialized")
		return
	}
	if blockchain.Client == nil || blockchain.ChainID == nil {
		log.Println("[SmartLP] blockchain client not initialized")
		return
	}
	if !common.IsHexAddress(config.AppConfig.SmartLPContractAddress) {
		log.Println("[SmartLP] SMART_LP_CONTRACT_ADDRESS invalid")
		return
	}

	monitorAddr := common.HexToAddress(config.AppConfig.SmartLPContractAddress)

	v3Managers := make([]common.Address, 0, 2)
	if common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
		v3Managers = append(v3Managers, common.HexToAddress(config.AppConfig.PancakeV3PositionManagerAddress))
	}
	if common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
		v3Managers = append(v3Managers, common.HexToAddress(config.AppConfig.UniswapV3PositionManagerAddress))
	}

	var v4PoolManager common.Address
	hasV4 := common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	if hasV4 {
		v4PoolManager = common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	}

	if len(v3Managers) == 0 && !hasV4 {
		log.Println("[SmartLP] no V3/V4 managers configured")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	head, err := blockchain.Client.BlockNumber(ctx)
	if err != nil {
		log.Printf("[SmartLP] get head block failed: %v", err)
		return
	}

	last, ok, err := s.loadLastScannedBlock(ctx)
	if err != nil {
		log.Printf("[SmartLP] load scan state failed: %v", err)
		return
	}
	if !ok {
		if err := s.saveLastScannedBlock(ctx, head); err != nil {
			log.Printf("[SmartLP] init scan state failed: %v", err)
		} else {
			log.Printf("[SmartLP] scan state initialized at block=%d (no backfill)", head)
		}
		return
	}
	if last >= head {
		log.Printf("[SmartLP] no new blocks to scan (last=%d head=%d)", last, head)
		return
	}

	from := last + 1
	to := head
	log.Printf("[SmartLP] scanning blocks %d-%d (v3_managers=%d v4=%v)", from, to, len(v3Managers), hasV4)

	events := make([]smartLPEvent, 0)
	if len(v3Managers) > 0 {
		v3Events, err := s.scanV3Logs(ctx, from, to, monitorAddr, v3Managers)
		if err != nil {
			log.Printf("[SmartLP] scan V3 logs failed: %v", err)
			return
		}
		log.Printf("[SmartLP] v3 logs scanned: %d events", len(v3Events))
		events = append(events, v3Events...)
	}
	if hasV4 {
		v4Events, err := s.scanV4Logs(ctx, from, to, monitorAddr, v4PoolManager)
		if err != nil {
			log.Printf("[SmartLP] scan V4 logs failed: %v", err)
			return
		}
		log.Printf("[SmartLP] v4 logs scanned: %d events", len(v4Events))
		events = append(events, v4Events...)
	}

	if len(events) > 0 {
		if err := s.insertEvents(ctx, events); err != nil {
			log.Printf("[SmartLP] insert events failed: %v", err)
			return
		}
		log.Printf("[SmartLP] inserted events: %d", len(events))
	} else {
		log.Printf("[SmartLP] no events found in range")
	}

	if err := s.saveLastScannedBlock(ctx, head); err != nil {
		log.Printf("[SmartLP] update scan state failed: %v", err)
		return
	}
	log.Printf("[SmartLP] scan completed (last=%d)", head)
}

func (s *SmartLPMonitor) loadLastScannedBlock(ctx context.Context) (uint64, bool, error) {
	var last uint64
	var cnt uint64
	q := "SELECT argMax(last_block, updated_at) AS last_block, count() AS cnt FROM smart_lp_scan_state WHERE id = 1"
	if err := s.ch.Conn.QueryRow(ctx, q).Scan(&last, &cnt); err != nil {
		return 0, false, err
	}
	if cnt == 0 {
		return 0, false, nil
	}
	return last, true, nil
}

func (s *SmartLPMonitor) saveLastScannedBlock(ctx context.Context, block uint64) error {
	q := "INSERT INTO smart_lp_scan_state (id, last_block, updated_at) VALUES (1, ?, ?)"
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt-1) * 500 * time.Millisecond
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
		if err := s.ch.Conn.Exec(ctx, q, block, time.Now()); err != nil {
			lastErr = err
			if !isRetryableClickHouseError(err) {
				return err
			}
			if config.AppConfig != nil && config.AppConfig.SmartLPDebug {
				log.Printf("[SmartLP] save scan state failed (retrying) attempt=%d err=%v", attempt, err)
			}
			continue
		}
		return nil
	}
	return lastErr
}

func (s *SmartLPMonitor) insertEvents(ctx context.Context, events []smartLPEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := s.ch.Conn.PrepareBatch(ctx, `INSERT INTO smart_lp_events (
		ts, event_seq, chain, pool_version, pool_id, wallet_address, action,
		token_id, amount0, amount1, liquidity_delta, tx_hash, block_number, log_index,
		contract_address, source
	)`)
	if err != nil {
		return err
	}

	for _, ev := range events {
		if err := batch.Append(
			ev.ts,
			ev.eventSeq,
			ev.chain,
			ev.poolVersion,
			ev.poolID,
			ev.walletAddress,
			ev.action,
			ev.tokenID,
			ev.amount0,
			ev.amount1,
			ev.liquidityDelta,
			ev.txHash,
			ev.blockNumber,
			ev.logIndex,
			ev.contractAddress,
			ev.source,
		); err != nil {
			return err
		}
	}

	return batch.Send()
}

func isRetryableClickHouseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "eof"):
		return true
	case strings.Contains(msg, "connection reset"):
		return true
	case strings.Contains(msg, "broken pipe"):
		return true
	case strings.Contains(msg, "tls: use of closed connection"):
		return true
	case strings.Contains(msg, "context deadline exceeded"):
		return true
	case strings.Contains(msg, "context canceled"):
		return true
	default:
		return false
	}
}

func (s *SmartLPMonitor) scanV3Logs(ctx context.Context, from, to uint64, monitorAddr common.Address, managers []common.Address) ([]smartLPEvent, error) {
	increaseID := crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)"))
	decreaseID := crypto.Keccak256Hash([]byte("DecreaseLiquidity(uint256,uint128,uint256,uint256)"))

	uint128Ty, _ := abi.NewType("uint128", "", nil)
	uint256Ty, _ := abi.NewType("uint256", "", nil)
	v3Args := abi.Arguments{
		{Type: uint128Ty},
		{Type: uint256Ty},
		{Type: uint256Ty},
	}

	pmCache := make(map[common.Address]*blockchain.V3PositionManager)
	for _, addr := range managers {
		if pm, err := blockchain.NewV3PositionManager(addr, blockchain.Client); err == nil {
			pmCache[addr] = pm
		}
	}

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	if chain == "" {
		chain = "bsc"
	}

	events := make([]smartLPEvent, 0)
	txCache := make(map[common.Hash]txMeta)

	debug := config.AppConfig != nil && config.AppConfig.SmartLPDebug
	rawLogs := 0
	metaErrs := 0
	toMismatches := 0
	posErrs := 0
	poolErrs := 0
	unpackErrs := 0

	step := uint64(2000)
	for start := from; start <= to; {
		end := start + step - 1
		if end > to {
			end = to
		}

		query := ethereum.FilterQuery{
			FromBlock: big.NewInt(int64(start)),
			ToBlock:   big.NewInt(int64(end)),
			Addresses: managers,
			Topics:    [][]common.Hash{{increaseID, decreaseID}},
		}

		logs, err := blockchain.Client.FilterLogs(ctx, query)
		if err != nil {
			if isEthGetLogsRangeLimited(err) && step > 10 {
				step = 10
				continue
			}
			return nil, err
		}
		rawLogs += len(logs)

		for _, lg := range logs {
			if len(lg.Topics) == 0 {
				continue
			}
			action := ""
			switch lg.Topics[0] {
			case increaseID:
				action = "add"
			case decreaseID:
				action = "remove"
			default:
				continue
			}

			meta, ok := txCache[lg.TxHash]
			if !ok {
				m, err := loadTxMeta(ctx, lg.TxHash)
				if err != nil {
					metaErrs++
					if debug && metaErrs <= 3 {
						log.Printf("[SmartLP] v3 tx meta fetch failed tx=%s err=%v", strings.ToLower(lg.TxHash.Hex()), err)
					}
					continue
				}
				meta = m
				txCache[lg.TxHash] = meta
			}
			if !meta.ok || meta.to != monitorAddr {
				toMismatches++
				continue
			}

			if len(lg.Topics) < 2 {
				continue
			}
			tokenID := new(big.Int).SetBytes(lg.Topics[1].Bytes())
			if tokenID == nil || tokenID.Sign() <= 0 {
				continue
			}

			decoded, err := v3Args.Unpack(lg.Data)
			if err != nil || len(decoded) < 3 {
				unpackErrs++
				continue
			}

			liq, _ := decoded[0].(*big.Int)
			amount0, _ := decoded[1].(*big.Int)
			amount1, _ := decoded[2].(*big.Int)
			if liq == nil {
				liq = big.NewInt(0)
			}
			if amount0 == nil {
				amount0 = big.NewInt(0)
			}
			if amount1 == nil {
				amount1 = big.NewInt(0)
			}

			pm := pmCache[lg.Address]
			if pm == nil {
				continue
			}
			pos, err := pm.Positions(nil, tokenID)
			if err != nil {
				posErrs++
				if debug && posErrs <= 3 {
					log.Printf("[SmartLP] v3 positions call failed npm=%s token_id=%s tx=%s err=%v", strings.ToLower(lg.Address.Hex()), tokenID.String(), strings.ToLower(lg.TxHash.Hex()), err)
				}
				continue
			}

			poolAddr, err := resolveV3PoolAddress(lg.Address, pos.Token0, pos.Token1, pos.Fee)
			if err != nil {
				poolErrs++
				if debug && poolErrs <= 3 {
					log.Printf("[SmartLP] v3 resolve pool failed npm=%s token0=%s token1=%s fee=%d tx=%s err=%v", strings.ToLower(lg.Address.Hex()), strings.ToLower(pos.Token0.Hex()), strings.ToLower(pos.Token1.Hex()), pos.Fee, strings.ToLower(lg.TxHash.Hex()), err)
				}
				continue
			}

			eventSeq := uint64(lg.BlockNumber)*1_000_000 + uint64(lg.Index)
			events = append(events, smartLPEvent{
				ts:              time.Now(),
				eventSeq:        eventSeq,
				chain:           chain,
				poolVersion:     "v3",
				poolID:          strings.ToLower(poolAddr.Hex()),
				walletAddress:   strings.ToLower(meta.from.Hex()),
				action:          action,
				tokenID:         tokenID.String(),
				amount0:         amount0.String(),
				amount1:         amount1.String(),
				liquidityDelta:  liq.String(),
				txHash:          strings.ToLower(lg.TxHash.Hex()),
				blockNumber:     lg.BlockNumber,
				logIndex:        uint32(lg.Index),
				contractAddress: strings.ToLower(monitorAddr.Hex()),
				source:          "v3_npm",
			})
		}

		start = end + 1
	}

	if debug {
		log.Printf("[SmartLP] v3 scan summary raw_logs=%d meta_err=%d to_mismatch=%d unpack_err=%d positions_err=%d pool_resolve_err=%d recorded=%d",
			rawLogs, metaErrs, toMismatches, unpackErrs, posErrs, poolErrs, len(events),
		)
	}
	return events, nil
}

func (s *SmartLPMonitor) scanV4Logs(ctx context.Context, from, to uint64, monitorAddr common.Address, poolManager common.Address) ([]smartLPEvent, error) {
	modifyID := crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)"))

	int24Ty, _ := abi.NewType("int24", "", nil)
	int256Ty, _ := abi.NewType("int256", "", nil)
	bytes32Ty, _ := abi.NewType("bytes32", "", nil)
	v4Args := abi.Arguments{
		{Type: int24Ty},
		{Type: int24Ty},
		{Type: int256Ty},
		{Type: bytes32Ty},
	}

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	if chain == "" {
		chain = "bsc"
	}

	events := make([]smartLPEvent, 0)
	txCache := make(map[common.Hash]txMeta)
	receiptCache := make(map[common.Hash]*types.Receipt)

	debug := config.AppConfig != nil && config.AppConfig.SmartLPDebug
	rawLogs := 0
	metaErrs := 0
	toMismatches := 0
	unpackErrs := 0
	receiptErrs := 0

	type v4Tokens struct {
		t0 common.Address
		t1 common.Address
		ok bool
	}
	tokensCache := make(map[string]v4Tokens)

	step := uint64(2000)
	for start := from; start <= to; {
		end := start + step - 1
		if end > to {
			end = to
		}

		query := ethereum.FilterQuery{
			FromBlock: big.NewInt(int64(start)),
			ToBlock:   big.NewInt(int64(end)),
			Addresses: []common.Address{poolManager},
			Topics:    [][]common.Hash{{modifyID}},
		}

		logs, err := blockchain.Client.FilterLogs(ctx, query)
		if err != nil {
			if isEthGetLogsRangeLimited(err) && step > 10 {
				step = 10
				continue
			}
			return nil, err
		}
		rawLogs += len(logs)

		for _, lg := range logs {
			if len(lg.Topics) < 2 {
				continue
			}

			meta, ok := txCache[lg.TxHash]
			if !ok {
				m, err := loadTxMeta(ctx, lg.TxHash)
				if err != nil {
					metaErrs++
					if debug && metaErrs <= 3 {
						log.Printf("[SmartLP] v4 tx meta fetch failed tx=%s err=%v", strings.ToLower(lg.TxHash.Hex()), err)
					}
					continue
				}
				meta = m
				txCache[lg.TxHash] = meta
			}
			if !meta.ok || meta.to != monitorAddr {
				toMismatches++
				continue
			}

			decoded, err := v4Args.Unpack(lg.Data)
			if err != nil || len(decoded) < 4 {
				unpackErrs++
				continue
			}
			liqDelta, _ := decoded[2].(*big.Int)
			if liqDelta == nil || liqDelta.Sign() == 0 {
				continue
			}

			action := "add"
			if liqDelta.Sign() < 0 {
				action = "remove"
			}

			poolID := strings.ToLower(lg.Topics[1].Hex())

			amount0 := "0"
			amount1 := "0"
			if meta.ok && (meta.from != common.Address{}) {
				toks, ok := tokensCache[poolID]
				if !ok {
					var c0, c1 common.Address
					var tokErr error
					if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
						posm := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
						c0, c1, _, _, _, tokErr = blockchain.GetUniswapV4PoolKeyFromPositionManager(posm, poolID)
					} else {
						tokErr = fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not set")
					}
					if tokErr != nil {
						c0, c1, _, _, _, tokErr = blockchain.GetUniswapV4PoolKeyFromInitializeEvent(poolManager, poolID)
					}
					if tokErr == nil {
						toks = v4Tokens{t0: c0, t1: c1, ok: true}
					} else {
						toks = v4Tokens{ok: false}
						if debug {
							log.Printf("[SmartLP] v4 resolve tokens failed pool_id=%s err=%v", poolID, tokErr)
						}
					}
					tokensCache[poolID] = toks
				}

				if toks.ok {
					rcpt, ok := receiptCache[lg.TxHash]
					if !ok {
						r, err := blockchain.Client.TransactionReceipt(ctx, lg.TxHash)
						if err != nil {
							receiptErrs++
							if debug && receiptErrs <= 3 {
								log.Printf("[SmartLP] v4 receipt fetch failed tx=%s err=%v", strings.ToLower(lg.TxHash.Hex()), err)
							}
						} else {
							rcpt = r
							receiptCache[lg.TxHash] = r
						}
					}
					if rcpt != nil {
						amount0 = netErc20TransferMagnitude(rcpt, toks.t0, meta.from, action)
						amount1 = netErc20TransferMagnitude(rcpt, toks.t1, meta.from, action)
					}
				}
			}

			eventSeq := uint64(lg.BlockNumber)*1_000_000 + uint64(lg.Index)
			events = append(events, smartLPEvent{
				ts:              time.Now(),
				eventSeq:        eventSeq,
				chain:           chain,
				poolVersion:     "v4",
				poolID:          poolID,
				walletAddress:   strings.ToLower(meta.from.Hex()),
				action:          action,
				tokenID:         "",
				amount0:         amount0,
				amount1:         amount1,
				liquidityDelta:  liqDelta.String(),
				txHash:          strings.ToLower(lg.TxHash.Hex()),
				blockNumber:     lg.BlockNumber,
				logIndex:        uint32(lg.Index),
				contractAddress: strings.ToLower(monitorAddr.Hex()),
				source:          "v4_pool_manager",
			})
		}

		start = end + 1
	}

	if debug {
		log.Printf("[SmartLP] v4 scan summary raw_logs=%d meta_err=%d to_mismatch=%d unpack_err=%d receipt_err=%d recorded=%d",
			rawLogs, metaErrs, toMismatches, unpackErrs, receiptErrs, len(events),
		)
	}
	return events, nil
}

func netErc20TransferMagnitude(receipt *types.Receipt, token common.Address, wallet common.Address, action string) string {
	if receipt == nil || token == (common.Address{}) || wallet == (common.Address{}) {
		return "0"
	}

	in := big.NewInt(0)
	out := big.NewInt(0)
	for _, lg := range receipt.Logs {
		if lg == nil {
			continue
		}
		if lg.Address != token {
			continue
		}
		if len(lg.Topics) < 3 || lg.Topics[0] != erc20TransferID {
			continue
		}
		// ERC20 Transfer: amount is non-indexed in data (32 bytes). Skip ERC721-style transfers (data empty).
		if len(lg.Data) < 32 {
			continue
		}
		amount := new(big.Int).SetBytes(lg.Data[:32])
		if amount.Sign() <= 0 {
			continue
		}
		from := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
		to := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
		if from == wallet {
			out.Add(out, amount)
		}
		if to == wallet {
			in.Add(in, amount)
		}
	}

	switch action {
	case "add":
		if out.Cmp(in) <= 0 {
			return "0"
		}
		return new(big.Int).Sub(out, in).String()
	case "remove":
		if in.Cmp(out) <= 0 {
			return "0"
		}
		return new(big.Int).Sub(in, out).String()
	default:
		return new(big.Int).Sub(in, out).String()
	}
}

type txMeta struct {
	from common.Address
	to   common.Address
	ok   bool
}

func loadTxMeta(ctx context.Context, hash common.Hash) (txMeta, error) {
	tx, _, err := blockchain.Client.TransactionByHash(ctx, hash)
	if err != nil {
		return txMeta{}, err
	}
	if tx.To() == nil {
		return txMeta{ok: false}, nil
	}
	signer := types.LatestSignerForChainID(blockchain.ChainID)
	from, err := types.Sender(signer, tx)
	if err != nil {
		return txMeta{}, err
	}
	return txMeta{from: from, to: *tx.To(), ok: true}, nil
}

func resolveV3PoolAddress(npm common.Address, token0 common.Address, token1 common.Address, fee uint64) (common.Address, error) {
	var factories []common.Address
	pancakeFactory := common.HexToAddress("0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865")
	uniswapFactory := common.HexToAddress("0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7")

	if config.AppConfig != nil {
		if common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) && npm == common.HexToAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
			factories = append(factories, pancakeFactory)
		} else if common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) && npm == common.HexToAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
			factories = append(factories, uniswapFactory)
		}
	}
	if len(factories) == 0 {
		factories = append(factories, pancakeFactory, uniswapFactory)
	}

	for _, factory := range factories {
		pool, err := blockchain.GetV3PoolFromFactory(factory, token0, token1, fee)
		if err != nil {
			continue
		}
		if pool != (common.Address{}) {
			return pool, nil
		}
	}
	return common.Address{}, fmt.Errorf("v3 pool not found")
}

func isEthGetLogsRangeLimited(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "eth_getLogs") && strings.Contains(msg, "10 block range")
}

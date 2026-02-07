package smart_lp

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/clickhouse"
	"TgLpBot/base/config"
	"context"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var erc20TransferID = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

type SmartLPMonitor struct {
	ch *clickhouse.ClickHouseService

	stopChan chan struct{}
	ticker   *time.Ticker
	interval time.Duration

	removeTicker   *time.Ticker
	removeInterval time.Duration
}

type smartLPV3Pos struct {
	token0 common.Address
	token1 common.Address
	fee    uint64
	tickL  int
	tickU  int
	ok     bool
}

type smartLPV4Tokens struct {
	t0 common.Address
	t1 common.Address
	ok bool
}

type smartLPReceiptScanner struct {
	debug       bool
	chain       string
	callTimeout time.Duration

	v3ManagersSet map[common.Address]struct{}
	pmCache       map[common.Address]*blockchain.V3PositionManager
	v3Args        abi.Arguments
	v3PosCache    map[string]smartLPV3Pos
	v3PoolCache   map[string]common.Address

	hasV4         bool
	v4PoolManager common.Address
	v4Args        abi.Arguments
	tokensCache   map[string]smartLPV4Tokens

	increaseID common.Hash
	decreaseID common.Hash
	modifyID   common.Hash
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
	netAmount0      string
	netAmount1      string
	liquidityDelta  string
	tickLower       int
	tickUpper       int
	txHash          string
	blockNumber     uint64
	logIndex        uint32
	contractAddress string
	source          string
}

func NewSmartLPMonitor(ch *clickhouse.ClickHouseService) *SmartLPMonitor {
	interval := 60 * time.Second
	if config.AppConfig != nil && config.AppConfig.SmartLPScanIntervalSeconds > 0 {
		interval = time.Duration(config.AppConfig.SmartLPScanIntervalSeconds) * time.Second
	}
	removeInterval := interval
	return &SmartLPMonitor{
		ch:             ch,
		stopChan:       make(chan struct{}),
		ticker:         time.NewTicker(interval),
		interval:       interval,
		removeTicker:   time.NewTicker(removeInterval),
		removeInterval: removeInterval,
	}
}

func (s *SmartLPMonitor) Start() {
	if config.AppConfig == nil || !config.AppConfig.SmartLPEnabled {
		log.Println("[SmartLP] disabled (SMART_LP_ENABLED=0)")
		return
	}
	if err := s.resetScanStateToHead(); err != nil {
		log.Printf("[SmartLP] reset scan state to head failed: %v", err)
	}

	if wsURL := smartLPWebsocketURL(); wsURL != "" {
		if s.ticker != nil {
			s.ticker.Stop()
			s.ticker = nil
		}
		log.Printf("[SmartLP] websocket enabled url=%s", wsURL)
		go s.runWebsocket(wsURL)
		go s.runRemoveWatcherLoop()
		return
	}

	if config.AppConfig != nil && config.AppConfig.SmartLPDebug {
		if s != nil && s.interval > 0 {
			log.Printf("[SmartLP] websocket not configured; using polling interval=%s", s.interval)
		} else {
			log.Printf("[SmartLP] websocket not configured; using polling mode")
		}
	}
	go s.runLoop()
	go s.runRemoveWatcherLoop()
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
	if s.removeTicker != nil {
		s.removeTicker.Stop()
	}
}

func (s *SmartLPMonitor) runLoop() {
	log.Println("[SmartLP] monitor started")
	s.runOnce()
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

func (s *SmartLPMonitor) runWebsocket(wsURL string) {
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

	monitorAddrList := parseHexAddressList(config.AppConfig.SmartLPContractAddress)
	if len(monitorAddrList) == 0 {
		log.Println("[SmartLP] SMART_LP_CONTRACT_ADDRESS invalid/empty")
		return
	}
	monitorAddrs := make(map[common.Address]struct{}, len(monitorAddrList))
	for _, addr := range monitorAddrList {
		monitorAddrs[addr] = struct{}{}
	}

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

	debug := config.AppConfig != nil && config.AppConfig.SmartLPDebug

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	if chain == "" {
		chain = "bsc"
	}

	callTimeout := 30 * time.Second
	if config.AppConfig != nil && config.AppConfig.SmartLPRPCTimeoutSeconds > 0 {
		callTimeout = time.Duration(config.AppConfig.SmartLPRPCTimeoutSeconds) * time.Second
	}

	scanner := newSmartLPReceiptScanner(chain, callTimeout, debug, v3Managers, v4PoolManager)

	backoff := 1 * time.Second
	for {
		if err := s.runWebsocketSession(wsURL, monitorAddrList, monitorAddrs, scanner); err != nil {
			select {
			case <-s.stopChan:
				log.Println("[SmartLP] websocket monitor stopped")
				return
			default:
			}
			if debug {
				log.Printf("[SmartLP] websocket session ended: %v (reconnecting after %s)", err, backoff)
			} else {
				log.Printf("[SmartLP] websocket session ended (reconnecting after %s): %v", backoff, err)
			}

			t := time.NewTimer(backoff)
			select {
			case <-s.stopChan:
				t.Stop()
				log.Println("[SmartLP] websocket monitor stopped")
				return
			case <-t.C:
			}
			if backoff < 30*time.Second {
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
			continue
		}

		log.Println("[SmartLP] websocket monitor stopped")
		return
	}
}

func (s *SmartLPMonitor) runWebsocketSession(wsURL string, monitorAddrList []common.Address, monitorAddrs map[common.Address]struct{}, scanner *smartLPReceiptScanner) error {
	dialCtx, cancelDial := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelDial()
	go func() {
		select {
		case <-s.stopChan:
			cancelDial()
		case <-dialCtx.Done():
		}
	}()

	client, err := ethclient.DialContext(dialCtx, wsURL)
	if err != nil {
		return err
	}
	defer client.Close()

	headerCh := make(chan *types.Header, 256)
	subCtx, cancelSub := context.WithCancel(context.Background())
	defer cancelSub()
	go func() {
		select {
		case <-s.stopChan:
			cancelSub()
		case <-subCtx.Done():
		}
	}()

	sub, err := client.SubscribeNewHead(subCtx, headerCh)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	if scanner != nil && scanner.debug {
		monStrs := make([]string, 0, len(monitorAddrList))
		for _, a := range monitorAddrList {
			monStrs = append(monStrs, strings.ToLower(a.Hex()))
		}
		log.Printf("[SmartLP] websocket subscribed new heads monitor_contracts=%s", strings.Join(monStrs, ","))
	} else {
		log.Printf("[SmartLP] websocket subscribed new heads monitor_contracts=%d", len(monitorAddrList))
	}

	blockQueue := make(chan uint64, 512)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		s.smartLPWebsocketBlockWorker(blockQueue, monitorAddrs, scanner)
	}()

	type seenBlock struct {
		hash common.Hash
		ts   time.Time
	}
	seenBlocks := make(map[uint64]seenBlock)
	var lastQueued uint64
	cleanupTicker := time.NewTicker(20 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-s.stopChan:
			close(blockQueue)
			<-workerDone
			return nil
		case err := <-sub.Err():
			close(blockQueue)
			<-workerDone
			if err == nil {
				return fmt.Errorf("websocket subscription ended")
			}
			return err
		case head := <-headerCh:
			if head == nil || head.Number == nil {
				continue
			}
			bn := head.Number.Uint64()
			if bn == 0 {
				continue
			}
			hh := head.Hash()
			if prev, ok := seenBlocks[bn]; ok && prev.hash == hh {
				continue
			}
			seenBlocks[bn] = seenBlock{hash: hh, ts: time.Now()}

			start := bn
			if lastQueued > 0 && bn > lastQueued+1 {
				start = lastQueued + 1
			}
			if bn <= lastQueued {
				start = bn
			}

			for i := start; i <= bn; i++ {
				select {
				case blockQueue <- i:
				default:
					if scanner != nil && scanner.debug {
						log.Printf("[SmartLP] websocket block queue full; drop block=%d", i)
					}
					delete(seenBlocks, i)
				}
			}
			if bn > lastQueued {
				lastQueued = bn
			}
		case <-cleanupTicker.C:
			cutoff := time.Now().Add(-2 * time.Hour)
			for b, info := range seenBlocks {
				if info.ts.Before(cutoff) {
					delete(seenBlocks, b)
				}
			}
		}
	}
}

func (s *SmartLPMonitor) smartLPWebsocketBlockWorker(blockQueue <-chan uint64, monitorAddrs map[common.Address]struct{}, scanner *smartLPReceiptScanner) {
	if s == nil || s.ch == nil || s.ch.Conn == nil {
		return
	}
	if scanner == nil {
		return
	}

	for bn := range blockQueue {
		if blockchain.Client == nil || blockchain.ChainID == nil {
			continue
		}

		events, err := s.scanSmartLPBlockWithScanner(context.Background(), bn, monitorAddrs, scanner)
		if err != nil {
			log.Printf("[SmartLP] websocket scan block failed block=%d err=%v", bn, err)
			continue
		}
		if len(events) == 0 {
			continue
		}
		insertCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = s.insertEvents(insertCtx, events)
		cancel()
		if err != nil {
			log.Printf("[SmartLP] insert events failed: %v", err)
			continue
		}
		if err := s.upsertWatchedWallets(context.Background(), events, "scan_add"); err != nil {
			log.Printf("[SmartLP] upsert watched wallets failed: %v", err)
		}
		log.Printf("[SmartLP] inserted events: %d", len(events))
	}
}

func (s *SmartLPMonitor) scanSmartLPBlockWithScanner(ctx context.Context, blockNumber uint64, monitorAddrs map[common.Address]struct{}, scanner *smartLPReceiptScanner) ([]smartLPEvent, error) {
	if s == nil || scanner == nil {
		return nil, nil
	}
	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if blockNumber == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	callTimeout := scanner.callTimeout
	if callTimeout <= 0 {
		callTimeout = 30 * time.Second
	}

	type rpcTx struct {
		Hash common.Hash     `json:"hash"`
		From common.Address  `json:"from"`
		To   *common.Address `json:"to"`
	}
	type rpcBlock struct {
		Transactions []rpcTx `json:"transactions"`
	}

	var blk rpcBlock
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		blockCtx, cancel := context.WithTimeout(ctx, callTimeout)
		err = blockchain.Client.Client().CallContext(blockCtx, &blk, "eth_getBlockByNumber", fmt.Sprintf("0x%x", blockNumber), true)
		cancel()
		if err == nil {
			break
		}
		if attempt >= 3 || !isRetryableRPCError(err) {
			break
		}
		if scanner.debug {
			log.Printf("[SmartLP] websocket getBlock retrying block=%d attempt=%d err=%v", blockNumber, attempt, err)
		}
		delay := time.Duration(attempt) * 500 * time.Millisecond
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
	if err != nil {
		return nil, fmt.Errorf("eth_getBlockByNumber failed block=%d: %w", blockNumber, err)
	}

	events := make([]smartLPEvent, 0)
	receiptErrs := 0
	candidateTxs := 0

	for _, tx := range blk.Transactions {
		if (tx.Hash == common.Hash{}) {
			continue
		}
		if tx.To == nil || *tx.To == (common.Address{}) {
			continue
		}
		contractTo := *tx.To
		if monitorAddrs != nil {
			if _, ok := monitorAddrs[contractTo]; !ok {
				continue
			}
		}
		candidateTxs++

		receiptCtx, cancel := context.WithTimeout(ctx, callTimeout)
		receipt, err := blockchain.Client.TransactionReceipt(receiptCtx, tx.Hash)
		cancel()
		if err != nil || receipt == nil {
			receiptErrs++
			if scanner.debug && receiptErrs <= 3 {
				log.Printf("[SmartLP] websocket receipt fetch failed tx=%s err=%v", strings.ToLower(tx.Hash.Hex()), err)
			}
			continue
		}

		txEvents := scanner.scanReceipt(ctx, receipt, tx.From, contractTo, tx.Hash)
		if len(txEvents) > 0 {
			events = append(events, txEvents...)
		}
	}

	if scanner.debug {
		log.Printf("[SmartLP] websocket block=%d txs=%d candidates=%d events=%d", blockNumber, len(blk.Transactions), candidateTxs, len(events))
	}

	return events, nil
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
	monitorAddrList := parseHexAddressList(config.AppConfig.SmartLPContractAddress)
	if len(monitorAddrList) == 0 {
		log.Println("[SmartLP] SMART_LP_CONTRACT_ADDRESS invalid/empty")
		return
	}
	monitorAddrs := make(map[common.Address]struct{}, len(monitorAddrList))
	for _, addr := range monitorAddrList {
		monitorAddrs[addr] = struct{}{}
	}

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

	scanTimeout := 10 * time.Minute
	if config.AppConfig != nil && config.AppConfig.SmartLPScanTimeoutSeconds > 0 {
		scanTimeout = time.Duration(config.AppConfig.SmartLPScanTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
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
	headBlock := head
	to := headBlock
	maxBlocks := 200
	if config.AppConfig != nil && config.AppConfig.SmartLPMaxBlocksPerScan > 0 {
		maxBlocks = config.AppConfig.SmartLPMaxBlocksPerScan
	}
	if maxBlocks > 0 {
		maxU := uint64(maxBlocks)
		if maxU > 0 && from+maxU-1 < to {
			to = from + maxU - 1
		}
	}
	log.Printf("[SmartLP] scanning blocks %d-%d (head=%d mode=receipts v3_managers=%d v4=%v)", from, to, headBlock, len(v3Managers), hasV4)

	events, lastScanned, err := s.scanBlocks(ctx, from, to, monitorAddrs, v3Managers, v4PoolManager)
	if err != nil {
		log.Printf("[SmartLP] scan blocks failed: %v", err)
		if len(events) > 0 {
			if err := s.insertEvents(ctx, events); err != nil {
				log.Printf("[SmartLP] insert events failed: %v", err)
				return
			}
			if err := s.upsertWatchedWallets(ctx, events, "scan_add"); err != nil {
				log.Printf("[SmartLP] upsert watched wallets failed: %v", err)
			}
			log.Printf("[SmartLP] inserted events: %d", len(events))
		}
		if lastScanned >= from {
			if err := s.saveLastScannedBlock(ctx, lastScanned); err != nil {
				log.Printf("[SmartLP] update scan state failed: %v", err)
				return
			}
			log.Printf("[SmartLP] partial scan completed (last=%d head=%d)", lastScanned, headBlock)
		}
		return
	}

	if len(events) > 0 {
		if err := s.insertEvents(ctx, events); err != nil {
			log.Printf("[SmartLP] insert events failed: %v", err)
			return
		}
		if err := s.upsertWatchedWallets(ctx, events, "scan_add"); err != nil {
			log.Printf("[SmartLP] upsert watched wallets failed: %v", err)
		}
		log.Printf("[SmartLP] inserted events: %d", len(events))
	} else {
		log.Printf("[SmartLP] no events found in range")
	}

	if err := s.saveLastScannedBlock(ctx, lastScanned); err != nil {
		log.Printf("[SmartLP] update scan state failed: %v", err)
		return
	}
	log.Printf("[SmartLP] scan completed (last=%d head=%d)", lastScanned, headBlock)
}

func (s *SmartLPMonitor) resetScanStateToHead() error {
	if config.AppConfig == nil || !config.AppConfig.SmartLPEnabled {
		return nil
	}
	if s == nil || s.ch == nil || s.ch.Conn == nil {
		return fmt.Errorf("clickhouse not initialized")
	}
	if blockchain.Client == nil {
		return fmt.Errorf("blockchain client not initialized")
	}

	rpcTimeout := 30 * time.Second
	if config.AppConfig.SmartLPRPCTimeoutSeconds > 0 {
		rpcTimeout = time.Duration(config.AppConfig.SmartLPRPCTimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	head, err := blockchain.Client.BlockNumber(ctx)
	if err != nil {
		return err
	}
	if err := s.saveLastScannedBlock(ctx, head); err != nil {
		return err
	}
	log.Printf("[SmartLP] scan state reset to head=%d (skip backfill)", head)
	return nil
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

	batch, err := s.ch.PrepareBatch(ctx, `INSERT INTO smart_lp_events (
		ts, event_seq, chain, pool_version, pool_id, wallet_address, action,
		token_id, amount0, amount1, net_amount0, net_amount1, liquidity_delta, tick_lower, tick_upper, tx_hash, block_number, log_index,
		contract_address, source
	)`)
	if err != nil {
		return err
	}
	defer func() { _ = batch.Abort() }()

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
			ev.netAmount0,
			ev.netAmount1,
			ev.liquidityDelta,
			int32(ev.tickLower),
			int32(ev.tickUpper),
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
	case strings.Contains(msg, "acquire conn timeout"):
		return true
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

func isRetryableRPCError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded"):
		return true
	case strings.Contains(msg, "timeout"):
		return true
	case strings.Contains(msg, "i/o timeout"):
		return true
	case strings.Contains(msg, "connection reset"):
		return true
	case strings.Contains(msg, "broken pipe"):
		return true
	case strings.Contains(msg, "eof"):
		return true
	case strings.Contains(msg, "too many requests"):
		return true
	case strings.Contains(msg, "rate limit"):
		return true
	default:
		return false
	}
}

func (s *SmartLPMonitor) scanBlocks(ctx context.Context, from, to uint64, monitorAddrs map[common.Address]struct{}, v3Managers []common.Address, v4PoolManager common.Address) ([]smartLPEvent, uint64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return nil, 0, fmt.Errorf("smartlp monitor is nil")
	}
	if blockchain.Client == nil || blockchain.ChainID == nil {
		return nil, 0, fmt.Errorf("blockchain client not initialized")
	}
	if blockchain.Client.Client() == nil {
		return nil, 0, fmt.Errorf("rpc client not initialized")
	}

	debug := config.AppConfig != nil && config.AppConfig.SmartLPDebug

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	if chain == "" {
		chain = "bsc"
	}

	callTimeout := 30 * time.Second
	if config.AppConfig != nil && config.AppConfig.SmartLPRPCTimeoutSeconds > 0 {
		callTimeout = time.Duration(config.AppConfig.SmartLPRPCTimeoutSeconds) * time.Second
	}

	events := make([]smartLPEvent, 0)
	lastScanned := uint64(0)
	if from > 0 {
		lastScanned = from - 1
	}

	// V3 setup (PositionManager events parsed from receipt logs)
	increaseID := crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)"))
	decreaseID := crypto.Keccak256Hash([]byte("DecreaseLiquidity(uint256,uint128,uint256,uint256)"))

	v3ManagersSet := make(map[common.Address]struct{}, len(v3Managers))
	pmCache := make(map[common.Address]*blockchain.V3PositionManager, len(v3Managers))

	var v3Args abi.Arguments
	if len(v3Managers) > 0 {
		uint128Ty, _ := abi.NewType("uint128", "", nil)
		uint256Ty, _ := abi.NewType("uint256", "", nil)
		v3Args = abi.Arguments{
			{Type: uint128Ty},
			{Type: uint256Ty},
			{Type: uint256Ty},
		}

		for _, addr := range v3Managers {
			v3ManagersSet[addr] = struct{}{}
			if pm, err := blockchain.NewV3PositionManager(addr, blockchain.Client); err == nil {
				pmCache[addr] = pm
			} else if debug {
				log.Printf("[SmartLP] init v3 position manager failed addr=%s err=%v", strings.ToLower(addr.Hex()), err)
			}
		}
	}

	type v3Pos struct {
		token0 common.Address
		token1 common.Address
		fee    uint64
		tickL  int
		tickU  int
		ok     bool
	}
	v3PosCache := make(map[string]v3Pos)
	v3PoolCache := make(map[string]common.Address)

	// V4 setup (PoolManager ModifyLiquidity events parsed from receipt logs)
	hasV4 := v4PoolManager != (common.Address{})
	modifyID := crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)"))

	var v4Args abi.Arguments
	if hasV4 {
		int24Ty, _ := abi.NewType("int24", "", nil)
		int256Ty, _ := abi.NewType("int256", "", nil)
		bytes32Ty, _ := abi.NewType("bytes32", "", nil)
		v4Args = abi.Arguments{
			{Type: int24Ty},
			{Type: int24Ty},
			{Type: int256Ty},
			{Type: bytes32Ty},
		}
	}

	type v4Tokens struct {
		t0 common.Address
		t1 common.Address
		ok bool
	}
	tokensCache := make(map[string]v4Tokens)

	type rpcTx struct {
		Hash common.Hash     `json:"hash"`
		From common.Address  `json:"from"`
		To   *common.Address `json:"to"`
	}
	type rpcBlock struct {
		Transactions []rpcTx `json:"transactions"`
	}

	blocksScanned := 0
	txsScanned := 0
	candidateTxs := 0
	receiptErrs := 0
	unpackErrs := 0
	posErrs := 0
	poolErrs := 0
	v3Raw := 0
	v4Raw := 0

	lastProgressLog := time.Now()
	logProgress := func(bn uint64) {
		if !debug && time.Since(lastProgressLog) < 20*time.Second {
			return
		}
		lastProgressLog = time.Now()
		log.Printf("[SmartLP] progress block=%d/%d candidates=%d events=%d", bn, to, candidateTxs, len(events))
	}

	for bn := from; bn <= to; bn++ {
		var blk rpcBlock

		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			if ctx.Err() != nil {
				return events, lastScanned, ctx.Err()
			}

			blockCtx, cancel := context.WithTimeout(ctx, callTimeout)
			err = blockchain.Client.Client().CallContext(blockCtx, &blk, "eth_getBlockByNumber", fmt.Sprintf("0x%x", bn), true)
			cancel()
			if err == nil {
				break
			}
			if attempt >= 3 || !isRetryableRPCError(err) {
				break
			}
			if debug {
				log.Printf("[SmartLP] getBlock retrying block=%d attempt=%d err=%v", bn, attempt, err)
			}

			delay := time.Duration(attempt) * 500 * time.Millisecond
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return events, lastScanned, ctx.Err()
			case <-t.C:
			}
		}
		if err != nil {
			return events, lastScanned, fmt.Errorf("eth_getBlockByNumber failed block=%d: %w", bn, err)
		}
		blocksScanned++
		logProgress(bn)

		txsScanned += len(blk.Transactions)

		for _, tx := range blk.Transactions {
			if (tx.Hash == common.Hash{}) {
				continue
			}
			if tx.To == nil || *tx.To == (common.Address{}) {
				continue
			}
			contractTo := *tx.To
			if monitorAddrs != nil {
				if _, ok := monitorAddrs[contractTo]; !ok {
					continue
				}
			}
			candidateTxs++

			fromAddr := tx.From

			receiptCtx, cancel := context.WithTimeout(ctx, callTimeout)
			receipt, err := blockchain.Client.TransactionReceipt(receiptCtx, tx.Hash)
			cancel()
			if err != nil {
				receiptErrs++
				if debug && receiptErrs <= 3 {
					log.Printf("[SmartLP] receipt fetch failed tx=%s err=%v", strings.ToLower(tx.Hash.Hex()), err)
				}
				continue
			}

			for _, lg := range receipt.Logs {
				if lg == nil || len(lg.Topics) == 0 {
					continue
				}

				// V3: IncreaseLiquidity/DecreaseLiquidity logs on V3 position managers
				if len(v3ManagersSet) > 0 {
					if _, ok := v3ManagersSet[lg.Address]; ok {
						v3Raw++

						action := ""
						switch lg.Topics[0] {
						case increaseID:
							action = "add"
						case decreaseID:
							action = "remove"
						default:
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

						posKey := strings.ToLower(lg.Address.Hex()) + "|" + tokenID.String()
						pos, ok := v3PosCache[posKey]
						if !ok {
							pm := pmCache[lg.Address]
							if pm == nil {
								v3PosCache[posKey] = v3Pos{ok: false}
							} else {
								callCtx, cancel := context.WithTimeout(ctx, callTimeout)
								p, err := pm.Positions(&bind.CallOpts{Context: callCtx}, tokenID)
								cancel()
								if err != nil {
									posErrs++
									if debug && posErrs <= 3 {
										log.Printf("[SmartLP] v3 positions call failed npm=%s token_id=%s tx=%s err=%v", strings.ToLower(lg.Address.Hex()), tokenID.String(), strings.ToLower(tx.Hash.Hex()), err)
									}
									v3PosCache[posKey] = v3Pos{ok: false}
								} else {
									v3PosCache[posKey] = v3Pos{token0: p.Token0, token1: p.Token1, fee: p.Fee, tickL: p.TickLower, tickU: p.TickUpper, ok: true}
								}
							}
							pos = v3PosCache[posKey]
						}
						if !pos.ok {
							continue
						}

						poolKey := strings.ToLower(lg.Address.Hex()) + "|" + strings.ToLower(pos.token0.Hex()) + "|" + strings.ToLower(pos.token1.Hex()) + "|" + fmt.Sprintf("%d", pos.fee)
						poolAddr, ok := v3PoolCache[poolKey]
						if !ok {
							pool, err := resolveV3PoolAddress(ctx, callTimeout, lg.Address, pos.token0, pos.token1, pos.fee)
							if err != nil {
								poolErrs++
								if debug && poolErrs <= 3 {
									log.Printf("[SmartLP] v3 resolve pool failed npm=%s token0=%s token1=%s fee=%d tx=%s err=%v", strings.ToLower(lg.Address.Hex()), strings.ToLower(pos.token0.Hex()), strings.ToLower(pos.token1.Hex()), pos.fee, strings.ToLower(tx.Hash.Hex()), err)
								}
								v3PoolCache[poolKey] = common.Address{}
							} else {
								v3PoolCache[poolKey] = pool
							}
							poolAddr = v3PoolCache[poolKey]
						}
						if poolAddr == (common.Address{}) {
							continue
						}

						net0 := netErc20TransferMagnitude(receipt, pos.token0, fromAddr, action)
						net1 := netErc20TransferMagnitude(receipt, pos.token1, fromAddr, action)

						eventSeq := bn*1_000_000 + uint64(lg.Index)
						events = append(events, smartLPEvent{
							ts:              time.Now(),
							eventSeq:        eventSeq,
							chain:           chain,
							poolVersion:     "v3",
							poolID:          strings.ToLower(poolAddr.Hex()),
							walletAddress:   strings.ToLower(fromAddr.Hex()),
							action:          action,
							tokenID:         tokenID.String(),
							amount0:         amount0.String(),
							amount1:         amount1.String(),
							netAmount0:      net0,
							netAmount1:      net1,
							liquidityDelta:  liq.String(),
							tickLower:       pos.tickL,
							tickUpper:       pos.tickU,
							txHash:          strings.ToLower(tx.Hash.Hex()),
							blockNumber:     bn,
							logIndex:        uint32(lg.Index),
							contractAddress: strings.ToLower(contractTo.Hex()),
							source:          "v3_npm",
						})
						continue
					}
				}

				// V4: ModifyLiquidity logs on V4 PoolManager
				if hasV4 && lg.Address == v4PoolManager && lg.Topics[0] == modifyID {
					v4Raw++
					if len(lg.Topics) < 2 {
						continue
					}

					decoded, err := v4Args.Unpack(lg.Data)
					if err != nil || len(decoded) < 4 {
						unpackErrs++
						continue
					}
					tickLowerBI, _ := decoded[0].(*big.Int)
					tickUpperBI, _ := decoded[1].(*big.Int)
					liqDelta, _ := decoded[2].(*big.Int)
					if liqDelta == nil || liqDelta.Sign() == 0 {
						continue
					}
					tickLower := 0
					tickUpper := 0
					if tickLowerBI != nil {
						tickLower = int(tickLowerBI.Int64())
					}
					if tickUpperBI != nil {
						tickUpper = int(tickUpperBI.Int64())
					}

					action := "add"
					if liqDelta.Sign() < 0 {
						action = "remove"
					}

					poolID := strings.ToLower(lg.Topics[1].Hex())

					amount0 := "0"
					amount1 := "0"

					toks, ok := tokensCache[poolID]
					if !ok {
						var c0, c1 common.Address
						var tokErr error
						if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
							posm := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
							callCtx, cancel := context.WithTimeout(ctx, callTimeout)
							c0, c1, _, _, _, tokErr = blockchain.GetUniswapV4PoolKeyFromPositionManagerCtx(callCtx, posm, poolID)
							cancel()
						} else {
							tokErr = fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not set")
						}
						if tokErr != nil {
							c0, c1, _, _, _, tokErr = blockchain.GetUniswapV4PoolKeyFromInitializeEvent(v4PoolManager, poolID)
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
						amount0 = netErc20TransferMagnitude(receipt, toks.t0, fromAddr, action)
						amount1 = netErc20TransferMagnitude(receipt, toks.t1, fromAddr, action)
					}

					eventSeq := bn*1_000_000 + uint64(lg.Index)
					events = append(events, smartLPEvent{
						ts:              time.Now(),
						eventSeq:        eventSeq,
						chain:           chain,
						poolVersion:     "v4",
						poolID:          poolID,
						walletAddress:   strings.ToLower(fromAddr.Hex()),
						action:          action,
						tokenID:         "",
						amount0:         amount0,
						amount1:         amount1,
						netAmount0:      amount0,
						netAmount1:      amount1,
						liquidityDelta:  liqDelta.String(),
						tickLower:       tickLower,
						tickUpper:       tickUpper,
						txHash:          strings.ToLower(tx.Hash.Hex()),
						blockNumber:     bn,
						logIndex:        uint32(lg.Index),
						contractAddress: strings.ToLower(contractTo.Hex()),
						source:          "v4_pool_manager",
					})
					continue
				}
			}
		}

		if debug && (blocksScanned == 1 || blocksScanned%10 == 0 || bn == to) {
			log.Printf("[SmartLP] scanned block=%d txs=%d candidates=%d events=%d", bn, len(blk.Transactions), candidateTxs, len(events))
		}

		lastScanned = bn
	}

	if debug {
		log.Printf("[SmartLP] receipt scan summary blocks=%d txs=%d candidates=%d receipt_err=%d unpack_err=%d v3_raw=%d v4_raw=%d v3_pos_err=%d v3_pool_err=%d recorded=%d",
			blocksScanned, txsScanned, candidateTxs, receiptErrs, unpackErrs, v3Raw, v4Raw, posErrs, poolErrs, len(events),
		)
	}
	return events, lastScanned, nil
}

func newSmartLPReceiptScanner(chain string, callTimeout time.Duration, debug bool, v3Managers []common.Address, v4PoolManager common.Address) *smartLPReceiptScanner {
	sc := &smartLPReceiptScanner{
		debug:       debug,
		chain:       chain,
		callTimeout: callTimeout,

		v3ManagersSet: make(map[common.Address]struct{}, len(v3Managers)),
		pmCache:       make(map[common.Address]*blockchain.V3PositionManager, len(v3Managers)),
		v3PosCache:    make(map[string]smartLPV3Pos),
		v3PoolCache:   make(map[string]common.Address),

		hasV4:         v4PoolManager != (common.Address{}),
		v4PoolManager: v4PoolManager,
		tokensCache:   make(map[string]smartLPV4Tokens),

		increaseID: crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)")),
		decreaseID: crypto.Keccak256Hash([]byte("DecreaseLiquidity(uint256,uint128,uint256,uint256)")),
		modifyID:   crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)")),
	}

	if len(v3Managers) > 0 {
		uint128Ty, _ := abi.NewType("uint128", "", nil)
		uint256Ty, _ := abi.NewType("uint256", "", nil)
		sc.v3Args = abi.Arguments{
			{Type: uint128Ty},
			{Type: uint256Ty},
			{Type: uint256Ty},
		}

		for _, addr := range v3Managers {
			sc.v3ManagersSet[addr] = struct{}{}
			if blockchain.Client == nil {
				continue
			}
			if pm, err := blockchain.NewV3PositionManager(addr, blockchain.Client); err == nil {
				sc.pmCache[addr] = pm
			} else if debug {
				log.Printf("[SmartLP] init v3 position manager failed addr=%s err=%v", strings.ToLower(addr.Hex()), err)
			}
		}
	}

	if sc.hasV4 {
		int24Ty, _ := abi.NewType("int24", "", nil)
		int256Ty, _ := abi.NewType("int256", "", nil)
		bytes32Ty, _ := abi.NewType("bytes32", "", nil)
		sc.v4Args = abi.Arguments{
			{Type: int24Ty},
			{Type: int24Ty},
			{Type: int256Ty},
			{Type: bytes32Ty},
		}
	}

	return sc
}

func (sc *smartLPReceiptScanner) scanReceipt(ctx context.Context, receipt *types.Receipt, fromAddr common.Address, contractTo common.Address, txHash common.Hash) []smartLPEvent {
	if ctx == nil {
		ctx = context.Background()
	}
	if sc == nil || receipt == nil {
		return nil
	}

	txHashStr := strings.ToLower(txHash.Hex())
	contractToStr := strings.ToLower(contractTo.Hex())
	fromStr := strings.ToLower(fromAddr.Hex())

	events := make([]smartLPEvent, 0)
	for _, lg := range receipt.Logs {
		if lg == nil || len(lg.Topics) == 0 {
			continue
		}

		// V3: IncreaseLiquidity/DecreaseLiquidity logs on V3 position managers
		if len(sc.v3ManagersSet) > 0 {
			if _, ok := sc.v3ManagersSet[lg.Address]; ok {
				action := ""
				switch lg.Topics[0] {
				case sc.increaseID:
					action = "add"
				case sc.decreaseID:
					action = "remove"
				default:
					action = ""
				}
				if action == "" {
					goto v4
				}

				if len(lg.Topics) < 2 {
					goto v4
				}
				tokenID := new(big.Int).SetBytes(lg.Topics[1].Bytes())
				if tokenID == nil || tokenID.Sign() <= 0 {
					goto v4
				}

				decoded, err := sc.v3Args.Unpack(lg.Data)
				if err != nil || len(decoded) < 3 {
					goto v4
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

				posKey := strings.ToLower(lg.Address.Hex()) + "|" + tokenID.String()
				pos, ok := sc.v3PosCache[posKey]
				if !ok {
					pm := sc.pmCache[lg.Address]
					if pm == nil {
						sc.v3PosCache[posKey] = smartLPV3Pos{ok: false}
					} else {
						callCtx, cancel := context.WithTimeout(ctx, sc.callTimeout)
						p, err := pm.Positions(&bind.CallOpts{Context: callCtx}, tokenID)
						cancel()
						if err != nil {
							if sc.debug {
								log.Printf("[SmartLP] v3 positions call failed npm=%s token_id=%s tx=%s err=%v", strings.ToLower(lg.Address.Hex()), tokenID.String(), txHashStr, err)
							}
							sc.v3PosCache[posKey] = smartLPV3Pos{ok: false}
						} else {
							sc.v3PosCache[posKey] = smartLPV3Pos{token0: p.Token0, token1: p.Token1, fee: p.Fee, tickL: p.TickLower, tickU: p.TickUpper, ok: true}
						}
					}
					pos = sc.v3PosCache[posKey]
				}
				if !pos.ok {
					goto v4
				}

				poolKey := strings.ToLower(lg.Address.Hex()) + "|" + strings.ToLower(pos.token0.Hex()) + "|" + strings.ToLower(pos.token1.Hex()) + "|" + fmt.Sprintf("%d", pos.fee)
				poolAddr, ok := sc.v3PoolCache[poolKey]
				if !ok {
					pool, err := resolveV3PoolAddress(ctx, sc.callTimeout, lg.Address, pos.token0, pos.token1, pos.fee)
					if err != nil {
						if sc.debug {
							log.Printf("[SmartLP] v3 resolve pool failed npm=%s token0=%s token1=%s fee=%d tx=%s err=%v", strings.ToLower(lg.Address.Hex()), strings.ToLower(pos.token0.Hex()), strings.ToLower(pos.token1.Hex()), pos.fee, txHashStr, err)
						}
						sc.v3PoolCache[poolKey] = common.Address{}
					} else {
						sc.v3PoolCache[poolKey] = pool
					}
					poolAddr = sc.v3PoolCache[poolKey]
				}
				if poolAddr == (common.Address{}) {
					goto v4
				}

				net0 := netErc20TransferMagnitude(receipt, pos.token0, fromAddr, action)
				net1 := netErc20TransferMagnitude(receipt, pos.token1, fromAddr, action)

				bn := lg.BlockNumber
				eventSeq := bn*1_000_000 + uint64(lg.Index)
				events = append(events, smartLPEvent{
					ts:              time.Now(),
					eventSeq:        eventSeq,
					chain:           sc.chain,
					poolVersion:     "v3",
					poolID:          strings.ToLower(poolAddr.Hex()),
					walletAddress:   fromStr,
					action:          action,
					tokenID:         tokenID.String(),
					amount0:         amount0.String(),
					amount1:         amount1.String(),
					netAmount0:      net0,
					netAmount1:      net1,
					liquidityDelta:  liq.String(),
					tickLower:       pos.tickL,
					tickUpper:       pos.tickU,
					txHash:          txHashStr,
					blockNumber:     bn,
					logIndex:        uint32(lg.Index),
					contractAddress: contractToStr,
					source:          "v3_npm",
				})
				continue
			}
		}

	v4:
		// V4: ModifyLiquidity logs on V4 PoolManager
		if sc.hasV4 && lg.Address == sc.v4PoolManager && lg.Topics[0] == sc.modifyID {
			if len(lg.Topics) < 2 {
				continue
			}

			decoded, err := sc.v4Args.Unpack(lg.Data)
			if err != nil || len(decoded) < 4 {
				continue
			}
			tickLowerBI, _ := decoded[0].(*big.Int)
			tickUpperBI, _ := decoded[1].(*big.Int)
			liqDelta, _ := decoded[2].(*big.Int)
			if liqDelta == nil || liqDelta.Sign() == 0 {
				continue
			}
			tickLower := 0
			tickUpper := 0
			if tickLowerBI != nil {
				tickLower = int(tickLowerBI.Int64())
			}
			if tickUpperBI != nil {
				tickUpper = int(tickUpperBI.Int64())
			}

			action := "add"
			if liqDelta.Sign() < 0 {
				action = "remove"
			}

			poolID := strings.ToLower(lg.Topics[1].Hex())

			amount0 := "0"
			amount1 := "0"

			toks, ok := sc.tokensCache[poolID]
			if !ok {
				var c0, c1 common.Address
				var tokErr error
				if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
					posm := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
					callCtx, cancel := context.WithTimeout(ctx, sc.callTimeout)
					c0, c1, _, _, _, tokErr = blockchain.GetUniswapV4PoolKeyFromPositionManagerCtx(callCtx, posm, poolID)
					cancel()
				} else {
					tokErr = fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not set")
				}
				if tokErr != nil {
					c0, c1, _, _, _, tokErr = blockchain.GetUniswapV4PoolKeyFromInitializeEvent(sc.v4PoolManager, poolID)
				}
				if tokErr == nil {
					toks = smartLPV4Tokens{t0: c0, t1: c1, ok: true}
				} else {
					toks = smartLPV4Tokens{ok: false}
					if sc.debug {
						log.Printf("[SmartLP] v4 resolve tokens failed pool_id=%s err=%v", poolID, tokErr)
					}
				}
				sc.tokensCache[poolID] = toks
			}

			if toks.ok {
				amount0 = netErc20TransferMagnitude(receipt, toks.t0, fromAddr, action)
				amount1 = netErc20TransferMagnitude(receipt, toks.t1, fromAddr, action)
			}

			bn := lg.BlockNumber
			eventSeq := bn*1_000_000 + uint64(lg.Index)
			events = append(events, smartLPEvent{
				ts:              time.Now(),
				eventSeq:        eventSeq,
				chain:           sc.chain,
				poolVersion:     "v4",
				poolID:          poolID,
				walletAddress:   fromStr,
				action:          action,
				tokenID:         "",
				amount0:         amount0,
				amount1:         amount1,
				netAmount0:      amount0,
				netAmount1:      amount1,
				liquidityDelta:  liqDelta.String(),
				tickLower:       tickLower,
				tickUpper:       tickUpper,
				txHash:          txHashStr,
				blockNumber:     bn,
				logIndex:        uint32(lg.Index),
				contractAddress: contractToStr,
				source:          "v4_pool_manager",
			})
			continue
		}
	}

	return events
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

func resolveV3PoolAddress(ctx context.Context, callTimeout time.Duration, npm common.Address, token0 common.Address, token1 common.Address, fee uint64) (common.Address, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if callTimeout <= 0 {
		callTimeout = 15 * time.Second
	}
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
		callCtx, cancel := context.WithTimeout(ctx, callTimeout)
		pool, err := blockchain.GetV3PoolFromFactoryCtx(callCtx, factory, token0, token1, fee)
		cancel()
		if err != nil {
			continue
		}
		if pool != (common.Address{}) {
			return pool, nil
		}
	}
	return common.Address{}, fmt.Errorf("v3 pool not found")
}

func parseHexAddressList(raw string) []common.Address {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})

	addrs := make([]common.Address, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" || !common.IsHexAddress(f) {
			continue
		}
		addr := common.HexToAddress(f)
		key := strings.ToLower(addr.Hex())
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		addrs = append(addrs, addr)
	}
	return addrs
}

func (s *SmartLPMonitor) upsertWatchedWallets(ctx context.Context, events []smartLPEvent, source string) error {
	if s == nil || s.ch == nil || s.ch.Conn == nil || len(events) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	type pair struct {
		chain  string
		wallet string
	}
	uniq := make(map[pair]time.Time)
	for _, ev := range events {
		if strings.ToLower(strings.TrimSpace(ev.action)) != "add" {
			continue
		}
		chain := strings.ToLower(strings.TrimSpace(ev.chain))
		wallet := strings.ToLower(strings.TrimSpace(ev.walletAddress))
		if chain == "" || !common.IsHexAddress(wallet) {
			continue
		}
		key := pair{chain: chain, wallet: wallet}
		last := ev.ts
		if last.IsZero() {
			last = time.Now()
		}
		if prev, ok := uniq[key]; !ok || last.After(prev) {
			uniq[key] = last
		}
	}
	if len(uniq) == 0 {
		return nil
	}

	batch, err := s.ch.PrepareBatch(ctx, `INSERT INTO smart_lp_watched_wallets (
		chain, wallet_address, last_add_at, source, updated_at
	)`)
	if err != nil {
		return err
	}
	defer func() { _ = batch.Abort() }()

	now := time.Now()
	src := strings.TrimSpace(source)
	if src == "" {
		src = "smart_lp"
	}
	for key, lastAddAt := range uniq {
		if err := batch.Append(
			key.chain,
			key.wallet,
			lastAddAt,
			src,
			now,
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

func (s *SmartLPMonitor) loadWatchedWallets(ctx context.Context, chain string) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	if s == nil || s.ch == nil || s.ch.Conn == nil {
		return out, fmt.Errorf("clickhouse not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	chain = strings.ToLower(strings.TrimSpace(chain))
	if chain == "" {
		return out, nil
	}

	rows, err := s.ch.Conn.Query(ctx, `
		SELECT wallet_address
		FROM smart_lp_watched_wallets
		WHERE lowerUTF8(chain) = ?
		GROUP BY wallet_address
	`, chain)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var wallet string
		if err := rows.Scan(&wallet); err != nil {
			return out, err
		}
		wallet = strings.ToLower(strings.TrimSpace(wallet))
		if common.IsHexAddress(wallet) {
			out[wallet] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *SmartLPMonitor) loadRemoveLastScannedBlock(ctx context.Context) (uint64, bool, error) {
	var last uint64
	var cnt uint64
	q := "SELECT argMax(last_block, updated_at) AS last_block, count() AS cnt FROM smart_lp_remove_scan_state WHERE id = 1"
	if err := s.ch.Conn.QueryRow(ctx, q).Scan(&last, &cnt); err != nil {
		return 0, false, err
	}
	if cnt == 0 {
		return 0, false, nil
	}
	return last, true, nil
}

func (s *SmartLPMonitor) saveRemoveLastScannedBlock(ctx context.Context, block uint64) error {
	q := "INSERT INTO smart_lp_remove_scan_state (id, last_block, updated_at) VALUES (1, ?, ?)"
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
			continue
		}
		return nil
	}
	return lastErr
}

func (s *SmartLPMonitor) runRemoveWatcherLoop() {
	if s == nil || s.removeTicker == nil {
		return
	}
	if config.AppConfig != nil && config.AppConfig.SmartLPDebug {
		log.Printf("[SmartLP] remove watcher started interval=%s", s.removeInterval)
	}
	s.runRemoveWatcherOnce()
	for {
		select {
		case <-s.removeTicker.C:
			s.runRemoveWatcherOnce()
		case <-s.stopChan:
			if config.AppConfig != nil && config.AppConfig.SmartLPDebug {
				log.Printf("[SmartLP] remove watcher stopped")
			}
			return
		}
	}
}

func (s *SmartLPMonitor) runRemoveWatcherOnce() {
	if config.AppConfig == nil || !config.AppConfig.SmartLPEnabled {
		return
	}
	if s == nil || s.ch == nil || s.ch.Conn == nil {
		return
	}
	if blockchain.Client == nil || blockchain.ChainID == nil {
		return
	}

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	if chain == "" {
		chain = "bsc"
	}

	v3Managers := make([]common.Address, 0, 2)
	if common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
		v3Managers = append(v3Managers, common.HexToAddress(config.AppConfig.PancakeV3PositionManagerAddress))
	}
	if common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
		v3Managers = append(v3Managers, common.HexToAddress(config.AppConfig.UniswapV3PositionManagerAddress))
	}
	hasV4 := common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	var v4PoolManager common.Address
	if hasV4 {
		v4PoolManager = common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	}
	if len(v3Managers) == 0 && !hasV4 {
		return
	}

	scanTimeout := 10 * time.Minute
	if config.AppConfig.SmartLPScanTimeoutSeconds > 0 {
		scanTimeout = time.Duration(config.AppConfig.SmartLPScanTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	watch, err := s.loadWatchedWallets(ctx, chain)
	if err != nil {
		log.Printf("[SmartLP] remove watcher load watchlist failed: %v", err)
		return
	}
	if len(watch) == 0 {
		if config.AppConfig.SmartLPDebug {
			log.Printf("[SmartLP] remove watcher skip: empty watchlist")
		}
		return
	}

	head, err := blockchain.Client.BlockNumber(ctx)
	if err != nil {
		log.Printf("[SmartLP] remove watcher get head block failed: %v", err)
		return
	}

	last, ok, err := s.loadRemoveLastScannedBlock(ctx)
	if err != nil {
		log.Printf("[SmartLP] remove watcher load state failed: %v", err)
		return
	}
	if !ok {
		if err := s.saveRemoveLastScannedBlock(ctx, head); err != nil {
			log.Printf("[SmartLP] remove watcher init state failed: %v", err)
		}
		return
	}
	if last >= head {
		return
	}

	from := last + 1
	to := head
	maxBlocks := 200
	if config.AppConfig.SmartLPMaxBlocksPerScan > 0 {
		maxBlocks = config.AppConfig.SmartLPMaxBlocksPerScan
	}
	if maxBlocks > 0 {
		maxU := uint64(maxBlocks)
		if maxU > 0 && from+maxU-1 < to {
			to = from + maxU - 1
		}
	}

	debug := config.AppConfig != nil && config.AppConfig.SmartLPDebug
	scanner := newSmartLPReceiptScanner(chain, 30*time.Second, debug, v3Managers, v4PoolManager)
	if config.AppConfig != nil && config.AppConfig.SmartLPRPCTimeoutSeconds > 0 {
		scanner.callTimeout = time.Duration(config.AppConfig.SmartLPRPCTimeoutSeconds) * time.Second
	}
	callTimeout := scanner.callTimeout
	if callTimeout <= 0 {
		callTimeout = 30 * time.Second
	}

	fromBI := new(big.Int).SetUint64(from)
	toBI := new(big.Int).SetUint64(to)
	candidateTxBlocks := make(map[common.Hash]uint64)

	collectCandidateLogs := func(q ethereum.FilterQuery, label string) error {
		logs, err := blockchain.Client.FilterLogs(ctx, q)
		if err != nil {
			return err
		}
		for _, lg := range logs {
			if lg.TxHash == (common.Hash{}) {
				continue
			}
			bn := lg.BlockNumber
			if bn == 0 {
				bn = from
			}
			if prev, ok := candidateTxBlocks[lg.TxHash]; !ok || bn < prev {
				candidateTxBlocks[lg.TxHash] = bn
			}
		}
		if debug {
			log.Printf("[SmartLP] remove watcher candidate logs source=%s range=%d-%d logs=%d txs=%d", label, from, to, len(logs), len(candidateTxBlocks))
		}
		return nil
	}

	if len(v3Managers) > 0 {
		q := ethereum.FilterQuery{
			FromBlock: fromBI,
			ToBlock:   toBI,
			Addresses: v3Managers,
			Topics:    [][]common.Hash{{scanner.decreaseID}},
		}
		if err := collectCandidateLogs(q, "v3_decrease"); err != nil {
			log.Printf("[SmartLP] remove watcher filter v3 logs failed: %v", err)
			return
		}
	}

	if hasV4 {
		q := ethereum.FilterQuery{
			FromBlock: fromBI,
			ToBlock:   toBI,
			Addresses: []common.Address{v4PoolManager},
			Topics:    [][]common.Hash{{scanner.modifyID}},
		}
		if err := collectCandidateLogs(q, "v4_modify"); err != nil {
			log.Printf("[SmartLP] remove watcher filter v4 logs failed: %v", err)
			return
		}
	}

	if len(candidateTxBlocks) == 0 {
		if err := s.saveRemoveLastScannedBlock(ctx, to); err != nil {
			log.Printf("[SmartLP] remove watcher save state failed: %v", err)
		}
		return
	}

	type candidateTx struct {
		hash  common.Hash
		block uint64
	}
	candidates := make([]candidateTx, 0, len(candidateTxBlocks))
	for txHash, block := range candidateTxBlocks {
		candidates = append(candidates, candidateTx{hash: txHash, block: block})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].block == candidates[j].block {
			return strings.ToLower(candidates[i].hash.Hex()) < strings.ToLower(candidates[j].hash.Hex())
		}
		return candidates[i].block < candidates[j].block
	})

	type rpcTx struct {
		Hash common.Hash     `json:"hash"`
		From common.Address  `json:"from"`
		To   *common.Address `json:"to"`
	}

	filtered := make([]smartLPEvent, 0, len(candidates))
	minFailedBlock := uint64(0)
	recordFailed := func(bn uint64) {
		if bn == 0 {
			bn = from
		}
		if minFailedBlock == 0 || bn < minFailedBlock {
			minFailedBlock = bn
		}
	}

	for _, item := range candidates {
		var tx rpcTx
		txCtx, cancel := context.WithTimeout(ctx, callTimeout)
		err := blockchain.Client.Client().CallContext(txCtx, &tx, "eth_getTransactionByHash", item.hash)
		cancel()
		if err != nil {
			if debug {
				log.Printf("[SmartLP] remove watcher tx lookup failed tx=%s err=%v", strings.ToLower(item.hash.Hex()), err)
			}
			recordFailed(item.block)
			continue
		}

		fromAddr := tx.From
		var contractTo common.Address
		if tx.To != nil {
			contractTo = *tx.To
		}

		if fromAddr == (common.Address{}) || contractTo == (common.Address{}) {
			txObjCtx, txObjCancel := context.WithTimeout(ctx, callTimeout)
			txObj, _, txErr := blockchain.Client.TransactionByHash(txObjCtx, item.hash)
			txObjCancel()
			if txErr == nil && txObj != nil {
				if contractTo == (common.Address{}) && txObj.To() != nil {
					contractTo = *txObj.To()
				}
				if fromAddr == (common.Address{}) && blockchain.ChainID != nil {
					signer := types.LatestSignerForChainID(blockchain.ChainID)
					if sender, senderErr := types.Sender(signer, txObj); senderErr == nil {
						fromAddr = sender
					}
				}
			}
		}

		if fromAddr == (common.Address{}) || contractTo == (common.Address{}) {
			recordFailed(item.block)
			continue
		}

		receiptCtx, receiptCancel := context.WithTimeout(ctx, callTimeout)
		receipt, err := blockchain.Client.TransactionReceipt(receiptCtx, item.hash)
		receiptCancel()
		if err != nil || receipt == nil {
			if debug {
				log.Printf("[SmartLP] remove watcher receipt lookup failed tx=%s err=%v", strings.ToLower(item.hash.Hex()), err)
			}
			recordFailed(item.block)
			continue
		}

		txEvents := scanner.scanReceipt(ctx, receipt, fromAddr, contractTo, item.hash)
		for _, ev := range txEvents {
			if strings.ToLower(strings.TrimSpace(ev.action)) != "remove" {
				continue
			}
			wallet := strings.ToLower(strings.TrimSpace(ev.walletAddress))
			if _, ok := watch[wallet]; !ok {
				continue
			}
			ev.source = "watch_remove"
			filtered = append(filtered, ev)
		}
	}

	if len(filtered) > 0 {
		if err := s.insertEvents(ctx, filtered); err != nil {
			log.Printf("[SmartLP] remove watcher insert failed: %v", err)
			return
		}
		log.Printf("[SmartLP] remove watcher inserted events: %d", len(filtered))
	}

	lastScanned := to
	if minFailedBlock > 0 {
		if minFailedBlock <= from {
			lastScanned = from - 1
		} else {
			lastScanned = minFailedBlock - 1
		}
	}

	if lastScanned >= from {
		if err := s.saveRemoveLastScannedBlock(ctx, lastScanned); err != nil {
			log.Printf("[SmartLP] remove watcher save state failed: %v", err)
		}
	} else if debug && minFailedBlock > 0 {
		log.Printf("[SmartLP] remove watcher preserved scan cursor (retry from block=%d)", from)
	}
}

func smartLPWebsocketURL() string {
	if config.AppConfig == nil {
		return ""
	}
	wsURL := strings.TrimSpace(config.AppConfig.BSCRpcWSURL)
	if wsURL != "" {
		return wsURL
	}
	rpcURL := strings.TrimSpace(config.AppConfig.BSCRpcURL)
	if strings.HasPrefix(rpcURL, "ws://") || strings.HasPrefix(rpcURL, "wss://") {
		return rpcURL
	}
	return ""
}

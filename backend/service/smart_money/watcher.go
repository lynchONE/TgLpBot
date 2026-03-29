package smart_money

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/rpcpool"
	"context"
	"fmt"
	"log"
	"math/big"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"TgLpBot/service/pricing"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"gorm.io/gorm"
)

var (
	TopicIncreaseLiquidity = crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)"))
	TopicDecreaseLiquidity = crypto.Keccak256Hash([]byte("DecreaseLiquidity(uint256,uint128,uint256,uint256)"))
	TopicModifyLiquidity   = crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)"))
	TopicTransfer          = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
)

const (
	smartMoneyMaxBlocksPerPoll        = 25
	smartMoneyMaxEventWorkers         = 8
	smartMoneyTransferWalletChunkSize = 32
	smartMoneyBaseErrorDelay          = 5 * time.Second
	smartMoneyRateLimitDelay          = 15 * time.Second
	smartMoneyMaxRetryDelay           = time.Minute
)

type Watcher struct {
	repo             *Repository
	notifier         func(*models.SmartMoneyLPEvent)
	lpContracts      []common.Address
	lpTopics         [][]common.Hash
	chainID          int64
	pollIntervalSec  int
	maxBlocksPerPoll int
	eventWorkers     int
	stopCh           chan struct{}
}

type blockTransaction struct {
	Hash  common.Hash
	From  string
	To    string
	Value string
}

type blockSnapshot struct {
	Number       uint64
	Timestamp    time.Time
	Transactions []blockTransaction
	TxSenders    map[common.Hash]string
}

type rawBlockSnapshot struct {
	Number       string `json:"number"`
	Timestamp    string `json:"timestamp"`
	Transactions []struct {
		Hash  string  `json:"hash"`
		From  string  `json:"from"`
		To    *string `json:"to"`
		Value string  `json:"value"`
	} `json:"transactions"`
}

type contractInteractionStats struct {
	MatchedTxCount int
	WalletCount    int
}

type lpLogStats struct {
	TotalLogs        int
	ActiveWalletLogs int
	HandledEvents    int
}

type transferTokenMeta struct {
	address  string
	symbol   string
	decimals int
	priceUSD float64
}

type blockProcessStats struct {
	TxCount             int
	ContractTxCount     int
	ContractWalletCount int
	LPLogCount          int
	LPActiveWalletLogs  int
	LPEventCount        int
}

type watcherScanStats struct {
	FromBlock           uint64
	ToBlock             uint64
	LatestBlock         uint64
	Blocks              int
	TxCount             int
	ContractTxCount     int
	ContractWalletCount int
	LPLogCount          int
	LPActiveWalletLogs  int
	LPEventCount        int
	StartedAt           time.Time
}

type pollResult struct {
	Remaining uint64
}

func NewWatcher(repo *Repository, chainID int64, pancakeV3NPM, uniswapV3NPM, uniswapV4PoolManager string, pollIntervalSec int) *Watcher {
	var lpContracts []common.Address
	if pancakeV3NPM != "" {
		lpContracts = append(lpContracts, common.HexToAddress(pancakeV3NPM))
	}
	if uniswapV3NPM != "" {
		lpContracts = append(lpContracts, common.HexToAddress(uniswapV3NPM))
	}
	if uniswapV4PoolManager != "" {
		lpContracts = append(lpContracts, common.HexToAddress(uniswapV4PoolManager))
	}

	if pollIntervalSec <= 0 {
		pollIntervalSec = 2
	}

	lpTopics := [][]common.Hash{{
		TopicIncreaseLiquidity,
		TopicDecreaseLiquidity,
		TopicModifyLiquidity,
	}}

	return &Watcher{
		repo:             repo,
		lpContracts:      lpContracts,
		lpTopics:         lpTopics,
		chainID:          chainID,
		pollIntervalSec:  pollIntervalSec,
		maxBlocksPerPoll: smartMoneyMaxBlocksPerPoll,
		eventWorkers:     defaultSmartMoneyEventWorkers(),
		stopCh:           make(chan struct{}),
	}
}

func defaultSmartMoneyEventWorkers() int {
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		workers = 2
	}
	if workers > smartMoneyMaxEventWorkers {
		workers = smartMoneyMaxEventWorkers
	}
	return workers
}

func (w *Watcher) hasLPContracts() bool {
	return len(w.lpContracts) > 0
}

func (w *Watcher) SetNotifier(fn func(*models.SmartMoneyLPEvent)) {
	w.notifier = fn
}

func (w *Watcher) Start(ctx context.Context) {
	if !hasSmartMoneyRPC(ctx, rpcpool.TransportHTTP) {
		log.Println("[SmartMoney Watcher] HTTP RPC not configured, watcher disabled")
		return
	}

	var lastProcessed uint64
	initialized := false
	pollDelay := time.Duration(w.pollIntervalSec) * time.Second
	if pollDelay <= 0 {
		pollDelay = 2 * time.Second
	}
	rateLimitDelay := smartMoneyRateLimitDelay

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}

		if !initialized {
			startBlock, err := w.getLatestHTTPBlock(ctx)
			if err != nil {
				delay := smartMoneyBaseErrorDelay
				if rpcpool.IsRateLimitedError(err) {
					delay = rateLimitDelay
					log.Printf("[SmartMoney Watcher] init rate limited: %v, backing off for %s...", err, delay)
					rateLimitDelay *= 2
					if rateLimitDelay > smartMoneyMaxRetryDelay {
						rateLimitDelay = smartMoneyMaxRetryDelay
					}
				} else {
					rateLimitDelay = smartMoneyRateLimitDelay
					log.Printf("[SmartMoney Watcher] init latest block failed: %v, retrying in %s...", err, delay)
				}
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return
				case <-w.stopCh:
					return
				}
				continue
			}

			lastProcessed = startBlock
			if err := w.repo.UpsertLPScanState(ctx, int(w.chainID), lastProcessed); err != nil {
				log.Printf("[SmartMoney Watcher] persist initial scan state failed: %v", err)
				select {
				case <-time.After(smartMoneyBaseErrorDelay):
				case <-ctx.Done():
					return
				case <-w.stopCh:
					return
				}
				continue
			}
			initialized = true
			rateLimitDelay = smartMoneyRateLimitDelay
			log.Printf("[SmartMoney Watcher] started from latest block %d, mode=http-polling, interval=%s, lp_contracts=%d, event_workers=%d",
				lastProcessed, pollDelay, len(w.lpContracts), w.eventWorkers)
		}

		result, err := w.pollOnce(ctx, &lastProcessed)
		if err != nil {
			delay := smartMoneyBaseErrorDelay
			if rpcpool.IsRateLimitedError(err) {
				delay = rateLimitDelay
				log.Printf("[SmartMoney Watcher] rate limited: %v, backing off for %s...", err, delay)
				rateLimitDelay *= 2
				if rateLimitDelay > smartMoneyMaxRetryDelay {
					rateLimitDelay = smartMoneyMaxRetryDelay
				}
			} else {
				rateLimitDelay = smartMoneyRateLimitDelay
				log.Printf("[SmartMoney Watcher] polling error: %v, retrying in %s...", err, delay)
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			}
			continue
		}
		rateLimitDelay = smartMoneyRateLimitDelay
		if result.Remaining > 0 {
			continue
		}

		select {
		case <-time.After(pollDelay):
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		}
	}
}

func (w *Watcher) Stop() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

func (w *Watcher) pollOnce(ctx context.Context, lastProcessed *uint64) (pollResult, error) {
	rpcClient, httpClient, eff, err := w.openHTTPRPC(ctx)
	if err != nil {
		return pollResult{}, err
	}
	defer rpcClient.Close()

	latestBlock, err := w.getLatestHTTPBlockWithClient(ctx, httpClient, eff)
	if err != nil {
		return pollResult{}, err
	}
	if latestBlock <= *lastProcessed {
		return pollResult{}, nil
	}

	fromBlock := *lastProcessed + 1
	toBlock := latestBlock
	if w.maxBlocksPerPoll > 0 {
		maxToBlock := fromBlock + uint64(w.maxBlocksPerPoll) - 1
		if toBlock > maxToBlock {
			toBlock = maxToBlock
		}
	}

	stats := watcherScanStats{
		FromBlock:   fromBlock,
		ToBlock:     toBlock,
		LatestBlock: latestBlock,
		Blocks:      int(toBlock - fromBlock + 1),
		StartedAt:   time.Now(),
	}
	log.Printf("[SmartMoney Watcher] scanning blocks=%d-%d latest=%d",
		stats.FromBlock, stats.ToBlock, stats.LatestBlock)

	contracts, err := w.repo.GetActiveWatchContractsByChain(ctx, int(w.chainID))
	if err != nil {
		return pollResult{}, err
	}
	contractByAddr := make(map[string]models.WatchContract, len(contracts))
	seenWallets := make(map[string]map[string]struct{}, len(contracts))
	for _, contract := range contracts {
		addr := strings.ToLower(strings.TrimSpace(contract.ContractAddress))
		if addr == "" {
			continue
		}
		contractByAddr[addr] = contract
	}

	smartMoneyWallets := make(map[string]struct{})
	if len(w.lpContracts) > 0 || len(contractByAddr) > 0 {
		smartMoneyWallets, err = w.repo.GetAllActiveWalletAddresses(ctx, int(w.chainID))
		if err != nil {
			return pollResult{}, err
		}
	}
	userWalletRefsByAddress, err := w.repo.GetAllUserWalletRefs(ctx)
	if err != nil {
		return pollResult{}, err
	}
	trackedTransferWallets := make(map[string]struct{}, len(smartMoneyWallets)+len(userWalletRefsByAddress))
	for addr := range smartMoneyWallets {
		trackedTransferWallets[addr] = struct{}{}
	}
	for addr := range userWalletRefsByAddress {
		trackedTransferWallets[addr] = struct{}{}
	}

	blocks, err := w.loadBlockSnapshots(ctx, rpcClient, eff, fromBlock, toBlock)
	if err != nil {
		return pollResult{}, err
	}
	logsByBlock, err := w.loadLPLogsByBlock(ctx, httpClient, eff, fromBlock, toBlock)
	if err != nil {
		return pollResult{}, err
	}
	excludedLPTxHashesByBlock := collectActiveWalletLPTxHashes(blocks, logsByBlock, trackedTransferWallets)
	transferEventsByBlock, err := w.loadWalletTransferEventsByBlock(ctx, httpClient, eff, blocks, trackedTransferWallets, excludedLPTxHashesByBlock)
	if err != nil {
		return pollResult{}, err
	}

	for _, block := range blocks {
		blockStats := blockProcessStats{
			TxCount:    len(block.Transactions),
			LPLogCount: len(logsByBlock[block.Number]),
		}

		if len(contractByAddr) > 0 {
			contractStats, newWallets, err := w.processContractInteractionsBlock(ctx, block, contractByAddr, seenWallets)
			if err != nil {
				return pollResult{}, fmt.Errorf("process block %d contract interactions: %w", block.Number, err)
			}
			blockStats.ContractTxCount = contractStats.MatchedTxCount
			for _, wallet := range newWallets {
				smartMoneyWallets[wallet] = struct{}{}
				trackedTransferWallets[wallet] = struct{}{}
			}
		}

		if blockStats.LPLogCount > 0 {
			lpStats, err := w.processLPLogsForBlock(ctx, block, logsByBlock[block.Number], smartMoneyWallets)
			if err != nil {
				return pollResult{}, fmt.Errorf("process block %d lp logs: %w", block.Number, err)
			}
			blockStats.LPActiveWalletLogs = lpStats.ActiveWalletLogs
			blockStats.LPEventCount = lpStats.HandledEvents
		}
		if transferEvents := transferEventsByBlock[block.Number]; len(transferEvents) > 0 {
			smartMoneyTransferEvents := filterSmartMoneyTransferEvents(transferEvents, smartMoneyWallets)
			userWalletTransferEvents := expandUserWalletTransferEvents(transferEvents, userWalletRefsByAddress, smartMoneyChainName(int(w.chainID)))
			if len(smartMoneyTransferEvents) > 0 || len(userWalletTransferEvents) > 0 {
				if err := w.repo.WithTx(ctx, func(tx *gorm.DB) error {
					if len(smartMoneyTransferEvents) > 0 {
						if _, err := w.repo.InsertWalletTransferEvents(tx, smartMoneyTransferEvents); err != nil {
							return err
						}
					}
					if len(userWalletTransferEvents) > 0 {
						if _, err := w.repo.InsertUserWalletTransferEvents(tx, userWalletTransferEvents); err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					return pollResult{}, fmt.Errorf("persist block %d transfer events: %w", block.Number, err)
				}
			}
		}

		stats.addBlock(blockStats)
		if block.Number > *lastProcessed {
			*lastProcessed = block.Number
			if err := w.repo.UpsertLPScanState(ctx, int(w.chainID), *lastProcessed); err != nil {
				return pollResult{}, fmt.Errorf("persist scan state block %d: %w", *lastProcessed, err)
			}
		}
	}

	if len(contracts) > 0 {
		walletCount, err := w.finalizeContractInteractions(ctx, contracts, seenWallets, fromBlock, toBlock)
		if err != nil {
			return pollResult{}, err
		}
		stats.ContractWalletCount = walletCount
	}

	log.Printf("[SmartMoney Watcher] scanned blocks=%d-%d latest=%d blocks=%d tx=%d contract_txs=%d contract_wallets=%d lp_logs=%d lp_wallet_logs=%d lp_events=%d elapsed=%s remaining=%d",
		stats.FromBlock, stats.ToBlock, stats.LatestBlock, stats.Blocks, stats.TxCount,
		stats.ContractTxCount, stats.ContractWalletCount, stats.LPLogCount, stats.LPActiveWalletLogs,
		stats.LPEventCount, time.Since(stats.StartedAt).Round(time.Millisecond), stats.remaining())
	return pollResult{Remaining: stats.remaining()}, nil
}

func (w *Watcher) getLatestHTTPBlock(ctx context.Context) (uint64, error) {
	eff, err := resolveSmartMoneyRPC(ctx, rpcpool.TransportHTTP)
	if err != nil {
		return 0, err
	}
	client, err := ethclient.Dial(eff.URL)
	if err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return 0, err
	}
	defer client.Close()

	blockNum, err := client.BlockNumber(ctx)
	if err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return 0, err
	}
	return blockNum, nil
}

func (w *Watcher) getLatestHTTPBlockWithClient(ctx context.Context, client *ethclient.Client, eff rpcpool.Effective) (uint64, error) {
	blockNum, err := client.BlockNumber(ctx)
	if err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return 0, err
	}
	return blockNum, nil
}

func (w *Watcher) openHTTPRPC(ctx context.Context) (*rpc.Client, *ethclient.Client, rpcpool.Effective, error) {
	eff, err := resolveSmartMoneyRPC(ctx, rpcpool.TransportHTTP)
	if err != nil {
		return nil, nil, eff, err
	}
	rpcClient, err := rpc.DialContext(ctx, eff.URL)
	if err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return nil, nil, eff, err
	}
	return rpcClient, ethclient.NewClient(rpcClient), eff, nil
}

func (w *Watcher) loadBlockSnapshots(ctx context.Context, rpcClient *rpc.Client, eff rpcpool.Effective, fromBlock uint64, toBlock uint64) ([]*blockSnapshot, error) {
	if toBlock < fromBlock {
		return nil, nil
	}

	count := int(toBlock - fromBlock + 1)
	raws := make([]rawBlockSnapshot, count)
	batch := make([]rpc.BatchElem, count)
	for i := 0; i < count; i++ {
		blockNum := fromBlock + uint64(i)
		batch[i] = rpc.BatchElem{
			Method: "eth_getBlockByNumber",
			Args:   []interface{}{fmt.Sprintf("0x%x", blockNum), true},
			Result: &raws[i],
		}
	}

	if err := rpcClient.BatchCallContext(ctx, batch); err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return w.loadBlockSnapshotsSequential(ctx, rpcClient, eff, fromBlock, toBlock)
	}

	snapshots := make([]*blockSnapshot, 0, count)
	for i := 0; i < count; i++ {
		if batch[i].Error != nil {
			handleSmartMoneyRPCEndpointError(eff, batch[i].Error)
			return w.loadBlockSnapshotsSequential(ctx, rpcClient, eff, fromBlock, toBlock)
		}

		blockNum := fromBlock + uint64(i)
		snapshot, err := decodeRawBlockSnapshot(blockNum, raws[i])
		if err != nil {
			return nil, fmt.Errorf("decode block %d: %w", blockNum, err)
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func (w *Watcher) loadBlockSnapshotsSequential(ctx context.Context, rpcClient *rpc.Client, eff rpcpool.Effective, fromBlock uint64, toBlock uint64) ([]*blockSnapshot, error) {
	snapshots := make([]*blockSnapshot, 0, int(toBlock-fromBlock+1))
	for blockNum := fromBlock; blockNum <= toBlock; blockNum++ {
		snapshot, err := w.loadBlockSnapshot(ctx, rpcClient, eff, blockNum)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func (w *Watcher) loadBlockSnapshot(ctx context.Context, rpcClient *rpc.Client, eff rpcpool.Effective, blockNum uint64) (*blockSnapshot, error) {
	var raw rawBlockSnapshot
	if err := rpcClient.CallContext(ctx, &raw, "eth_getBlockByNumber", fmt.Sprintf("0x%x", blockNum), true); err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return nil, err
	}
	return decodeRawBlockSnapshot(blockNum, raw)
}

func decodeRawBlockSnapshot(blockNum uint64, raw rawBlockSnapshot) (*blockSnapshot, error) {
	timestampNum, err := parseHexUint64(raw.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("decode block timestamp: %w", err)
	}

	snapshot := &blockSnapshot{
		Number:       blockNum,
		Timestamp:    time.Unix(int64(timestampNum), 0),
		Transactions: make([]blockTransaction, 0, len(raw.Transactions)),
		TxSenders:    make(map[common.Hash]string, len(raw.Transactions)),
	}

	for _, tx := range raw.Transactions {
		hashHex := strings.TrimSpace(tx.Hash)
		from := normalizeHexAddress(tx.From)
		if hashHex == "" || from == "" {
			continue
		}

		var to string
		if tx.To != nil {
			to = normalizeHexAddress(*tx.To)
		}
		value := "0"
		if amount, err := parseHexBigInt(tx.Value); err == nil && amount != nil && amount.Sign() > 0 {
			value = amount.String()
		}
		hash := common.HexToHash(hashHex)
		snapshot.Transactions = append(snapshot.Transactions, blockTransaction{
			Hash:  hash,
			From:  from,
			To:    to,
			Value: value,
		})
		snapshot.TxSenders[hash] = from
	}
	return snapshot, nil
}

func (w *Watcher) processContractInteractionsBlock(ctx context.Context, block *blockSnapshot, contractByAddr map[string]models.WatchContract, seenWallets map[string]map[string]struct{}) (contractInteractionStats, []string, error) {
	stats := contractInteractionStats{}
	newWallets := make([]string, 0)
	if len(contractByAddr) == 0 {
		return stats, newWallets, nil
	}

	for _, tx := range block.Transactions {
		if tx.To == "" {
			continue
		}

		contractAddr := tx.To
		contract, ok := contractByAddr[contractAddr]
		if !ok {
			continue
		}
		stats.MatchedTxCount++

		sender := tx.From
		if sender == "" || sender == "0x0000000000000000000000000000000000000000" {
			continue
		}

		sourceContract := contractAddr
		wallet := &models.MonitoredWallet{
			Address:        sender,
			ChainID:        contract.ChainID,
			Source:         "contract_interaction",
			SourceContract: &sourceContract,
			IsActive:       true,
		}
		if err := w.repo.UpsertMonitoredWallet(ctx, wallet); err != nil {
			log.Printf("[SmartMoney Watcher] upsert wallet %s error: %v", sender, err)
			continue
		}
		newWallets = append(newWallets, sender)

		if seenWallets[contractAddr] == nil {
			seenWallets[contractAddr] = make(map[string]struct{})
		}
		seenWallets[contractAddr][sender] = struct{}{}
	}

	return stats, newWallets, nil
}

func (w *Watcher) finalizeContractInteractions(ctx context.Context, contracts []models.WatchContract, seenWallets map[string]map[string]struct{}, fromBlock uint64, toBlock uint64) (int, error) {
	totalWallets := 0
	for _, contract := range contracts {
		if contract.LastScannedBlock < toBlock {
			if err := w.repo.UpdateWatchContractLastBlock(ctx, contract.ID, toBlock); err != nil {
				return totalWallets, fmt.Errorf("update watch contract %s last block: %w", contract.ContractAddress, err)
			}
		}

		if wallets := seenWallets[strings.ToLower(contract.ContractAddress)]; len(wallets) > 0 {
			totalWallets += len(wallets)
			log.Printf("[SmartMoney Watcher] contract interaction: contract=%s blocks=%d-%d wallets=%d",
				shortAddr(contract.ContractAddress), fromBlock, toBlock, len(wallets))
		}
	}
	return totalWallets, nil
}

func (w *Watcher) loadLPLogsByBlock(ctx context.Context, client *ethclient.Client, eff rpcpool.Effective, fromBlock uint64, toBlock uint64) (map[uint64][]types.Log, error) {
	logsByBlock := make(map[uint64][]types.Log)
	if len(w.lpContracts) == 0 {
		return logsByBlock, nil
	}

	query := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: w.lpContracts,
		Topics:    w.lpTopics,
	}

	logs, err := client.FilterLogs(ctx, query)
	if err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return nil, err
	}
	for _, vlog := range logs {
		logsByBlock[vlog.BlockNumber] = append(logsByBlock[vlog.BlockNumber], vlog)
	}
	return logsByBlock, nil
}

func collectActiveWalletLPTxHashes(blocks []*blockSnapshot, logsByBlock map[uint64][]types.Log, activeWallets map[string]struct{}) map[uint64]map[string]struct{} {
	out := make(map[uint64]map[string]struct{})
	if len(blocks) == 0 || len(logsByBlock) == 0 || len(activeWallets) == 0 {
		return out
	}

	blockByNumber := make(map[uint64]*blockSnapshot, len(blocks))
	for _, block := range blocks {
		if block == nil {
			continue
		}
		blockByNumber[block.Number] = block
	}

	for blockNumber, logs := range logsByBlock {
		block := blockByNumber[blockNumber]
		if block == nil {
			continue
		}
		for _, vlog := range logs {
			sender := block.TxSenders[vlog.TxHash]
			if sender == "" {
				continue
			}
			if _, ok := activeWallets[sender]; !ok {
				continue
			}
			if out[blockNumber] == nil {
				out[blockNumber] = make(map[string]struct{})
			}
			out[blockNumber][strings.ToLower(vlog.TxHash.Hex())] = struct{}{}
		}
	}
	return out
}

func isExcludedWalletTransferTx(excluded map[uint64]map[string]struct{}, blockNumber uint64, txHash string) bool {
	if len(excluded) == 0 || strings.TrimSpace(txHash) == "" {
		return false
	}
	blockSet := excluded[blockNumber]
	if len(blockSet) == 0 {
		return false
	}
	_, ok := blockSet[strings.ToLower(strings.TrimSpace(txHash))]
	return ok
}

func (w *Watcher) loadWalletTransferEventsByBlock(ctx context.Context, client *ethclient.Client, eff rpcpool.Effective, blocks []*blockSnapshot, activeWallets map[string]struct{}, excludedLPTxHashesByBlock map[uint64]map[string]struct{}) (map[uint64][]*models.SmartMoneyWalletTransferEvent, error) {
	out := make(map[uint64][]*models.SmartMoneyWalletTransferEvent)
	if len(blocks) == 0 || len(activeWallets) == 0 {
		return out, nil
	}

	for blockNumber, events := range buildNativeTransferEventsByBlock(blocks, int(w.chainID), activeWallets, excludedLPTxHashesByBlock) {
		out[blockNumber] = append(out[blockNumber], events...)
	}

	erc20ByBlock, err := w.loadERC20TransferEventsByBlock(ctx, client, eff, blocks, activeWallets, excludedLPTxHashesByBlock)
	if err != nil {
		return nil, err
	}
	for blockNumber, events := range erc20ByBlock {
		out[blockNumber] = append(out[blockNumber], events...)
	}
	return out, nil
}

func filterSmartMoneyTransferEvents(events []*models.SmartMoneyWalletTransferEvent, activeWallets map[string]struct{}) []*models.SmartMoneyWalletTransferEvent {
	if len(events) == 0 || len(activeWallets) == 0 {
		return nil
	}

	out := make([]*models.SmartMoneyWalletTransferEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		addr := strings.ToLower(strings.TrimSpace(event.WalletAddress))
		if _, ok := activeWallets[addr]; !ok {
			continue
		}
		out = append(out, event)
	}
	return out
}

func expandUserWalletTransferEvents(events []*models.SmartMoneyWalletTransferEvent, walletRefsByAddress map[string][]UserWalletRef, chain string) []*models.UserWalletTransferEvent {
	if len(events) == 0 || len(walletRefsByAddress) == 0 {
		return nil
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	out := make([]*models.UserWalletTransferEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		addr := strings.ToLower(strings.TrimSpace(event.WalletAddress))
		refs := walletRefsByAddress[addr]
		if len(refs) == 0 {
			continue
		}
		for _, ref := range refs {
			out = append(out, &models.UserWalletTransferEvent{
				UserID:        ref.UserID,
				WalletID:      ref.WalletID,
				WalletAddress: ref.WalletAddress,
				Chain:         chain,
				Direction:     event.Direction,
				AssetType:     event.AssetType,
				TokenAddress:  event.TokenAddress,
				TokenSymbol:   event.TokenSymbol,
				TokenDecimals: event.TokenDecimals,
				AmountRaw:     event.AmountRaw,
				AmountDecimal: event.AmountDecimal,
				AmountUSD:     event.AmountUSD,
				TxHash:        event.TxHash,
				BlockNumber:   event.BlockNumber,
				LogIndex:      event.LogIndex,
				TxTimestamp:   event.TxTimestamp,
			})
		}
	}
	return out
}

func buildNativeTransferEventsByBlock(blocks []*blockSnapshot, chainID int, activeWallets map[string]struct{}, excludedLPTxHashesByBlock map[uint64]map[string]struct{}) map[uint64][]*models.SmartMoneyWalletTransferEvent {
	out := make(map[uint64][]*models.SmartMoneyWalletTransferEvent)
	if len(blocks) == 0 || len(activeWallets) == 0 {
		return out
	}

	chain := smartMoneyChainName(chainID)
	priceUSD := pricing.GetNativePriceUSD(chain)
	tokenAddress := ""
	tokenSymbol := ""
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			tokenAddress = strings.ToLower(strings.TrimSpace(cc.WrappedNativeAddress))
			tokenSymbol = strings.TrimSpace(cc.WrappedNativeSymbol)
		}
	}

	for _, block := range blocks {
		if block == nil {
			continue
		}
		for _, tx := range block.Transactions {
			if tx.Value == "" || tx.Value == "0" {
				continue
			}
			txHash := strings.ToLower(tx.Hash.Hex())
			if isExcludedWalletTransferTx(excludedLPTxHashesByBlock, block.Number, txHash) {
				continue
			}
			if tx.From != "" && tx.From == tx.To {
				continue
			}

			amountDecimal := weiStringToFloat(tx.Value, 18)
			if amountDecimal <= 0 {
				continue
			}
			amountUSD := amountDecimal * priceUSD

			if _, ok := activeWallets[tx.From]; ok {
				out[block.Number] = append(out[block.Number], &models.SmartMoneyWalletTransferEvent{
					WalletAddress: tx.From,
					ChainID:       chainID,
					Direction:     models.SmartMoneyTransferDirectionOut,
					AssetType:     models.SmartMoneyTransferAssetNative,
					TokenAddress:  tokenAddress,
					TokenSymbol:   tokenSymbol,
					TokenDecimals: 18,
					AmountRaw:     tx.Value,
					AmountDecimal: amountDecimal,
					AmountUSD:     amountUSD,
					TxHash:        txHash,
					BlockNumber:   block.Number,
					LogIndex:      -1,
					TxTimestamp:   block.Timestamp,
				})
			}
			if tx.To != "" {
				if _, ok := activeWallets[tx.To]; ok {
					out[block.Number] = append(out[block.Number], &models.SmartMoneyWalletTransferEvent{
						WalletAddress: tx.To,
						ChainID:       chainID,
						Direction:     models.SmartMoneyTransferDirectionIn,
						AssetType:     models.SmartMoneyTransferAssetNative,
						TokenAddress:  tokenAddress,
						TokenSymbol:   tokenSymbol,
						TokenDecimals: 18,
						AmountRaw:     tx.Value,
						AmountDecimal: amountDecimal,
						AmountUSD:     amountUSD,
						TxHash:        txHash,
						BlockNumber:   block.Number,
						LogIndex:      -1,
						TxTimestamp:   block.Timestamp,
					})
				}
			}
		}
	}
	return out
}

func (w *Watcher) loadERC20TransferEventsByBlock(ctx context.Context, client *ethclient.Client, eff rpcpool.Effective, blocks []*blockSnapshot, activeWallets map[string]struct{}, excludedLPTxHashesByBlock map[uint64]map[string]struct{}) (map[uint64][]*models.SmartMoneyWalletTransferEvent, error) {
	out := make(map[uint64][]*models.SmartMoneyWalletTransferEvent)
	if len(blocks) == 0 || len(activeWallets) == 0 {
		return out, nil
	}

	blockTimeByNumber := make(map[uint64]time.Time, len(blocks))
	fromBlock := blocks[0].Number
	toBlock := blocks[len(blocks)-1].Number
	walletAddresses := make([]string, 0, len(activeWallets))
	for addr := range activeWallets {
		if strings.TrimSpace(addr) == "" {
			continue
		}
		walletAddresses = append(walletAddresses, addr)
	}
	sort.Strings(walletAddresses)
	if len(walletAddresses) == 0 {
		return out, nil
	}
	for _, block := range blocks {
		if block == nil {
			continue
		}
		blockTimeByNumber[block.Number] = block.Timestamp
	}

	walletTopics := make([]common.Hash, 0, len(walletAddresses))
	for _, addr := range walletAddresses {
		walletTopics = append(walletTopics, common.BytesToHash(common.HexToAddress(addr).Bytes()))
	}

	candidates := make([]*models.SmartMoneyWalletTransferEvent, 0)
	for start := 0; start < len(walletTopics); start += smartMoneyTransferWalletChunkSize {
		end := start + smartMoneyTransferWalletChunkSize
		if end > len(walletTopics) {
			end = len(walletTopics)
		}
		walletChunk := walletTopics[start:end]

		outLogs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(fromBlock),
			ToBlock:   new(big.Int).SetUint64(toBlock),
			Topics:    [][]common.Hash{{TopicTransfer}, walletChunk},
		})
		if err != nil {
			handleSmartMoneyRPCEndpointError(eff, err)
			return nil, err
		}
		candidates = append(candidates, buildERC20TransferEventsFromLogs(outLogs, int(w.chainID), models.SmartMoneyTransferDirectionOut, activeWallets, blockTimeByNumber, excludedLPTxHashesByBlock)...)

		inLogs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(fromBlock),
			ToBlock:   new(big.Int).SetUint64(toBlock),
			Topics:    [][]common.Hash{{TopicTransfer}, nil, walletChunk},
		})
		if err != nil {
			handleSmartMoneyRPCEndpointError(eff, err)
			return nil, err
		}
		candidates = append(candidates, buildERC20TransferEventsFromLogs(inLogs, int(w.chainID), models.SmartMoneyTransferDirectionIn, activeWallets, blockTimeByNumber, excludedLPTxHashesByBlock)...)
	}

	enrichERC20TransferEvents(ctx, client, int(w.chainID), candidates)
	for _, event := range candidates {
		if event == nil {
			continue
		}
		out[event.BlockNumber] = append(out[event.BlockNumber], event)
	}
	return out, nil
}

func buildERC20TransferEventsFromLogs(logs []types.Log, chainID int, direction string, activeWallets map[string]struct{}, blockTimeByNumber map[uint64]time.Time, excludedLPTxHashesByBlock map[uint64]map[string]struct{}) []*models.SmartMoneyWalletTransferEvent {
	out := make([]*models.SmartMoneyWalletTransferEvent, 0, len(logs))
	for _, vlog := range logs {
		if len(vlog.Topics) < 3 || len(vlog.Data) == 0 {
			continue
		}
		txHash := strings.ToLower(vlog.TxHash.Hex())
		if isExcludedWalletTransferTx(excludedLPTxHashesByBlock, vlog.BlockNumber, txHash) {
			continue
		}

		from := normalizeHexAddress(common.BytesToAddress(vlog.Topics[1].Bytes()).Hex())
		to := normalizeHexAddress(common.BytesToAddress(vlog.Topics[2].Bytes()).Hex())
		if from == "" && to == "" {
			continue
		}
		if from != "" && from == to {
			continue
		}

		walletAddress := from
		if direction == models.SmartMoneyTransferDirectionIn {
			walletAddress = to
		}
		if _, ok := activeWallets[walletAddress]; !ok {
			continue
		}

		amountRaw := new(big.Int).SetBytes(vlog.Data).String()
		if amountRaw == "" || amountRaw == "0" {
			continue
		}
		tokenAddress := strings.ToLower(strings.TrimSpace(vlog.Address.Hex()))

		out = append(out, &models.SmartMoneyWalletTransferEvent{
			WalletAddress: walletAddress,
			ChainID:       chainID,
			Direction:     direction,
			AssetType:     models.SmartMoneyTransferAssetERC20,
			TokenAddress:  tokenAddress,
			AmountRaw:     amountRaw,
			TxHash:        txHash,
			BlockNumber:   vlog.BlockNumber,
			LogIndex:      int(vlog.Index),
			TxTimestamp:   blockTimeByNumber[vlog.BlockNumber],
		})
	}
	return out
}

func enrichERC20TransferEvents(ctx context.Context, client *ethclient.Client, chainID int, events []*models.SmartMoneyWalletTransferEvent) {
	if len(events) == 0 || client == nil {
		return
	}

	chain := smartMoneyChainName(chainID)
	network := smartMoneyChainSlugForPricing(chainID)
	tokenAddresses := make([]string, 0)
	seen := make(map[string]struct{})
	for _, event := range events {
		if event == nil || event.AssetType != models.SmartMoneyTransferAssetERC20 {
			continue
		}
		addr := strings.ToLower(strings.TrimSpace(event.TokenAddress))
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		tokenAddresses = append(tokenAddresses, addr)
	}
	if len(tokenAddresses) == 0 {
		return
	}

	prices, err := smTokenPriceService.GetUSDPrices(network, tokenAddresses)
	if err != nil {
		log.Printf("[SmartMoney Watcher] erc20 transfer price lookup failed chain=%s err=%v", network, err)
	}

	wrappedNativeAddr := ""
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			wrappedNativeAddr = strings.ToLower(strings.TrimSpace(cc.WrappedNativeAddress))
		}
	}
	nativePriceUSD := pricing.GetNativePriceUSD(chain)

	metaByToken := make(map[string]transferTokenMeta, len(tokenAddresses))
	for _, tokenAddress := range tokenAddresses {
		meta := transferTokenMeta{
			address:  tokenAddress,
			decimals: readTokenDecimalsWithClient(client, tokenAddress),
			priceUSD: prices[tokenAddress],
		}
		if wrappedNativeAddr != "" && tokenAddress == wrappedNativeAddr && meta.priceUSD <= 0 {
			meta.priceUSD = nativePriceUSD
		}
		if symbol, err := blockchain.GetTokenSymbolWithClient(client, common.HexToAddress(tokenAddress)); err == nil {
			meta.symbol = strings.TrimSpace(symbol)
		}
		metaByToken[tokenAddress] = meta
	}

	for _, event := range events {
		if event == nil || event.AssetType != models.SmartMoneyTransferAssetERC20 {
			continue
		}
		meta := metaByToken[strings.ToLower(strings.TrimSpace(event.TokenAddress))]
		if meta.decimals <= 0 {
			meta.decimals = 18
		}
		event.TokenDecimals = meta.decimals
		event.TokenSymbol = meta.symbol
		event.AmountDecimal = weiStringToFloat(event.AmountRaw, meta.decimals)
		event.AmountUSD = event.AmountDecimal * meta.priceUSD
	}
}

func (w *Watcher) processLPLogsForBlock(ctx context.Context, block *blockSnapshot, logs []types.Log, activeWallets map[string]struct{}) (lpLogStats, error) {
	stats := lpLogStats{
		TotalLogs: len(logs),
	}
	if len(logs) == 0 || len(activeWallets) == 0 {
		return stats, nil
	}

	blockTime := block.Timestamp
	events := make([]*models.SmartMoneyLPEvent, 0, len(logs))
	for _, vlog := range logs {
		sender, ok := block.TxSenders[vlog.TxHash]
		if !ok || sender == "" {
			log.Printf("[SmartMoney Watcher] tx sender missing in block snapshot: tx=%s", shortAddr(vlog.TxHash.Hex()))
			continue
		}
		if _, ok := activeWallets[sender]; !ok {
			continue
		}
		stats.ActiveWalletLogs++

		event, err := w.buildLPEvent(vlog, sender, block.Number, blockTime)
		if err != nil {
			return stats, fmt.Errorf("parse lp log tx=%s log_index=%d: %w", vlog.TxHash.Hex(), vlog.Index, err)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return stats, nil
	}

	groups := groupLPEventsByPosition(events)
	handled, err := w.processLPEventGroups(ctx, groups)
	stats.HandledEvents = handled
	if err != nil {
		return stats, err
	}
	return stats, nil
}

func (s *watcherScanStats) addBlock(blockStats blockProcessStats) {
	if s == nil {
		return
	}
	s.TxCount += blockStats.TxCount
	s.ContractTxCount += blockStats.ContractTxCount
	s.ContractWalletCount += blockStats.ContractWalletCount
	s.LPLogCount += blockStats.LPLogCount
	s.LPActiveWalletLogs += blockStats.LPActiveWalletLogs
	s.LPEventCount += blockStats.LPEventCount
}

func (s watcherScanStats) remaining() uint64 {
	if s.LatestBlock <= s.ToBlock {
		return 0
	}
	return s.LatestBlock - s.ToBlock
}

func smartMoneyPositionGroupKey(event *models.SmartMoneyLPEvent) string {
	if event == nil {
		return ""
	}
	if positionRef := BuildPositionRefFromEvent(event); positionRef != "" {
		return positionRef
	}
	return fmt.Sprintf("%d:%s:%d", event.BlockNumber, strings.ToLower(strings.TrimSpace(event.TxHash)), event.LogIndex)
}

func groupLPEventsByPosition(events []*models.SmartMoneyLPEvent) [][]*models.SmartMoneyLPEvent {
	if len(events) == 0 {
		return nil
	}

	byKey := make(map[string][]*models.SmartMoneyLPEvent, len(events))
	order := make([]string, 0, len(events))
	for _, event := range events {
		key := smartMoneyPositionGroupKey(event)
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], event)
	}

	out := make([][]*models.SmartMoneyLPEvent, 0, len(order))
	for _, key := range order {
		group := byKey[key]
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].BlockNumber != group[j].BlockNumber {
				return group[i].BlockNumber < group[j].BlockNumber
			}
			return group[i].LogIndex < group[j].LogIndex
		})
		out = append(out, group)
	}
	return out
}

func (w *Watcher) buildLPEvent(vlog types.Log, sender string, blockNum uint64, blockTime time.Time) (*models.SmartMoneyLPEvent, error) {
	event, err := w.parseLog(vlog)
	if err != nil {
		return nil, err
	}
	event.WalletAddress = sender
	event.ChainID = int(w.chainID)
	event.BlockNumber = blockNum
	event.TxHash = vlog.TxHash.Hex()
	event.LogIndex = int(vlog.Index)
	event.TxTimestamp = blockTime
	return event, nil
}

func (w *Watcher) processLPEventGroups(ctx context.Context, groups [][]*models.SmartMoneyLPEvent) (int, error) {
	if len(groups) == 0 {
		return 0, nil
	}

	workers := w.eventWorkers
	if workers <= 1 || len(groups) == 1 {
		handled := 0
		for _, group := range groups {
			if err := w.processLPEventGroup(ctx, group); err != nil {
				return handled, err
			}
			handled += len(group)
		}
		return handled, nil
	}
	if workers > len(groups) {
		workers = len(groups)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan []*models.SmartMoneyLPEvent)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		handled  int
		firstErr error
	)

	workerFn := func() {
		defer wg.Done()
		for group := range jobs {
			if runCtx.Err() != nil {
				return
			}
			if err := w.processLPEventGroup(runCtx, group); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			handled += len(group)
			mu.Unlock()
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go workerFn()
	}
	for _, group := range groups {
		if runCtx.Err() != nil {
			break
		}
		jobs <- group
	}
	close(jobs)
	wg.Wait()

	return handled, firstErr
}

func (w *Watcher) processLPEventGroup(ctx context.Context, group []*models.SmartMoneyLPEvent) error {
	for _, event := range group {
		if ctx.Err() != nil {
			return nil
		}
		if err := w.handleParsedLPEvent(ctx, event); err != nil {
			return fmt.Errorf("handle lp event tx=%s log_index=%d: %w", event.TxHash, event.LogIndex, err)
		}
	}
	return nil
}

func (w *Watcher) handleParsedLPEvent(ctx context.Context, event *models.SmartMoneyLPEvent) error {
	if event == nil {
		return nil
	}
	if err := EnrichLPEvent(ctx, event); err != nil {
		log.Printf("[SmartMoney Watcher] enrich LP event metadata failed: protocol=%s nft=%v tx=%s err=%v",
			event.Protocol, event.NftTokenID, shortAddr(event.TxHash), err)
	}

	// If amounts are 0 (V4 ModifyLiquidity doesn't include amounts),
	// resolve from Transfer events in the same tx receipt.
	if event.Token0Amount == "0" && event.Token1Amount == "0" &&
		event.Token0Address != "" && event.Token1Address != "" {
		w.resolveAmountsFromReceipt(ctx, event)
	}

	// Compute USD amounts via OKX/Gecko real prices
	ComputeEventAmountUSD(ctx, event)

	inserted := false
	if err := w.repo.WithTx(ctx, func(tx *gorm.DB) error {
		var err error
		inserted, err = w.repo.InsertLPEvent(tx, event)
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}
		return w.repo.UpsertLPPosition(tx, event)
	}); err != nil {
		return err
	}
	if !inserted {
		return nil
	}

	log.Printf("[SmartMoney Watcher] %s LP event: wallet=%s pool=%s type=%s nft=%v tx=%s",
		event.Protocol, shortAddr(event.WalletAddress), shortAddr(event.PoolAddress), event.EventType,
		event.NftTokenID, shortAddr(event.TxHash))

	if w.notifier != nil {
		w.notifier(event)
	}
	return nil
}

// resolveAmountsFromReceipt fetches the tx receipt and extracts token
// amounts from ERC-20 Transfer events for protocols (like V4) whose
// LP event doesn't include amounts directly.
func (w *Watcher) resolveAmountsFromReceipt(ctx context.Context, event *models.SmartMoneyLPEvent) {
	if event == nil || event.TxHash == "" {
		return
	}
	chain := smartMoneyChainName(event.ChainID)
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	receipt, err := client.TransactionReceipt(callCtx, common.HexToHash(event.TxHash))
	if err != nil || receipt == nil {
		log.Printf("[SmartMoney Watcher] resolveAmountsFromReceipt: fetch receipt failed tx=%s: %v",
			shortAddr(event.TxHash), err)
		return
	}

	topicTransfer := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	token0 := common.HexToAddress(event.Token0Address)
	token1 := common.HexToAddress(event.Token1Address)

	// For V4, the pool_address is a poolId hash, not a contract.
	// Tokens flow through the LP contract addresses (NPM / PoolManager).
	targets := make(map[common.Address]struct{}, len(w.lpContracts)+1)
	for _, c := range w.lpContracts {
		targets[c] = struct{}{}
	}
	poolAddr := common.HexToAddress(event.PoolAddress)
	targets[poolAddr] = struct{}{}

	var sum0, sum1 big.Int
	for _, vlog := range receipt.Logs {
		if len(vlog.Topics) < 3 || vlog.Topics[0] != topicTransfer {
			continue
		}
		if len(vlog.Data) < 32 {
			continue
		}
		tokenAddr := vlog.Address
		if tokenAddr != token0 && tokenAddr != token1 {
			continue
		}
		amount := new(big.Int).SetBytes(vlog.Data[:32])
		if amount.Sign() <= 0 {
			continue
		}

		from := common.BytesToAddress(vlog.Topics[1].Bytes())
		to := common.BytesToAddress(vlog.Topics[2].Bytes())

		// Match transfers directly involving LP infrastructure contracts
		var match bool
		if event.EventType == "add" {
			_, match = targets[to] // tokens flowing INTO pool/manager
		} else {
			_, match = targets[from] // tokens flowing OUT of pool/manager
		}
		if !match {
			continue
		}

		if tokenAddr == token0 {
			sum0.Add(&sum0, amount)
		} else {
			sum1.Add(&sum1, amount)
		}
	}

	if sum0.Sign() > 0 {
		event.Token0Amount = sum0.String()
	}
	if sum1.Sign() > 0 {
		event.Token1Amount = sum1.String()
	}
}

func (w *Watcher) parseLog(vlog types.Log) (*models.SmartMoneyLPEvent, error) {
	if len(vlog.Topics) == 0 {
		return nil, fmt.Errorf("no topics")
	}

	switch vlog.Topics[0] {
	case TopicIncreaseLiquidity:
		return w.parseIncreaseLiquidity(vlog)
	case TopicDecreaseLiquidity:
		return w.parseDecreaseLiquidity(vlog)
	case TopicModifyLiquidity:
		return w.parseModifyLiquidity(vlog)
	}
	return nil, fmt.Errorf("unknown topic: %s", vlog.Topics[0].Hex())
}

func (w *Watcher) parseIncreaseLiquidity(vlog types.Log) (*models.SmartMoneyLPEvent, error) {
	if len(vlog.Topics) < 2 {
		return nil, fmt.Errorf("IncreaseLiquidity: insufficient topics")
	}
	tokenID := new(big.Int).SetBytes(vlog.Topics[1].Bytes())
	nftID := tokenID.Uint64()

	liquidityDeltaStr := "0"
	amount0Str := "0"
	amount1Str := "0"
	if len(vlog.Data) >= 32 {
		liquidityDelta := new(big.Int).SetBytes(vlog.Data[0:32])
		liquidityDeltaStr = liquidityDelta.String()
	}
	if len(vlog.Data) >= 96 {
		amount0 := new(big.Int).SetBytes(vlog.Data[32:64])
		amount1 := new(big.Int).SetBytes(vlog.Data[64:96])
		amount0Str = amount0.String()
		amount1Str = amount1.String()
	}

	protocol := detectProtocol(vlog.Address)
	return &models.SmartMoneyLPEvent{
		Protocol:       protocol,
		EventType:      "add",
		NftTokenID:     &nftID,
		PoolAddress:    strings.ToLower(vlog.Address.Hex()),
		LiquidityDelta: liquidityDeltaStr,
		Token0Amount:   amount0Str,
		Token1Amount:   amount1Str,
	}, nil
}

func (w *Watcher) parseDecreaseLiquidity(vlog types.Log) (*models.SmartMoneyLPEvent, error) {
	if len(vlog.Topics) < 2 {
		return nil, fmt.Errorf("DecreaseLiquidity: insufficient topics")
	}
	tokenID := new(big.Int).SetBytes(vlog.Topics[1].Bytes())
	nftID := tokenID.Uint64()

	liquidityDeltaStr := "0"
	amount0Str := "0"
	amount1Str := "0"
	if len(vlog.Data) >= 32 {
		liquidityDelta := new(big.Int).SetBytes(vlog.Data[0:32])
		liquidityDelta.Neg(liquidityDelta)
		liquidityDeltaStr = liquidityDelta.String()
	}
	if len(vlog.Data) >= 96 {
		amount0 := new(big.Int).SetBytes(vlog.Data[32:64])
		amount1 := new(big.Int).SetBytes(vlog.Data[64:96])
		amount0Str = amount0.String()
		amount1Str = amount1.String()
	}

	protocol := detectProtocol(vlog.Address)
	return &models.SmartMoneyLPEvent{
		Protocol:       protocol,
		EventType:      "remove",
		NftTokenID:     &nftID,
		PoolAddress:    strings.ToLower(vlog.Address.Hex()),
		LiquidityDelta: liquidityDeltaStr,
		Token0Amount:   amount0Str,
		Token1Amount:   amount1Str,
	}, nil
}

func (w *Watcher) parseModifyLiquidity(vlog types.Log) (*models.SmartMoneyLPEvent, error) {
	if len(vlog.Data) < 128 {
		return nil, fmt.Errorf("ModifyLiquidity: insufficient data")
	}

	tickLower, err := decodeSignedInt24Word(vlog.Data[0:32])
	if err != nil {
		return nil, fmt.Errorf("ModifyLiquidity: decode tickLower: %w", err)
	}
	tickUpper, err := decodeSignedInt24Word(vlog.Data[32:64])
	if err != nil {
		return nil, fmt.Errorf("ModifyLiquidity: decode tickUpper: %w", err)
	}
	liquidityDelta := new(big.Int).SetBytes(vlog.Data[64:96])
	salt := new(big.Int).SetBytes(vlog.Data[96:128])

	maxInt256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1))
	if liquidityDelta.Cmp(maxInt256) > 0 {
		liquidityDelta.Sub(liquidityDelta, new(big.Int).Lsh(big.NewInt(1), 256))
	}

	eventType := "add"
	if liquidityDelta.Sign() < 0 {
		eventType = "remove"
	}

	var nftID *uint64
	if salt.Sign() > 0 && salt.IsUint64() {
		parsed := salt.Uint64()
		nftID = &parsed
	}

	return &models.SmartMoneyLPEvent{
		Protocol:       "uniswap_v4",
		EventType:      eventType,
		PoolAddress:    strings.ToLower(vlog.Address.Hex()),
		TickLower:      &tickLower,
		TickUpper:      &tickUpper,
		NftTokenID:     nftID,
		LiquidityDelta: liquidityDelta.String(),
		Token0Amount:   "0",
		Token1Amount:   "0",
	}, nil
}

func decodeSignedInt24Word(word []byte) (int, error) {
	if len(word) < 32 {
		return 0, fmt.Errorf("word too short: %d", len(word))
	}
	v := int32(word[29])<<16 | int32(word[30])<<8 | int32(word[31])
	if v&0x800000 != 0 {
		v -= 1 << 24
	}
	return int(v), nil
}

func detectProtocol(addr common.Address) string {
	addrLower := strings.ToLower(addr.Hex())
	switch addrLower {
	case "0x46a15b0b27311cedf172ab29e4f4766fbe7f4364":
		return "pancake_v3"
	case "0x7b8a01b39d58278b5de7e48c8449c9f4f5170613":
		return "uniswap_v3"
	default:
		return "uniswap_v3"
	}
}

func shortAddr(s string) string {
	if len(s) <= 10 {
		return s
	}
	return s[:6] + "..." + s[len(s)-4:]
}

func parseHexUint64(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 16, 64)
}

func parseHexBigInt(value string) (*big.Int, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	if value == "" {
		return big.NewInt(0), nil
	}
	out, ok := new(big.Int).SetString(value, 16)
	if !ok {
		return nil, fmt.Errorf("invalid hex bigint: %s", value)
	}
	return out, nil
}

func normalizeHexAddress(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 42 {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(value), "0x") {
		return ""
	}
	return strings.ToLower(value)
}

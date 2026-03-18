package smart_money

import (
	"TgLpBot/base/models"
	"TgLpBot/base/rpcpool"
	"context"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"time"

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
)

const (
	smartMoneyMaxBlocksPerPoll = 10
	smartMoneyBaseErrorDelay   = 5 * time.Second
	smartMoneyRateLimitDelay   = 15 * time.Second
	smartMoneyMaxRetryDelay    = time.Minute
)

type Watcher struct {
	repo             *Repository
	notifier         func(*models.SmartMoneyLPEvent)
	lpContracts      []common.Address
	lpTopics         [][]common.Hash
	chainID          int64
	pollIntervalSec  int
	maxBlocksPerPoll int
	stopCh           chan struct{}
}

type blockTransaction struct {
	Hash common.Hash
	From string
	To   string
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
		Hash string  `json:"hash"`
		From string  `json:"from"`
		To   *string `json:"to"`
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

func NewWatcher(repo *Repository, chainID int64, pancakeV3NPM, uniswapV3NPM, uniswapV4PM string, pollIntervalSec int) *Watcher {
	var lpContracts []common.Address
	if pancakeV3NPM != "" {
		lpContracts = append(lpContracts, common.HexToAddress(pancakeV3NPM))
	}
	if uniswapV3NPM != "" {
		lpContracts = append(lpContracts, common.HexToAddress(uniswapV3NPM))
	}
	if uniswapV4PM != "" {
		lpContracts = append(lpContracts, common.HexToAddress(uniswapV4PM))
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
		stopCh:           make(chan struct{}),
	}
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
			initialized = true
			rateLimitDelay = smartMoneyRateLimitDelay
			log.Printf("[SmartMoney Watcher] started from block %d, mode=http-polling, interval=%s, lp_contracts=%d",
				startBlock, pollDelay, len(w.lpContracts))
		}

		if err := w.pollOnce(ctx, &lastProcessed); err != nil {
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

func (w *Watcher) pollOnce(ctx context.Context, lastProcessed *uint64) error {
	rpcClient, httpClient, eff, err := w.openHTTPRPC(ctx)
	if err != nil {
		return err
	}
	defer rpcClient.Close()

	latestBlock, err := w.getLatestHTTPBlockWithClient(ctx, httpClient, eff)
	if err != nil {
		return err
	}
	if latestBlock <= *lastProcessed {
		return nil
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
		StartedAt:   time.Now(),
	}
	log.Printf("[SmartMoney Watcher] scanning blocks=%d-%d latest=%d",
		stats.FromBlock, stats.ToBlock, stats.LatestBlock)

	for blockNum := fromBlock; blockNum <= toBlock; blockNum++ {
		blockStats, err := w.processBlockWithClients(ctx, rpcClient, httpClient, eff, blockNum)
		if err != nil {
			return fmt.Errorf("process block %d: %w", blockNum, err)
		}
		stats.addBlock(blockStats)
		*lastProcessed = blockNum
	}
	log.Printf("[SmartMoney Watcher] scanned blocks=%d-%d latest=%d blocks=%d tx=%d contract_txs=%d contract_wallets=%d lp_logs=%d lp_wallet_logs=%d lp_events=%d elapsed=%s remaining=%d",
		stats.FromBlock, stats.ToBlock, stats.LatestBlock, stats.Blocks, stats.TxCount,
		stats.ContractTxCount, stats.ContractWalletCount, stats.LPLogCount, stats.LPActiveWalletLogs,
		stats.LPEventCount, time.Since(stats.StartedAt).Round(time.Millisecond), stats.remaining())
	return nil
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

func (w *Watcher) processBlockWithClients(ctx context.Context, rpcClient *rpc.Client, httpClient *ethclient.Client, eff rpcpool.Effective, blockNum uint64) (blockProcessStats, error) {
	block, err := w.loadBlockSnapshot(ctx, rpcClient, eff, blockNum)
	if err != nil {
		return blockProcessStats{}, err
	}

	stats := blockProcessStats{
		TxCount: len(block.Transactions),
	}

	contractStats, err := w.processContractInteractions(ctx, block, blockNum)
	if err != nil {
		return stats, err
	}
	stats.ContractTxCount = contractStats.MatchedTxCount
	stats.ContractWalletCount = contractStats.WalletCount

	lpStats, err := w.processLPLogs(ctx, httpClient, block, blockNum, eff)
	if err != nil {
		return stats, err
	}
	stats.LPLogCount = lpStats.TotalLogs
	stats.LPActiveWalletLogs = lpStats.ActiveWalletLogs
	stats.LPEventCount = lpStats.HandledEvents

	return stats, nil
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

func (w *Watcher) loadBlockSnapshot(ctx context.Context, rpcClient *rpc.Client, eff rpcpool.Effective, blockNum uint64) (*blockSnapshot, error) {
	var raw rawBlockSnapshot
	if err := rpcClient.CallContext(ctx, &raw, "eth_getBlockByNumber", fmt.Sprintf("0x%x", blockNum), true); err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return nil, err
	}

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
		hash := common.HexToHash(hashHex)
		snapshot.Transactions = append(snapshot.Transactions, blockTransaction{
			Hash: hash,
			From: from,
			To:   to,
		})
		snapshot.TxSenders[hash] = from
	}
	return snapshot, nil
}

func (w *Watcher) processContractInteractions(ctx context.Context, block *blockSnapshot, blockNum uint64) (contractInteractionStats, error) {
	stats := contractInteractionStats{}
	contracts, err := w.repo.GetActiveWatchContractsByChain(ctx, int(w.chainID))
	if err != nil {
		return stats, err
	}
	if len(contracts) == 0 {
		return stats, nil
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
	if len(contractByAddr) == 0 {
		return stats, nil
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

		if seenWallets[contractAddr] == nil {
			seenWallets[contractAddr] = make(map[string]struct{})
		}
		seenWallets[contractAddr][sender] = struct{}{}
	}

	for _, contract := range contracts {
		if contract.LastScannedBlock >= blockNum {
			continue
		}
		if err := w.repo.UpdateWatchContractLastBlock(ctx, contract.ID, blockNum); err != nil {
			return stats, fmt.Errorf("update watch contract %s last block: %w", contract.ContractAddress, err)
		}

		if wallets := seenWallets[strings.ToLower(contract.ContractAddress)]; len(wallets) > 0 {
			stats.WalletCount += len(wallets)
			log.Printf("[SmartMoney Watcher] contract interaction: contract=%s block=%d wallets=%d",
				shortAddr(contract.ContractAddress), blockNum, len(wallets))
		}
	}

	return stats, nil
}

func (w *Watcher) processLPLogs(ctx context.Context, client *ethclient.Client, block *blockSnapshot, blockNum uint64, eff rpcpool.Effective) (lpLogStats, error) {
	stats := lpLogStats{}
	if len(w.lpContracts) == 0 {
		return stats, nil
	}

	query := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(blockNum),
		ToBlock:   new(big.Int).SetUint64(blockNum),
		Addresses: w.lpContracts,
		Topics:    w.lpTopics,
	}

	logs, err := client.FilterLogs(ctx, query)
	if err != nil {
		handleSmartMoneyRPCEndpointError(eff, err)
		return stats, err
	}
	stats.TotalLogs = len(logs)
	if len(logs) == 0 {
		return stats, nil
	}

	activeWallets, err := w.repo.GetAllActiveWalletAddresses(ctx, int(w.chainID))
	if err != nil {
		return stats, err
	}
	if len(activeWallets) == 0 {
		return stats, nil
	}

	blockTime := block.Timestamp
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

		if err := w.handleLPLog(ctx, vlog, sender, blockNum, blockTime); err != nil {
			log.Printf("[SmartMoney Watcher] handle LP log error: tx=%s err=%v", shortAddr(vlog.TxHash.Hex()), err)
			continue
		}
		stats.HandledEvents++
	}

	return stats, nil
}

func (s *watcherScanStats) addBlock(blockStats blockProcessStats) {
	if s == nil {
		return
	}
	s.Blocks++
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

func (w *Watcher) handleLPLog(ctx context.Context, vlog types.Log, sender string, blockNum uint64, blockTime time.Time) error {
	event, err := w.parseLog(vlog)
	if err != nil {
		return err
	}
	event.WalletAddress = sender
	event.ChainID = int(w.chainID)
	event.BlockNumber = blockNum
	event.TxHash = vlog.TxHash.Hex()
	event.LogIndex = int(vlog.Index)
	event.TxTimestamp = blockTime

	if err := w.repo.WithTx(ctx, func(tx *gorm.DB) error {
		if err := w.repo.InsertLPEvent(tx, event); err != nil {
			return err
		}
		return w.repo.UpsertLPPosition(tx, event)
	}); err != nil {
		return err
	}

	log.Printf("[SmartMoney Watcher] %s LP event: wallet=%s pool=%s type=%s nft=%v tx=%s",
		event.Protocol, shortAddr(sender), shortAddr(event.PoolAddress), event.EventType,
		event.NftTokenID, shortAddr(event.TxHash))

	if w.notifier != nil {
		w.notifier(event)
	}
	return nil
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

	var amount0Str, amount1Str string
	if len(vlog.Data) >= 96 {
		amount0 := new(big.Int).SetBytes(vlog.Data[32:64])
		amount1 := new(big.Int).SetBytes(vlog.Data[64:96])
		amount0Str = amount0.String()
		amount1Str = amount1.String()
	}

	protocol := detectProtocol(vlog.Address)
	return &models.SmartMoneyLPEvent{
		Protocol:     protocol,
		EventType:    "add",
		NftTokenID:   &nftID,
		PoolAddress:  strings.ToLower(vlog.Address.Hex()),
		Token0Amount: amount0Str,
		Token1Amount: amount1Str,
	}, nil
}

func (w *Watcher) parseDecreaseLiquidity(vlog types.Log) (*models.SmartMoneyLPEvent, error) {
	if len(vlog.Topics) < 2 {
		return nil, fmt.Errorf("DecreaseLiquidity: insufficient topics")
	}
	tokenID := new(big.Int).SetBytes(vlog.Topics[1].Bytes())
	nftID := tokenID.Uint64()

	var amount0Str, amount1Str string
	if len(vlog.Data) >= 96 {
		amount0 := new(big.Int).SetBytes(vlog.Data[32:64])
		amount1 := new(big.Int).SetBytes(vlog.Data[64:96])
		amount0Str = amount0.String()
		amount1Str = amount1.String()
	}

	protocol := detectProtocol(vlog.Address)
	return &models.SmartMoneyLPEvent{
		Protocol:     protocol,
		EventType:    "remove",
		NftTokenID:   &nftID,
		PoolAddress:  strings.ToLower(vlog.Address.Hex()),
		Token0Amount: amount0Str,
		Token1Amount: amount1Str,
	}, nil
}

func (w *Watcher) parseModifyLiquidity(vlog types.Log) (*models.SmartMoneyLPEvent, error) {
	if len(vlog.Data) < 96 {
		return nil, fmt.Errorf("ModifyLiquidity: insufficient data")
	}

	tickLowerRaw := new(big.Int).SetBytes(vlog.Data[0:32])
	tickUpperRaw := new(big.Int).SetBytes(vlog.Data[32:64])
	liquidityDelta := new(big.Int).SetBytes(vlog.Data[64:96])

	maxInt256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1))
	if liquidityDelta.Cmp(maxInt256) > 0 {
		liquidityDelta.Sub(liquidityDelta, new(big.Int).Lsh(big.NewInt(1), 256))
	}

	if tickLowerRaw.Bit(23) == 1 {
		tickLowerRaw.Sub(tickLowerRaw, new(big.Int).Lsh(big.NewInt(1), 24))
	}
	if tickUpperRaw.Bit(23) == 1 {
		tickUpperRaw.Sub(tickUpperRaw, new(big.Int).Lsh(big.NewInt(1), 24))
	}

	eventType := "add"
	if liquidityDelta.Sign() < 0 {
		eventType = "remove"
	}

	tickLower := int(tickLowerRaw.Int64())
	tickUpper := int(tickUpperRaw.Int64())

	return &models.SmartMoneyLPEvent{
		Protocol:    "uniswap_v4",
		EventType:   eventType,
		PoolAddress: strings.ToLower(vlog.Address.Hex()),
		TickLower:   &tickLower,
		TickUpper:   &tickUpper,
	}, nil
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

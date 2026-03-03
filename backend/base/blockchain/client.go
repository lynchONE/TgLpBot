package blockchain

import (
	"TgLpBot/base/config"
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Legacy single-chain globals (default chain: "bsc"). Avoid using these in multi-chain logic.
var (
	Client  *ethclient.Client
	ChainID *big.Int
)

var (
	evmMu       sync.RWMutex
	evmClients  = make(map[string]*ethclient.Client)
	evmChainIDs = make(map[string]*big.Int)
)

// InitBlockchains initializes per-chain blockchain clients (single-instance multi-chain).
// Enabled chains are loaded from config.AppConfig.EnabledChains / CHAINS env.
func InitBlockchains() error {
	if config.AppConfig == nil {
		return fmt.Errorf("config not loaded")
	}

	enabled := config.AppConfig.EnabledChains
	if len(enabled) == 0 {
		enabled = []string{"bsc"}
	}

	type initResult struct {
		chain   string
		client  *ethclient.Client
		chainID *big.Int
	}

	results := make([]initResult, 0, len(enabled))
	var errs []string

	for _, raw := range enabled {
		chain := config.NormalizeChain(raw)
		cc, ok := config.AppConfig.GetChainConfig(chain)
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: chain config not found", chain))
			continue
		}
		if cc.Kind != config.ChainKindEVM {
			errs = append(errs, fmt.Sprintf("%s: chain kind not supported: %s", chain, cc.Kind))
			continue
		}

		rpcURL := strings.TrimSpace(cc.RpcURL)
		if rpcURL == "" {
			errs = append(errs, fmt.Sprintf("%s: rpc url not configured", chain))
			continue
		}

		log.Printf("Connecting to %s network (chainId=%d): %s", chain, cc.ChainID, rpcURL)
		c, err := ethclient.Dial(rpcURL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: dial failed: %v", chain, err))
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		blockNumber, err := c.BlockNumber(ctx)
		cancel()
		if err != nil {
			c.Close()
			errs = append(errs, fmt.Sprintf("%s: get block number failed: %v", chain, err))
			continue
		}

		log.Printf("%s blockchain connected successfully, current block: %d", chain, blockNumber)
		results = append(results, initResult{
			chain:   chain,
			client:  c,
			chainID: big.NewInt(cc.ChainID),
		})
	}

	evmMu.Lock()
	for _, c := range evmClients {
		if c != nil {
			c.Close()
		}
	}
	evmClients = make(map[string]*ethclient.Client, len(results))
	evmChainIDs = make(map[string]*big.Int, len(results))
	for _, r := range results {
		evmClients[r.chain] = r.client
		if r.chainID == nil {
			evmChainIDs[r.chain] = big.NewInt(0)
		} else {
			evmChainIDs[r.chain] = new(big.Int).Set(r.chainID)
		}
	}

	// Keep legacy globals pointing at bsc when available (backward compatibility).
	Client = nil
	ChainID = nil
	if c, ok := evmClients["bsc"]; ok {
		Client = c
		if id := evmChainIDs["bsc"]; id != nil {
			ChainID = new(big.Int).Set(id)
		}
	} else if len(enabled) > 0 {
		first := config.NormalizeChain(enabled[0])
		if c := evmClients[first]; c != nil {
			Client = c
			if id := evmChainIDs[first]; id != nil {
				ChainID = new(big.Int).Set(id)
			}
		}
	}

	evmMu.Unlock()

	if len(evmClients) == 0 {
		if len(errs) > 0 {
			return fmt.Errorf("init blockchains failed: %s", strings.Join(errs, "; "))
		}
		return fmt.Errorf("no blockchain clients initialized")
	}
	if len(errs) > 0 {
		return fmt.Errorf("init blockchains partial failure: %s", strings.Join(errs, "; "))
	}
	return nil
}

// CloseBlockchains closes all initialized blockchain clients.
func CloseBlockchains() {
	evmMu.Lock()
	for _, c := range evmClients {
		if c != nil {
			c.Close()
		}
	}
	evmClients = make(map[string]*ethclient.Client)
	evmChainIDs = make(map[string]*big.Int)
	Client = nil
	ChainID = nil
	evmMu.Unlock()
}

// GetEVMClient returns the EVM client and chainId for a given chain key (e.g. "bsc", "base").
func GetEVMClient(chain string) (*ethclient.Client, *big.Int, error) {
	chain = config.NormalizeChain(chain)
	evmMu.RLock()
	c := evmClients[chain]
	id := evmChainIDs[chain]
	evmMu.RUnlock()
	if c == nil {
		return nil, nil, fmt.Errorf("evm client not initialized for chain=%s", chain)
	}
	if id == nil {
		id = big.NewInt(0)
	}
	return c, new(big.Int).Set(id), nil
}

// InitBlockchain initializes blockchain clients (backward compatible wrapper).
func InitBlockchain() error { return InitBlockchains() }

// CloseBlockchain closes blockchain clients (backward compatible wrapper).
func CloseBlockchain() { CloseBlockchains() }

// GetBalanceWithClient returns the native balance (wei) of an address.
func GetBalanceWithClient(client *ethclient.Client, address common.Address) (*big.Int, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return client.BalanceAt(context.Background(), address, nil)
}

// GetTokenBalanceWithClient returns the balance (raw units) of a token for an address.
func GetTokenBalanceWithClient(client *ethclient.Client, tokenAddress, walletAddress common.Address) (*big.Int, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	token, err := NewERC20(tokenAddress, client)
	if err != nil {
		return nil, err
	}
	return token.BalanceOf(nil, walletAddress)
}

// GetTokenDecimalsWithClient returns token decimals.
func GetTokenDecimalsWithClient(client *ethclient.Client, tokenAddress common.Address) (uint8, error) {
	if client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	token, err := NewERC20(tokenAddress, client)
	if err != nil {
		return 0, err
	}
	return token.Decimals(nil)
}

// GetTokenSymbolWithClient returns token symbol.
func GetTokenSymbolWithClient(client *ethclient.Client, tokenAddress common.Address) (string, error) {
	if client == nil {
		return "", fmt.Errorf("blockchain client not initialized")
	}
	token, err := NewERC20(tokenAddress, client)
	if err != nil {
		return "", err
	}
	return token.Symbol(nil)
}

// GetNonceWithClient returns the pending nonce for an address.
func GetNonceWithClient(client *ethclient.Client, address common.Address) (uint64, error) {
	if client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	return client.PendingNonceAt(context.Background(), address)
}

// GetGasPriceWithClient returns suggested gas price (legacy/AccessList).
func GetGasPriceWithClient(client *ethclient.Client) (*big.Int, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return client.SuggestGasPrice(context.Background())
}

// GetGasPriceWithMultiplierWithClient returns suggestGasPrice*multiplier (legacy/AccessList).
func GetGasPriceWithMultiplierWithClient(client *ethclient.Client, multiplier float64) (*big.Int, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if multiplier <= 0 {
		multiplier = 1
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, err
	}

	if multiplier != 1 {
		r := new(big.Rat).SetInt(gasPrice)
		m := new(big.Rat)
		if _, ok := m.SetString(fmt.Sprintf("%.6f", multiplier)); ok {
			r.Mul(r, m)
			scaled := new(big.Int).Quo(r.Num(), r.Denom())
			if scaled.Sign() > 0 {
				gasPrice = scaled
			}
		}
	}

	return gasPrice, nil
}

// SendTransactionWithClient sends a signed transaction.
func SendTransactionWithClient(client *ethclient.Client, signedTx *types.Transaction) error {
	if client == nil {
		return fmt.Errorf("blockchain client not initialized")
	}
	if signedTx == nil {
		return fmt.Errorf("signed tx is nil")
	}
	return client.SendTransaction(context.Background(), signedTx)
}

// SendRawTxParams holds parameters for building and sending a raw (non-contract) transaction
// with automatic retry for transient BSC mempool errors.
type SendRawTxParams struct {
	Client     *ethclient.Client
	ChainID    *big.Int
	PrivateKey *ecdsa.PrivateKey
	From       common.Address
	To         common.Address
	Value      *big.Int
	Data       []byte
	GasLimit   uint64
	GasPrice   *big.Int
}

// SendRawTransactionWithRetry builds, signs, and sends a legacy transaction.
// It retries on "nonce too low" (re-fetches nonce and re-signs) and
// "in-flight transaction limit" (waits for pending txs to clear).
// Returns the successfully sent signed transaction.
func SendRawTransactionWithRetry(p SendRawTxParams) (*types.Transaction, error) {
	if p.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if p.ChainID == nil {
		return nil, fmt.Errorf("chainID is nil")
	}
	if p.PrivateKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}

	const maxAttempts = 4

	nonce, err := GetNonceWithClient(p.Client, p.From)
	if err != nil {
		return nil, fmt.Errorf("get nonce failed: %w", err)
	}

	signer := types.NewEIP155Signer(p.ChainID)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		rawTx := types.NewTransaction(nonce, p.To, p.Value, p.GasLimit, p.GasPrice, p.Data)
		signed, signErr := types.SignTx(rawTx, signer, p.PrivateKey)
		if signErr != nil {
			return nil, fmt.Errorf("sign tx failed: %w", signErr)
		}

		sendErr := SendTransactionWithClient(p.Client, signed)
		if sendErr == nil {
			return signed, nil
		}

		if attempt == maxAttempts || !IsSendTxRetryable(sendErr) {
			return nil, sendErr
		}

		log.Printf("[blockchain] SendRawTransactionWithRetry attempt %d/%d failed: %v", attempt, maxAttempts, sendErr)

		if IsNonceTooLowError(sendErr) {
			// Nonce was stale; wait briefly for RPC to sync then re-fetch.
			time.Sleep(500 * time.Millisecond)
		} else {
			// in-flight limit or rate limit: wait longer for mempool to drain.
			delay := time.Duration(attempt) * 2 * time.Second
			if delay > 6*time.Second {
				delay = 6 * time.Second
			}
			time.Sleep(delay)
		}

		// Always re-fetch nonce before next attempt.
		freshNonce, nerr := GetNonceWithClient(p.Client, p.From)
		if nerr != nil {
			return nil, fmt.Errorf("re-fetch nonce after retry failed: %w", nerr)
		}
		nonce = freshNonce
	}

	return nil, fmt.Errorf("SendRawTransactionWithRetry: exhausted all attempts")
}

// GetTransactionReceiptWithClient returns the receipt of a transaction.
func GetTransactionReceiptWithClient(client *ethclient.Client, txHash common.Hash) (*types.Receipt, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return client.TransactionReceipt(context.Background(), txHash)
}

// SignTransactionWithChainID signs a transaction with the given chainId.
func SignTransactionWithChainID(tx *types.Transaction, chainID *big.Int, privateKeyHex string) (*types.Transaction, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx is nil")
	}
	if chainID == nil {
		return nil, fmt.Errorf("chainID is nil")
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}
	return signedTx, nil
}

// ------------------------------------------------------------------------------------
// Legacy helpers for default chain (kept for backwards compatibility).
// ------------------------------------------------------------------------------------

func GetBalance(address common.Address) (*big.Int, error) {
	return GetBalanceWithClient(Client, address)
}

func GetTokenBalance(tokenAddress, walletAddress common.Address) (*big.Int, error) {
	return GetTokenBalanceWithClient(Client, tokenAddress, walletAddress)
}

func GetTokenDecimals(tokenAddress common.Address) (uint8, error) {
	return GetTokenDecimalsWithClient(Client, tokenAddress)
}

func GetTokenSymbol(tokenAddress common.Address) (string, error) {
	return GetTokenSymbolWithClient(Client, tokenAddress)
}

func GetNonce(address common.Address) (uint64, error) { return GetNonceWithClient(Client, address) }

func GetGasPrice() (*big.Int, error) { return GetGasPriceWithClient(Client) }

func SignTransaction(tx *types.Transaction, privateKeyHex string) (*types.Transaction, error) {
	return SignTransactionWithChainID(tx, ChainID, privateKeyHex)
}

func SendTransaction(signedTx *types.Transaction) (common.Hash, error) {
	if err := SendTransactionWithClient(Client, signedTx); err != nil {
		return common.Hash{}, err
	}
	return signedTx.Hash(), nil
}

func WaitForTransaction(txHash common.Hash) (*types.Receipt, error) {
	return GetTransactionReceiptWithClient(Client, txHash)
}

func GetTransactionReceipt(txHash common.Hash) (*types.Receipt, error) {
	return GetTransactionReceiptWithClient(Client, txHash)
}

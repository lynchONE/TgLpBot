package blockchain

import (
	"TgLpBot/config"
	"context"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var Client *ethclient.Client
var ChainID *big.Int

// InitBlockchain initializes blockchain client
func InitBlockchain() error {
	var err error
	Client, err = ethclient.Dial(config.AppConfig.BSCRpcURL)
	if err != nil {
		return fmt.Errorf("failed to connect to BSC network: %w", err)
	}
	
	ChainID = big.NewInt(config.AppConfig.BSCChainID)
	
	// Test connection
	blockNumber, err := Client.BlockNumber(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get block number: %w", err)
	}
	
	log.Printf("BSC blockchain connected successfully, current block: %d", blockNumber)
	return nil
}

// CloseBlockchain closes the blockchain client
func CloseBlockchain() {
	if Client != nil {
		Client.Close()
	}
}

// GetBalance returns the balance of an address
func GetBalance(address common.Address) (*big.Int, error) {
	balance, err := Client.BalanceAt(context.Background(), address, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	return balance, nil
}

// GetTokenBalance returns the balance of a token for an address
func GetTokenBalance(tokenAddress, walletAddress common.Address) (*big.Int, error) {
	token, err := NewERC20(tokenAddress, Client)
	if err != nil {
		return nil, err
	}
	
	balance, err := token.BalanceOf(nil, walletAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get token balance: %w", err)
	}
	
	return balance, nil
}

// GetTokenDecimals returns the decimals of a token
func GetTokenDecimals(tokenAddress common.Address) (uint8, error) {
	token, err := NewERC20(tokenAddress, Client)
	if err != nil {
		return 0, err
	}
	
	decimals, err := token.Decimals(nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get token decimals: %w", err)
	}
	
	return decimals, nil
}

// GetTokenSymbol returns the symbol of a token
func GetTokenSymbol(tokenAddress common.Address) (string, error) {
	token, err := NewERC20(tokenAddress, Client)
	if err != nil {
		return "", err
	}
	
	symbol, err := token.Symbol(nil)
	if err != nil {
		return "", fmt.Errorf("failed to get token symbol: %w", err)
	}
	
	return symbol, nil
}

// GetNonce returns the nonce for an address
func GetNonce(address common.Address) (uint64, error) {
	nonce, err := Client.PendingNonceAt(context.Background(), address)
	if err != nil {
		return 0, fmt.Errorf("failed to get nonce: %w", err)
	}
	return nonce, nil
}

// GetGasPrice returns the current gas price
func GetGasPrice() (*big.Int, error) {
	gasPrice, err := Client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %w", err)
	}
	
	// Check if gas price exceeds max
	maxGasPrice := big.NewInt(config.AppConfig.MaxGasPrice)
	if gasPrice.Cmp(maxGasPrice) > 0 {
		return maxGasPrice, nil
	}
	
	return gasPrice, nil
}

// SignTransaction signs a transaction with private key
func SignTransaction(tx *types.Transaction, privateKeyHex string) (*types.Transaction, error) {
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(ChainID), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}
	
	return signedTx, nil
}

// SendTransaction sends a signed transaction
func SendTransaction(signedTx *types.Transaction) (common.Hash, error) {
	err := Client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to send transaction: %w", err)
	}
	
	return signedTx.Hash(), nil
}

// WaitForTransaction waits for a transaction to be mined
func WaitForTransaction(txHash common.Hash) (*types.Receipt, error) {
	ctx := context.Background()
	
	receipt, err := Client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction receipt: %w", err)
	}
	
	return receipt, nil
}

// GetTransactionReceipt returns the receipt of a transaction
func GetTransactionReceipt(txHash common.Hash) (*types.Receipt, error) {
	receipt, err := Client.TransactionReceipt(context.Background(), txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction receipt: %w", err)
	}
	return receipt, nil
}


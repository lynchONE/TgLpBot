package liquidity

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func (s *LiquidityService) ApproveTokenForSpender(
	client *ethclient.Client,
	chainID *big.Int,
	privateKey *ecdsa.PrivateKey,
	from, token, spender common.Address,
	amount *big.Int,
	opts TxOptions,
) error {
	return s.approveToken(client, chainID, privateKey, from, token, spender, amount, opts)
}

func (s *LiquidityService) ApproveTokenViaPermit2ForSpender(
	client *ethclient.Client,
	chainID *big.Int,
	privateKey *ecdsa.PrivateKey,
	from common.Address,
	token common.Address,
	spender common.Address,
	amount *big.Int,
	opts TxOptions,
) error {
	return s.approveTokenViaPermit2(client, chainID, privateKey, from, token, spender, amount, opts)
}

func (s *LiquidityService) BuildAuthForTx(
	client *ethclient.Client,
	chainID *big.Int,
	privateKey *ecdsa.PrivateKey,
	nonce uint64,
	value *big.Int,
	opts TxOptions,
) (*bind.TransactOpts, error) {
	return s.buildAuth(client, chainID, privateKey, nonce, value, opts)
}

func (s *LiquidityService) WaitMinedTx(
	client *ethclient.Client,
	chainID *big.Int,
	tx *types.Transaction,
) (*types.Receipt, error) {
	return s.waitMined(client, chainID, tx)
}

package chainexec

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/ethclient"
)

// ChainExecutor is a pluggable chain execution entry point.
// It is designed to keep "chain" as a first-class dimension and to reserve
// space for future non-EVM executors (e.g. Solana).
type ChainExecutor interface {
	Chain() string
	Kind() config.ChainKind
	Config() config.ChainConfig
	ExplorerTxURL(txHash string) string
}

// EVMExecutor is an EVM implementation of ChainExecutor.
type EVMExecutor interface {
	ChainExecutor
	Client() *ethclient.Client
	ChainID() *big.Int
}

type evmExecutor struct {
	chain   string
	cfg     config.ChainConfig
	client  *ethclient.Client
	chainID *big.Int
}

func (e *evmExecutor) Chain() string          { return e.chain }
func (e *evmExecutor) Kind() config.ChainKind { return config.ChainKindEVM }
func (e *evmExecutor) Config() config.ChainConfig {
	return e.cfg
}
func (e *evmExecutor) Client() *ethclient.Client { return e.client }
func (e *evmExecutor) ChainID() *big.Int {
	if e == nil || e.chainID == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(e.chainID)
}

func (e *evmExecutor) ExplorerTxURL(txHash string) string {
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return ""
	}
	tpl := strings.TrimSpace(e.cfg.ExplorerTxURLTemplate)
	if tpl == "" {
		return ""
	}
	return fmt.Sprintf(tpl, txHash)
}

// Get returns the executor for the given chain.
func Get(chain string) (ChainExecutor, error) {
	chain = config.NormalizeChain(chain)
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return nil, fmt.Errorf("chain config not found: %s", chain)
	}
	switch cc.Kind {
	case config.ChainKindEVM:
		evm, err := GetEVM(chain)
		if err != nil {
			return nil, err
		}
		return evm, nil
	default:
		return nil, fmt.Errorf("unsupported chain kind=%s chain=%s", cc.Kind, chain)
	}
}

// GetEVM returns the EVM executor for the given chain.
func GetEVM(chain string) (EVMExecutor, error) {
	chain = config.NormalizeChain(chain)
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return nil, fmt.Errorf("chain config not found: %s", chain)
	}
	if cc.Kind != config.ChainKindEVM {
		return nil, fmt.Errorf("chain is not EVM: %s kind=%s", chain, cc.Kind)
	}

	client, chainID, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return nil, err
	}
	return &evmExecutor{
		chain:   chain,
		cfg:     cc,
		client:  client,
		chainID: chainID,
	}, nil
}

package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// RPCRetryClient wraps an ethclient.Client and retries on RPC rate limits (HTTP 429, -32003, etc).
// This is intentionally conservative: only rate-limit errors are retried to avoid masking real failures.
type RPCRetryClient struct {
	*ethclient.Client
}

func withRateLimitRetry(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return fmt.Errorf("retry fn is nil")
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == maxAttempts || !isRPCRateLimited(err) {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return lastErr
}

func withRateLimitRetryResult[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return zero, fmt.Errorf("retry fn is nil")
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		v, err := fn(ctx)
		if err == nil {
			return v, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if attempt == maxAttempts || !isRPCRateLimited(err) {
			break
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return zero, lastErr
}

func wrapRPCRetryClient(client *ethclient.Client) *RPCRetryClient {
	if client == nil {
		return nil
	}
	return &RPCRetryClient{Client: client}
}

func (c *RPCRetryClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) ([]byte, error) {
		return c.Client.CallContract(ctx, call, blockNumber)
	})
}

func (c *RPCRetryClient) PendingCallContract(ctx context.Context, call ethereum.CallMsg) ([]byte, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) ([]byte, error) {
		return c.Client.PendingCallContract(ctx, call)
	})
}

func (c *RPCRetryClient) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) ([]byte, error) {
		return c.Client.CodeAt(ctx, account, blockNumber)
	})
}

func (c *RPCRetryClient) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) ([]byte, error) {
		return c.Client.PendingCodeAt(ctx, account)
	})
}

func (c *RPCRetryClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if c == nil || c.Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) (uint64, error) {
		return c.Client.PendingNonceAt(ctx, account)
	})
}

func (c *RPCRetryClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) (*big.Int, error) {
		return c.Client.SuggestGasPrice(ctx)
	})
}

func (c *RPCRetryClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) (*big.Int, error) {
		return c.Client.SuggestGasTipCap(ctx)
	})
}

func (c *RPCRetryClient) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	if c == nil || c.Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) (uint64, error) {
		return c.Client.EstimateGas(ctx, call)
	})
}

func (c *RPCRetryClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if c == nil || c.Client == nil {
		return fmt.Errorf("blockchain client not initialized")
	}
	if tx == nil {
		return fmt.Errorf("tx is nil")
	}
	return withRateLimitRetry(ctx, func(ctx context.Context) error {
		return c.Client.SendTransaction(ctx, tx)
	})
}

func (c *RPCRetryClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) (*types.Receipt, error) {
		return c.Client.TransactionReceipt(ctx, txHash)
	})
}

func (c *RPCRetryClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	return withRateLimitRetryResult(ctx, func(ctx context.Context) (*types.Header, error) {
		return c.Client.HeaderByNumber(ctx, number)
	})
}

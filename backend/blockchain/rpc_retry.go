package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
)

func isRPCRateLimited(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "too many requests") {
		return true
	}
	if strings.Contains(msg, "rate limit") || strings.Contains(msg, "ratelimit") {
		return true
	}
	// Some providers only expose the HTTP status in the message.
	if strings.Contains(msg, "429") && (strings.Contains(msg, "too many") || strings.Contains(msg, "request")) {
		return true
	}
	return false
}

func isRetryableRPCCallError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// Contract execution errors are deterministic; retrying doesn't help.
	if strings.Contains(msg, "execution reverted") || strings.Contains(msg, "reverted") {
		return false
	}

	if isRPCRateLimited(err) {
		return true
	}

	// Transient transport / gateway errors.
	if strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "504") {
		return true
	}

	return false
}

func rpcRetryDelay(attempt int, err error) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	if isRPCRateLimited(err) {
		// Keep it conservative for user-facing endpoints.
		return time.Duration(attempt)*500*time.Millisecond + 200*time.Millisecond
	}
	return time.Duration(attempt) * 250 * time.Millisecond
}

func callContractWithRetry(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	if Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		raw, err := Client.CallContract(ctx, msg, nil)
		if err == nil {
			return raw, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt == maxAttempts || !isRetryableRPCCallError(err) {
			break
		}

		wait := rpcRetryDelay(attempt, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

func callContractWithRetryAtBlock(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	if Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		raw, err := Client.CallContract(ctx, msg, blockNumber)
		if err == nil {
			return raw, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt == maxAttempts || !isRetryableRPCCallError(err) {
			break
		}

		wait := rpcRetryDelay(attempt, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

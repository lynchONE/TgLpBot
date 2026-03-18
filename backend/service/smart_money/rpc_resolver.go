package smart_money

import (
	"TgLpBot/base/rpcpool"
	"context"
	"fmt"
	"strings"
	"time"
)

const smartMoneyChain = "bsc"

func resolveSmartMoneyRPC(ctx context.Context, transport string) (rpcpool.Effective, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	eff, err := rpcpool.Default().EffectiveURL(ctx, smartMoneyChain, transport)
	if strings.TrimSpace(eff.URL) != "" {
		return eff, nil
	}
	if err != nil {
		return eff, err
	}
	return eff, fmt.Errorf("smart money rpc unavailable: chain=%s transport=%s", smartMoneyChain, transport)
}

func hasSmartMoneyRPC(ctx context.Context, transport string) bool {
	eff, err := resolveSmartMoneyRPC(ctx, transport)
	return err == nil && strings.TrimSpace(eff.URL) != ""
}

func handleSmartMoneyRPCEndpointError(eff rpcpool.Effective, err error) {
	if err == nil || eff.Source != rpcpool.SourceDB || eff.Endpoint == nil {
		return
	}
	mgr := rpcpool.Default()
	if mgr == nil {
		return
	}
	ctx := context.Background()
	if rpcpool.IsQuotaExhaustedError(err) {
		_ = mgr.DisableUntilNextMonth(ctx, eff.Endpoint.ID)
		return
	}
	if rpcpool.IsRateLimitedError(err) {
		// Smart Money watcher's block scans are bursty by design. A temporary
		// rate limit should trigger local backoff instead of poisoning the RPC pool.
		return
	}
	_ = mgr.DisableEndpoint(ctx, eff.Endpoint.ID, time.Now().Add(10*time.Minute), rpcpool.ReasonHealthFail)
}

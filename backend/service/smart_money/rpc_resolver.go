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
	handleSmartMoneyRPCEndpointErrorWithManager(rpcpool.Default(), eff, err)
}

func handleSmartMoneyRPCEndpointErrorWithManager(mgr *rpcpool.Manager, eff rpcpool.Effective, err error) {
	if err == nil || eff.Source != rpcpool.SourceDB || eff.Endpoint == nil {
		return
	}
	if mgr == nil {
		return
	}

	if shouldBackoffSmartMoneyRPCError(err) {
		// Smart Money block scans are bursty by design. Providers may return
		// both per-second throttling and temporary credit-plan 429s here.
		// Let the watcher back off locally instead of poisoning the shared RPC pool.
		return
	}

	ctx := context.Background()
	_ = mgr.DisableEndpoint(ctx, eff.Endpoint.ID, time.Now().Add(10*time.Minute), rpcpool.ReasonHealthFail)
}

func shouldBackoffSmartMoneyRPCError(err error) bool {
	return rpcpool.IsRateLimitedError(err) || rpcpool.IsQuotaExhaustedError(err)
}

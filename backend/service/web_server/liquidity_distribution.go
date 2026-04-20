package web_server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/service/pool"
)

const (
	liquidityDistCacheTTL    = 30 * time.Second
	liquidityDistCallTimeout = 12 * time.Second
)

func (s *Server) handleLiquidityDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	initData := strings.TrimSpace(q.Get("initData"))
	if initData == "" {
		initData = strings.TrimSpace(q.Get("init_data"))
	}
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if _, status, msg, err := requireUserAccess(user.ID); err != nil || status != 0 {
		http.Error(w, msg, status)
		return
	}

	chain := config.NormalizeChain(q.Get("chain"))
	protocol := strings.ToLower(strings.TrimSpace(q.Get("protocol")))
	address := strings.TrimSpace(q.Get("address"))
	if chain == "" || protocol == "" || address == "" {
		http.Error(w, "chain, protocol, address required", http.StatusBadRequest)
		return
	}
	if protocol != "v3" && protocol != "v4" {
		http.Error(w, "protocol must be v3 or v4", http.StatusBadRequest)
		return
	}

	radius := 20
	if v := strings.TrimSpace(q.Get("radius")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			radius = n
		}
	}

	cacheKey := fmt.Sprintf("liq_dist:%s:%s:%s:%d", chain, protocol, strings.ToLower(address), radius)
	if cached, ok := readRedisRawCache(cacheKey); ok {
		writeJSONBytes(w, http.StatusOK, cached)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), liquidityDistCallTimeout)
	defer cancel()

	profile, err := pool.GetLiquidityDistribution(ctx, chain, protocol, address, radius)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	payload, err := marshalJSONPayload(profile)
	if err != nil {
		http.Error(w, "encode failed", http.StatusInternalServerError)
		return
	}
	writeRedisRawCache(cacheKey, payload, liquidityDistCacheTTL)
	writeJSONBytes(w, http.StatusOK, payload)
}

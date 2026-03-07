package web_server

import (
	"encoding/json"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/service/exchange"
	"TgLpBot/service/ws"
)

// wsIncoming is the generic envelope for client->server WS messages.
type wsIncoming struct {
	Type         string `json:"type"`
	Chain        string `json:"chain,omitempty"`
	TokenAddress string `json:"token_address,omitempty"`
	Bar          string `json:"bar,omitempty"`
}

// wsCandleUpdate is pushed to subscribers.
type wsCandleUpdate struct {
	Type         string        `json:"type"`
	Chain        string        `json:"chain"`
	TokenAddress string        `json:"token_address"`
	Bar          string        `json:"bar"`
	Candles      []tokenCandle `json:"candles"`
}

type klineTopic struct {
	chain        string
	tokenAddress string
	bar          string
	chainIndex   string // resolved from chain config
}

func (t klineTopic) key() string {
	return t.chain + ":" + t.tokenAddress + ":" + t.bar
}

// okxSubKey is the key for OKX WS subscription dedup.
func (t klineTopic) okxSubKey() string {
	return t.chainIndex + ":" + t.tokenAddress + ":" + t.bar
}

type klinePoller struct {
	topic  klineTopic
	ticker *time.Ticker
	done   chan struct{}
}

// KlineFeed manages real-time kline subscriptions over WebSocket.
// It uses OKX DEX WebSocket for real-time pushes and falls back to REST polling.
type KlineFeed struct {
	mu      sync.Mutex
	subs    map[*ws.Client]*klineTopic         // client -> current subscription
	topics  map[string]map[*ws.Client]struct{} // topic key -> set of clients
	pollers map[string]*klinePoller            // topic key -> active poller (fallback only)
	okxWS   *exchange.OKXDexWS                 // OKX DEX WebSocket client
	okxSubs map[string]int                     // okxSubKey -> reference count
}

func NewKlineFeed() *KlineFeed {
	f := &KlineFeed{
		subs:    make(map[*ws.Client]*klineTopic),
		topics:  make(map[string]map[*ws.Client]struct{}),
		pollers: make(map[string]*klinePoller),
		okxSubs: make(map[string]int),
	}

	// Try to start OKX DEX WebSocket.
	if config.AppConfig.OKXAPIKey != "" && config.AppConfig.OKXSecretKey != "" {
		f.okxWS = exchange.NewOKXDexWS(f.onOKXCandle)
		go f.okxWS.Run()
		log.Printf("[KlineFeed] OKX DEX WebSocket enabled")
	} else {
		log.Printf("[KlineFeed] OKX DEX WebSocket disabled (no API keys), using REST polling fallback")
	}

	return f
}

// HandleMessage processes an incoming WS message from a client.
func (f *KlineFeed) HandleMessage(client *ws.Client, raw []byte) {
	var m wsIncoming
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	switch m.Type {
	case "subscribe_kline":
		f.subscribe(client, m.Chain, m.TokenAddress, m.Bar)
	case "unsubscribe_kline":
		f.unsubscribe(client)
	}
}

// HandleClientRemove cleans up subscriptions when a client disconnects.
func (f *KlineFeed) HandleClientRemove(client *ws.Client) {
	f.unsubscribe(client)
}

func (f *KlineFeed) subscribe(client *ws.Client, chain, tokenAddress, bar string) {
	chain = config.NormalizeChain(chain)
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	bar = normalizeOKXBar(bar)
	if tokenAddress == "" || bar == "" {
		return
	}

	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || cc.ChainID <= 0 {
		return
	}
	chainIndex := strconv.FormatInt(cc.ChainID, 10)

	topic := klineTopic{chain: chain, tokenAddress: tokenAddress, bar: bar, chainIndex: chainIndex}
	key := topic.key()

	f.mu.Lock()
	defer f.mu.Unlock()

	// Remove old subscription if client already subscribed to something else.
	if old, ok := f.subs[client]; ok {
		oldKey := old.key()
		if oldKey == key {
			return // already subscribed to the same topic
		}
		f.removeClientLocked(client, oldKey)
	}

	f.subs[client] = &topic
	if f.topics[key] == nil {
		f.topics[key] = make(map[*ws.Client]struct{})
	}
	f.topics[key][client] = struct{}{}

	// Subscribe via OKX WS or start poller.
	if f.okxWS != nil {
		oKey := topic.okxSubKey()
		f.okxSubs[oKey]++
		if f.okxSubs[oKey] == 1 {
			f.okxWS.Subscribe(chainIndex, tokenAddress, bar)
			log.Printf("[KlineFeed] OKX WS subscribe %s", oKey)
		}
	} else {
		// Fallback: start poller
		if _, ok := f.pollers[key]; !ok {
			f.startPollerLocked(topic)
		}
	}
}

func (f *KlineFeed) unsubscribe(client *ws.Client) {
	f.mu.Lock()
	defer f.mu.Unlock()

	old, ok := f.subs[client]
	if !ok {
		return
	}
	f.removeClientLocked(client, old.key())
}

func (f *KlineFeed) removeClientLocked(client *ws.Client, topicKey string) {
	topic := f.subs[client]
	delete(f.subs, client)
	if clients, ok := f.topics[topicKey]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(f.topics, topicKey)

			if f.okxWS != nil && topic != nil {
				oKey := topic.okxSubKey()
				f.okxSubs[oKey]--
				if f.okxSubs[oKey] <= 0 {
					delete(f.okxSubs, oKey)
					f.okxWS.Unsubscribe(topic.chainIndex, topic.tokenAddress, topic.bar)
					log.Printf("[KlineFeed] OKX WS unsubscribe %s", oKey)
				}
			} else {
				f.stopPollerLocked(topicKey)
			}
		}
	}
}

// onOKXCandle is called by the OKX WebSocket client when a candle is received.
func (f *KlineFeed) onOKXCandle(candle exchange.OKXWSCandle) {
	bar := exchange.ChannelToBar(candle.Channel)
	ts := candle.TimestampMS / 1000
	if ts <= 0 {
		return
	}

	tc := tokenCandle{
		T:       ts,
		O:       sanitizeFloat(candle.Open),
		H:       sanitizeFloat(candle.High),
		L:       sanitizeFloat(candle.Low),
		C:       sanitizeFloat(candle.Close),
		V:       sanitizeFloat(candle.Volume),
		VUSD:    sanitizeFloat(candle.VolumeUSD),
		Confirm: candle.Confirm,
	}

	chainName := resolveChainName(candle.ChainIndex)

	// Look up the exact topic key.
	topicKey := chainName + ":" + candle.TokenContractAddress + ":" + bar

	f.mu.Lock()
	clients := f.topics[topicKey]
	targets := make([]*ws.Client, 0, len(clients))
	for c := range clients {
		targets = append(targets, c)
	}
	f.mu.Unlock()

	if len(targets) == 0 {
		return
	}

	msg, err := json.Marshal(wsCandleUpdate{
		Type:         "kline_update",
		Chain:        chainName,
		TokenAddress: candle.TokenContractAddress,
		Bar:          bar,
		Candles:      []tokenCandle{tc},
	})
	if err != nil {
		return
	}

	for _, c := range targets {
		c.Send(msg)
	}
}

// resolveChainName maps a chainIndex back to chain name.
func resolveChainName(chainIndex string) string {
	for _, name := range []string{"bsc", "base"} {
		if cc, ok := config.AppConfig.GetChainConfig(name); ok {
			if strconv.FormatInt(cc.ChainID, 10) == chainIndex {
				return name
			}
		}
	}
	return "bsc"
}

// --- Fallback REST polling (used when OKX WS is not available) ---

func pollerInterval(bar string) time.Duration {
	switch strings.ToLower(bar) {
	case "1m":
		return 1 * time.Second
	case "3m", "5m":
		return 1 * time.Second
	case "15m":
		return 1 * time.Second
	case "30m":
		return 1 * time.Second
	default:
		return 1 * time.Second
	}
}

func (f *KlineFeed) startPollerLocked(topic klineTopic) {
	key := topic.key()
	interval := pollerInterval(topic.bar)
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	p := &klinePoller{topic: topic, ticker: ticker, done: done}
	f.pollers[key] = p

	log.Printf("[KlineFeed] start poller %s interval=%s", key, interval)

	go func() {
		f.pollOnce(topic)
		for {
			select {
			case <-ticker.C:
				f.pollOnce(topic)
			case <-done:
				return
			}
		}
	}()
}

func (f *KlineFeed) stopPollerLocked(topicKey string) {
	if p, ok := f.pollers[topicKey]; ok {
		p.ticker.Stop()
		close(p.done)
		delete(f.pollers, topicKey)
		log.Printf("[KlineFeed] stop poller %s", topicKey)
	}
}

func (f *KlineFeed) pollOnce(topic klineTopic) {
	cc, ok := config.AppConfig.GetChainConfig(topic.chain)
	if !ok || cc.ChainID <= 0 {
		return
	}

	okxSvc := exchange.NewOKXDexService()
	resp, err := okxSvc.GetMarketCandles(exchange.MarketCandlesRequest{
		ChainIndex:           strconv.FormatInt(cc.ChainID, 10),
		TokenContractAddress: topic.tokenAddress,
		Bar:                  topic.bar,
		Limit:                2,
	})
	if err != nil {
		return
	}

	candles := make([]tokenCandle, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		ts := row.TimestampMS / 1000
		if ts <= 0 {
			continue
		}
		candles = append(candles, tokenCandle{
			T:       ts,
			O:       sanitizeFloat(row.Open),
			H:       sanitizeFloat(row.High),
			L:       sanitizeFloat(row.Low),
			C:       sanitizeFloat(row.Close),
			V:       sanitizeFloat(row.Volume),
			VUSD:    sanitizeFloat(row.VolumeUSD),
			Confirm: row.Confirm,
		})
	}
	sort.Slice(candles, func(i, j int) bool { return candles[i].T < candles[j].T })
	if len(candles) == 0 {
		return
	}

	msg, err := json.Marshal(wsCandleUpdate{
		Type:         "kline_update",
		Chain:        topic.chain,
		TokenAddress: topic.tokenAddress,
		Bar:          topic.bar,
		Candles:      candles,
	})
	if err != nil {
		return
	}

	f.mu.Lock()
	clients := f.topics[topic.key()]
	targets := make([]*ws.Client, 0, len(clients))
	for c := range clients {
		targets = append(targets, c)
	}
	f.mu.Unlock()

	for _, c := range targets {
		c.Send(msg)
	}
}

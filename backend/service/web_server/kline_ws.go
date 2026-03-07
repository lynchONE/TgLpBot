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
}

func (t klineTopic) key() string {
	return t.chain + ":" + t.tokenAddress + ":" + t.bar
}

type klinePoller struct {
	topic  klineTopic
	ticker *time.Ticker
	done   chan struct{}
}

// KlineFeed manages real-time kline subscriptions over WebSocket.
// Each client subscribes to one kline topic at a time.
// Unique topics are polled against OKX DEX API and results are pushed to subscribers.
type KlineFeed struct {
	mu      sync.Mutex
	subs    map[*ws.Client]*klineTopic         // client -> current subscription
	topics  map[string]map[*ws.Client]struct{} // topic key -> set of clients
	pollers map[string]*klinePoller            // topic key -> active poller
}

func NewKlineFeed() *KlineFeed {
	return &KlineFeed{
		subs:    make(map[*ws.Client]*klineTopic),
		topics:  make(map[string]map[*ws.Client]struct{}),
		pollers: make(map[string]*klinePoller),
	}
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

func pollerInterval(bar string) time.Duration {
	switch strings.ToLower(bar) {
	case "1m":
		return 2 * time.Second
	case "3m", "5m":
		return 3 * time.Second
	case "15m":
		return 5 * time.Second
	case "30m":
		return 8 * time.Second
	default:
		return 10 * time.Second
	}
}

func (f *KlineFeed) subscribe(client *ws.Client, chain, tokenAddress, bar string) {
	chain = config.NormalizeChain(chain)
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	bar = normalizeOKXBar(bar)
	if tokenAddress == "" || bar == "" {
		return
	}

	topic := klineTopic{chain: chain, tokenAddress: tokenAddress, bar: bar}
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

	// Start poller if this is the first subscriber.
	if _, ok := f.pollers[key]; !ok {
		f.startPollerLocked(topic)
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
	delete(f.subs, client)
	if clients, ok := f.topics[topicKey]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(f.topics, topicKey)
			f.stopPollerLocked(topicKey)
		}
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

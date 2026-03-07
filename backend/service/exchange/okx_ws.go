package exchange

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/config"

	"github.com/gorilla/websocket"
)

// OKXWSCandle represents a single candle pushed by OKX DEX WebSocket.
type OKXWSCandle struct {
	Channel              string
	ChainIndex           string
	TokenContractAddress string
	TimestampMS          int64
	Open                 float64
	High                 float64
	Low                  float64
	Close                float64
	Volume               float64
	VolumeUSD            float64
	Confirm              bool
}

// OKXWSCandleHandler is called when a candle update is received.
type OKXWSCandleHandler func(candle OKXWSCandle)

// okxWSSub represents an active subscription.
type okxWSSub struct {
	Channel              string
	ChainIndex           string
	TokenContractAddress string
}

func (s okxWSSub) key() string {
	return s.Channel + ":" + s.ChainIndex + ":" + s.TokenContractAddress
}

// OKXDexWS manages a persistent WebSocket connection to OKX DEX.
type OKXDexWS struct {
	mu       sync.Mutex
	wsURL    string
	conn     *websocket.Conn
	subs     map[string]okxWSSub // key -> sub
	handler  OKXWSCandleHandler
	done     chan struct{}
	closed   bool
	loggedIn bool
}

// NewOKXDexWS creates a new OKX DEX WebSocket client.
func NewOKXDexWS(handler OKXWSCandleHandler) *OKXDexWS {
	wsURL := strings.TrimSpace(config.AppConfig.OKXDexWSURL)
	if wsURL == "" {
		wsURL = "wss://wsdexpri.okx.com:443"
	}
	return &OKXDexWS{
		wsURL:   wsURL,
		subs:    make(map[string]okxWSSub),
		handler: handler,
		done:    make(chan struct{}),
	}
}

// Run starts the WebSocket connection loop (blocking). Call in a goroutine.
func (w *OKXDexWS) Run() {
	for {
		select {
		case <-w.done:
			return
		default:
		}
		if err := w.connectAndServe(); err != nil {
			log.Printf("[OKX-WS] session ended: %v", err)
		}
		select {
		case <-w.done:
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// Close shuts down the WebSocket client.
func (w *OKXDexWS) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	close(w.done)
	if w.conn != nil {
		w.conn.Close()
	}
}

// Subscribe adds a kline channel subscription.
func (w *OKXDexWS) Subscribe(chainIndex, tokenContractAddress, bar string) {
	channel := barToChannel(bar)
	sub := okxWSSub{
		Channel:              channel,
		ChainIndex:           chainIndex,
		TokenContractAddress: strings.ToLower(strings.TrimSpace(tokenContractAddress)),
	}
	key := sub.key()

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.subs[key]; exists {
		return
	}
	w.subs[key] = sub
	if w.conn != nil && w.loggedIn {
		w.sendSubscribe([]okxWSSub{sub})
	}
}

// Unsubscribe removes a kline channel subscription.
func (w *OKXDexWS) Unsubscribe(chainIndex, tokenContractAddress, bar string) {
	channel := barToChannel(bar)
	sub := okxWSSub{
		Channel:              channel,
		ChainIndex:           chainIndex,
		TokenContractAddress: strings.ToLower(strings.TrimSpace(tokenContractAddress)),
	}
	key := sub.key()

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.subs[key]; !exists {
		return
	}
	delete(w.subs, key)
	if w.conn != nil && w.loggedIn {
		w.sendUnsubscribe([]okxWSSub{sub})
	}
}

func (w *OKXDexWS) connectAndServe() error {
	apiKey := config.AppConfig.OKXAPIKey
	secretKey := config.AppConfig.OKXSecretKey
	passphrase := config.AppConfig.OKXPassphrase
	if apiKey == "" || secretKey == "" {
		return fmt.Errorf("OKX API credentials not configured")
	}

	log.Printf("[OKX-WS] connecting to %s", w.wsURL)
	conn, _, err := websocket.DefaultDialer.Dial(w.wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	w.mu.Lock()
	w.conn = conn
	w.loggedIn = false
	w.mu.Unlock()

	defer func() {
		conn.Close()
		w.mu.Lock()
		w.conn = nil
		w.loggedIn = false
		w.mu.Unlock()
	}()

	// Login
	if err := w.login(conn, apiKey, secretKey, passphrase); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	log.Printf("[OKX-WS] logged in")

	w.mu.Lock()
	w.loggedIn = true
	// Resubscribe all active topics
	allSubs := make([]okxWSSub, 0, len(w.subs))
	for _, sub := range w.subs {
		allSubs = append(allSubs, sub)
	}
	w.mu.Unlock()

	if len(allSubs) > 0 {
		w.sendSubscribe(allSubs)
		log.Printf("[OKX-WS] resubscribed %d channels", len(allSubs))
	}

	// Start ping ticker
	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()
	go func() {
		for {
			select {
			case <-pingTicker.C:
				w.mu.Lock()
				c := w.conn
				w.mu.Unlock()
				if c != nil {
					_ = c.WriteMessage(websocket.TextMessage, []byte("ping"))
				}
			case <-w.done:
				return
			}
		}
	}()

	// Read loop
	for {
		select {
		case <-w.done:
			return nil
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}
		w.handleMessage(msg)
	}
}

func (w *OKXDexWS) login(conn *websocket.Conn, apiKey, secretKey, passphrase string) error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	preHash := timestamp + "GET" + "/users/self/verify"
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(preHash))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	loginMsg := map[string]interface{}{
		"op": "login",
		"args": []map[string]string{{
			"apiKey":     apiKey,
			"passphrase": passphrase,
			"timestamp":  timestamp,
			"sign":       sign,
		}},
	}
	data, _ := json.Marshal(loginMsg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return err
	}

	// Wait for login response
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, resp, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}

	var loginResp struct {
		Event  string `json:"event"`
		Code   string `json:"code"`
		Msg    string `json:"msg"`
		ConnID string `json:"connId"`
	}
	if err := json.Unmarshal(resp, &loginResp); err != nil {
		return fmt.Errorf("parse login response: %w (raw: %s)", err, string(resp))
	}
	if loginResp.Code != "0" {
		return fmt.Errorf("login rejected: code=%s msg=%s", loginResp.Code, loginResp.Msg)
	}
	return nil
}

func (w *OKXDexWS) sendSubscribe(subs []okxWSSub) {
	if len(subs) == 0 {
		return
	}
	args := make([]map[string]string, len(subs))
	for i, s := range subs {
		args[i] = map[string]string{
			"channel":              s.Channel,
			"chainIndex":           s.ChainIndex,
			"tokenContractAddress": s.TokenContractAddress,
		}
	}
	msg, _ := json.Marshal(map[string]interface{}{"op": "subscribe", "args": args})

	w.mu.Lock()
	c := w.conn
	w.mu.Unlock()
	if c != nil {
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("[OKX-WS] subscribe write error: %v", err)
		}
	}
}

func (w *OKXDexWS) sendUnsubscribe(subs []okxWSSub) {
	if len(subs) == 0 {
		return
	}
	args := make([]map[string]string, len(subs))
	for i, s := range subs {
		args[i] = map[string]string{
			"channel":              s.Channel,
			"chainIndex":           s.ChainIndex,
			"tokenContractAddress": s.TokenContractAddress,
		}
	}
	msg, _ := json.Marshal(map[string]interface{}{"op": "unsubscribe", "args": args})

	w.mu.Lock()
	c := w.conn
	w.mu.Unlock()
	if c != nil {
		if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("[OKX-WS] unsubscribe write error: %v", err)
		}
	}
}

func (w *OKXDexWS) handleMessage(raw []byte) {
	text := strings.TrimSpace(string(raw))
	if text == "pong" {
		return
	}

	// Try to parse as candle push: {"arg":{...},"data":[[...]]}
	var push struct {
		Arg struct {
			Channel              string `json:"channel"`
			ChainIndex           string `json:"chainIndex"`
			TokenContractAddress string `json:"tokenContractAddress"`
		} `json:"arg"`
		Data [][]string `json:"data"`
	}
	if err := json.Unmarshal(raw, &push); err != nil {
		return
	}
	if push.Arg.Channel == "" || len(push.Data) == 0 {
		// Might be subscribe confirmation or error — log if debug.
		if config.AppConfig != nil && config.AppConfig.OKXDebug {
			log.Printf("[OKX-WS] non-candle msg: %s", text)
		}
		return
	}

	for _, row := range push.Data {
		if len(row) < 8 {
			continue
		}
		ts := parseOKXInt64(row[0])
		if ts <= 0 {
			continue
		}
		candle := OKXWSCandle{
			Channel:              push.Arg.Channel,
			ChainIndex:           push.Arg.ChainIndex,
			TokenContractAddress: push.Arg.TokenContractAddress,
			TimestampMS:          ts,
			Open:                 parseOKXFloat(row[1]),
			High:                 parseOKXFloat(row[2]),
			Low:                  parseOKXFloat(row[3]),
			Close:                parseOKXFloat(row[4]),
			Volume:               parseOKXFloat(row[5]),
			VolumeUSD:            parseOKXFloat(row[6]),
			Confirm:              parseOKXBool(row[7]),
		}
		if w.handler != nil {
			w.handler(candle)
		}
	}
}

// barToChannel converts a bar string (e.g. "1m", "5m") to OKX channel name.
func barToChannel(bar string) string {
	b := strings.TrimSpace(bar)
	if b == "" {
		b = "1m"
	}
	return "dex-token-candle" + b
}

// channelToBar converts an OKX channel name back to bar string.
func ChannelToBar(channel string) string {
	return strings.TrimPrefix(channel, "dex-token-candle")
}

package smart_money

import (
	"TgLpBot/base/models"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*websocket.Conn]struct{}),
	}
}

func (h *WSHub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[SmartMoney WS] upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	// Keep connection alive by reading (discard messages)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

type WSEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type WSLPEventData struct {
	WalletAddress        string  `json:"wallet_address"`
	WalletLabel          *string `json:"wallet_label"`
	WalletSource         string  `json:"wallet_source,omitempty"`
	WalletSourceContract string  `json:"wallet_source_contract,omitempty"`
	WalletColor          string  `json:"wallet_color"`
	EventType            string  `json:"event_type"`
	Protocol             string  `json:"protocol"`
	PoolAddress          string  `json:"pool_address"`
	Token0Symbol         string  `json:"token0_symbol"`
	Token1Symbol         string  `json:"token1_symbol"`
	FeeTier              *int    `json:"fee_tier"`
	NftTokenID           *uint64 `json:"nft_token_id"`
	TxHash               string  `json:"tx_hash"`
	TxTimestamp          string  `json:"tx_timestamp"`
	BscscanURL           string  `json:"bscscan_url"`
}

func (h *WSHub) BroadcastLPEvent(event *models.SmartMoneyLPEvent, walletLabel *string, walletSource string, walletSourceContract string) {
	wsData := WSLPEventData{
		WalletAddress:        event.WalletAddress,
		WalletLabel:          walletLabel,
		WalletSource:         walletSource,
		WalletSourceContract: walletSourceContract,
		WalletColor:          WalletColor(event.WalletAddress),
		EventType:            event.EventType,
		Protocol:             event.Protocol,
		PoolAddress:          event.PoolAddress,
		Token0Symbol:         event.Token0Symbol,
		Token1Symbol:         event.Token1Symbol,
		FeeTier:              event.FeeTier,
		NftTokenID:           event.NftTokenID,
		TxHash:               event.TxHash,
		TxTimestamp:          event.TxTimestamp.Format("2006-01-02T15:04:05Z"),
		BscscanURL:           "https://bscscan.com/tx/" + event.TxHash,
	}

	msg := WSEvent{
		Type: "lp_event",
		Data: wsData,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

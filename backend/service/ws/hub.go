package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var defaultHub *Hub

// SetDefault sets the package-level default Hub (called once at startup).
func SetDefault(h *Hub) { defaultHub = h }

// Default returns the package-level default Hub.
func Default() *Hub { return defaultHub }

// SendProgress pushes an operation-progress event to a single user via the default Hub.
func SendProgress(userID uint, operation string, taskID uint, currentStep, totalSteps int, status, errMsg string) {
	h := defaultHub
	if h == nil {
		log.Printf("[WS SendProgress] hub is nil, dropping: user=%d op=%s step=%d/%d status=%s", userID, operation, currentStep, totalSteps, status)
		return
	}
	msg := struct {
		Type        string `json:"type"`
		Operation   string `json:"operation"`
		TaskID      uint   `json:"task_id,omitempty"`
		CurrentStep int    `json:"current_step"`
		TotalSteps  int    `json:"total_steps"`
		Status      string `json:"status"`
		Error       string `json:"error,omitempty"`
	}{
		Type:        "operation_progress",
		Operation:   operation,
		TaskID:      taskID,
		CurrentStep: currentStep,
		TotalSteps:  totalSteps,
		Status:      status,
		Error:       errMsg,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	delivered := h.SendToUsers([]uint{userID}, data)
	log.Printf("[WS SendProgress] user=%d op=%s task=%d step=%d/%d status=%s delivered=%d", userID, operation, taskID, currentStep, totalSteps, status, delivered)
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 4096
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins (same as existing CORS policy)
	},
}

// Client represents a single WebSocket connection.
type Client struct {
	hub    *Hub
	userID uint
	conn   *websocket.Conn
	send   chan []byte
}

// Hub manages all active WebSocket clients grouped by userID.
type Hub struct {
	mu             sync.RWMutex
	clients        map[*Client]struct{}
	userIndex      map[uint]map[*Client]struct{} // userID -> set of clients
	register       chan *Client
	unregister     chan *Client
	messageHandler func(*Client, []byte)
	onClientRemove func(*Client)
}

// NewHub creates a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		userIndex:  make(map[uint]map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// SetMessageHandler sets a callback invoked when a client sends a message.
func (h *Hub) SetMessageHandler(fn func(*Client, []byte)) {
	h.messageHandler = fn
}

// SetOnClientRemove sets a callback invoked after a client is unregistered.
func (h *Hub) SetOnClientRemove(fn func(*Client)) {
	h.onClientRemove = fn
}

// Run starts the hub's main loop. Call this in a goroutine.
func (h *Hub) Run() {
	log.Println("[WS Hub] started")
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			if h.userIndex[client.userID] == nil {
				h.userIndex[client.userID] = make(map[*Client]struct{})
			}
			h.userIndex[client.userID][client] = struct{}{}
			h.mu.Unlock()
			log.Printf("[WS Hub] client connected user_id=%d total=%d", client.userID, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if s, ok := h.userIndex[client.userID]; ok {
					delete(s, client)
					if len(s) == 0 {
						delete(h.userIndex, client.userID)
					}
				}
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("[WS Hub] client disconnected user_id=%d total=%d", client.userID, len(h.clients))
			if h.onClientRemove != nil {
				h.onClientRemove(client)
			}
		}
	}
}

// SendToUsers sends a message to all connected clients of the given user IDs.
// Returns the number of clients the message was delivered to.
func (h *Hub) SendToUsers(userIDs []uint, message []byte) int {
	if len(userIDs) == 0 || len(message) == 0 {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	delivered := 0
	for _, uid := range userIDs {
		clients, ok := h.userIndex[uid]
		if !ok {
			continue
		}
		for c := range clients {
			select {
			case c.send <- message:
				delivered++
			default:
				log.Printf("[WS Hub] send buffer full, dropping message for user=%d", uid)
			}
		}
	}
	return delivered
}

// OnlineUserCount returns the number of unique users with active WS connections.
func (h *Hub) OnlineUserCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.userIndex)
}

// ServeWS upgrades an HTTP connection to WebSocket and registers the client.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID uint) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS Hub] upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:    h,
		userID: userID,
		conn:   conn,
		send:   make(chan []byte, 64),
	}
	h.register <- client

	go client.writePump()
	go client.readPump()
}

// Send enqueues a message for delivery to this client. Returns false if the buffer is full.
func (c *Client) Send(data []byte) bool {
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
}

// readPump reads messages from the client and dispatches to the hub's messageHandler.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		if c.hub.messageHandler != nil {
			c.hub.messageHandler(c, msg)
		}
	}
}

// writePump writes messages to the client and sends pings.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

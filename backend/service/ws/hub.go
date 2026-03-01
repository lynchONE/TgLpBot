package ws

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 512
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
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	userIndex  map[uint]map[*Client]struct{} // userID -> set of clients
	register   chan *Client
	unregister chan *Client
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
		}
	}
}

// SendToUsers sends a message to all connected clients of the given user IDs.
func (h *Hub) SendToUsers(userIDs []uint, message []byte) {
	if len(userIDs) == 0 || len(message) == 0 {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, uid := range userIDs {
		clients, ok := h.userIndex[uid]
		if !ok {
			continue
		}
		for c := range clients {
			select {
			case c.send <- message:
			default:
				// Buffer full, skip this message for this client.
			}
		}
	}
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

// readPump reads messages from the client (mainly for pong handling).
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
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
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

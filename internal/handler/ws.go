package handler

import (
	"log"
	"messenger/internal/database"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	userID string
}

type WSHandler struct {
	db      *database.DB
	clients map[*Client]bool
	mu      sync.RWMutex
}

func NewWSHandler(db *database.DB) *WSHandler {
	return &WSHandler{
		db:      db,
		clients: make(map[*Client]bool),
	}
}

func (h *WSHandler) Handle(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("❌ error upgrader:%v", err)
		return
	}
	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	log.Printf("👤 new client connected. total:%d", len(h.clients))

	go client.writePump()
	client.readPump(h)
}

func (c *Client) readPump(h *WSHandler) {
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		c.conn.Close()
		log.Printf("👋 client disconnected. total:%d", len(h.clients))
	}()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		h.broadcast(msg, c)
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

func (h *WSHandler) broadcast(msg []byte, sender *Client) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client != sender {
			select {
			case client.send <- msg:
			default:
				close(client.send)
				delete(h.clients, client)
			}
		}
	}
}

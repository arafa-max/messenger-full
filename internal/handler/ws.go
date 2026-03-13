package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"messenger/internal/database"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // в продакшне проверять Origin
	},
}

// ─── Типы событий ───────────────────────────────────────────

const (
	// Сообщения
	EventMessage = "message"
	EventTyping  = "typing"
	EventRead    = "read"
	EventOnline  = "online"
	EventOffline = "offline"

	// WebRTC сигналинг
	EventCallOffer   = "call.offer"   // Алиса → Боб: хочу позвонить
	EventCallAnswer  = "call.answer"  // Боб → Алиса: принимаю
	EventCallReject  = "call.reject"  // Боб → Алиса: отклоняю
	EventCallHangup  = "call.hangup"  // кто угодно: завершаю
	EventCallICE     = "call.ice"     // ICE candidate relay
	EventCallRinging = "call.ringing" // уведомление о входящем
)

// ─── Структуры ──────────────────────────────────────────────

// WSMessage — универсальный конверт для всех событий
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	From    string          `json:"from,omitempty"` // заполняется сервером
	To      string          `json:"to,omitempty"`   // userID получателя (для p2p)
	ChatID  string          `json:"chat_id,omitempty"`
	TS      int64           `json:"ts"`
}

// CallPayload — payload для WebRTC событий
type CallPayload struct {
	CallID    string `json:"call_id"`
	SDP       string `json:"sdp,omitempty"`       // offer/answer SDP
	Candidate string `json:"candidate,omitempty"` // ICE candidate
	CallType  string `json:"call_type,omitempty"` // "audio" | "video"
}

// ─── Client ─────────────────────────────────────────────────

// Client — одно WebSocket соединение
type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	userID   string
	deviceID string
	handler  *WSHandler

	// для WebRTC: в каком звонке сейчас
	activeCallID string
}

// ─── WSHandler ──────────────────────────────────────────────

// WSHandler — менеджер всех соединений
type WSHandler struct {
	db *database.DB

	// userID → список клиентов (один юзер может быть с нескольких устройств)
	users map[string][]*Client
	mu    sync.RWMutex
}

func NewWSHandler(db *database.DB) *WSHandler {
	return &WSHandler{
		db:    db,
		users: make(map[string][]*Client),
	}
}

// ─── Handle ─────────────────────────────────────────────────

func (h *WSHandler) Handle(c *gin.Context) {
	userID := c.GetString("user_id")
	deviceID := c.GetString("device_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("❌ ws upgrade: %v", err)
		return
	}

	client := &Client{
		conn:     conn,
		send:     make(chan []byte, 256),
		userID:   userID,
		deviceID: deviceID,
		handler:  h,
	}

	h.register(client)
	log.Printf("🟢 ws connected: user=%s device=%s total_users=%d",
		userID, deviceID, h.userCount())

	// Уведомляем других что юзер онлайн
	h.broadcastPresence(userID, EventOnline)

	go client.writePump()
	client.readPump()
}

// ─── Register / Unregister ──────────────────────────────────

func (h *WSHandler) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.users[c.userID] = append(h.users[c.userID], c)
}

func (h *WSHandler) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients := h.users[c.userID]
	newList := make([]*Client, 0, len(clients))
	for _, cl := range clients {
		if cl != c {
			newList = append(newList, cl)
		}
	}

	if len(newList) == 0 {
		delete(h.users, c.userID)
	} else {
		h.users[c.userID] = newList
	}
}

func (h *WSHandler) userCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users)
}

// ─── Read / Write pumps ─────────────────────────────────────

func (c *Client) readPump() {
	defer func() {
		c.handler.unregister(c)
		c.handler.broadcastPresence(c.userID, EventOffline)
		c.conn.Close()
		log.Printf("🔴 ws disconnected: user=%s", c.userID)
	}()

	c.conn.SetReadLimit(64 * 1024) // 64KB макс размер сообщения
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("⚠️ ws parse error user=%s: %v", c.userID, err)
			continue
		}

		msg.From = c.userID
		msg.TS = time.Now().UnixMilli()

		c.handler.route(c, &msg)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			// Ping каждые 30 секунд чтобы держать соединение живым
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ─── Router ─────────────────────────────────────────────────

// route — маршрутизация входящих сообщений по типу
func (h *WSHandler) route(sender *Client, msg *WSMessage) {
	switch msg.Type {

	// Обычное сообщение в чат — рассылаем участникам чата
	case EventMessage:
		if msg.ChatID != "" {
			h.broadcastToChat(msg.ChatID, msg, sender.userID)
		}

	// Typing индикатор — только в чат
	case EventTyping:
		if msg.ChatID != "" {
			h.broadcastToChat(msg.ChatID, msg, sender.userID)
		}

	// Read receipt
	case EventRead:
		if msg.To != "" {
			h.sendToUser(msg.To, msg)
		}

	// WebRTC сигналинг — всё p2p через сервер как relay
	case EventCallOffer, EventCallAnswer, EventCallReject,
		EventCallHangup, EventCallICE, EventCallRinging:
		if msg.To != "" {
			h.sendToUser(msg.To, msg)
		}

	default:
		log.Printf("⚠️ unknown event type: %s from user=%s", msg.Type, sender.userID)
	}
}

// ─── Send helpers ────────────────────────────────────────────

// sendToUser — отправить сообщение конкретному юзеру (все его устройства)
func (h *WSHandler) sendToUser(userID string, msg *WSMessage) {
	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	clients := h.users[userID]
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- raw:
		default:
			// буфер переполнен — клиент слишком медленный
			log.Printf("⚠️ send buffer full for user=%s", userID)
		}
	}
}

// broadcastToChat — отправить всем участникам чата кроме отправителя
// TODO: в Блоке 6 заменить на реальную выборку участников из БД
func (h *WSHandler) broadcastToChat(chatID string, msg *WSMessage, senderID string) {
	_ = chatID // TODO: фильтровать по участникам чата в блоке 6
	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for userID, clients := range h.users {
		if userID == senderID {
			continue
		}
		for _, c := range clients {
			select {
			case c.send <- raw:
			default:
			}
		}
	}
}

// broadcastPresence — онлайн/оффлайн статус
func (h *WSHandler) broadcastPresence(userID string, eventType string) {
	msg := &WSMessage{
		Type: eventType,
		From: userID,
		TS:   time.Now().UnixMilli(),
	}
	raw, _ := json.Marshal(msg)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for uid, clients := range h.users {
		if uid == userID {
			continue
		}
		for _, c := range clients {
			select {
			case c.send <- raw:
			default:
			}
		}
	}
}

// IsUserOnline — проверить онлайн ли юзер (используется в call_handler)
func (h *WSHandler) IsUserOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients, ok := h.users[userID]
	return ok && len(clients) > 0
}

// SendSignal — отправить WebRTC сигнал (используется в call_handler)
func (h *WSHandler) SendSignal(toUserID string, msg *WSMessage) {
	h.sendToUser(toUserID, msg)
}

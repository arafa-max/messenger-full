package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"messenger/internal/database"
	rdb "messenger/internal/redis"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // в продакшне проверять Origin
	},
}

// ─── Типы событий ────────────────────────────────────────────────────────────

const (
	EventMessage = "message"
	EventTyping  = "typing"
	EventRead    = "read"
	EventOnline  = "online"
	EventOffline = "offline"

	// WebRTC сигналинг
	EventCallOffer   = "call.offer"
	EventCallAnswer  = "call.answer"
	EventCallReject  = "call.reject"
	EventCallHangup  = "call.hangup"
	EventCallICE     = "call.ice"
	EventCallRinging = "call.ringing"
	// Screen sharing — добавить сюда:
	EventScreenShareStart = "screen.start"
	EventScreenShareStop  = "screen.stop"
	EventScreenShareOffer = "screen.offer"

	// Voice room
	EventHandRaise = "voice.hand_raise"
	EventHandLower = "voice.hand_lower"
	EventSpeaking  = "voice.speaking"
)

// ─── Структуры ───────────────────────────────────────────────────────────────

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	From    string          `json:"from,omitempty"`
	To      string          `json:"to,omitempty"`
	ChatID  string          `json:"chat_id,omitempty"`
	TS      int64           `json:"ts"`
}

type CallPayload struct {
	CallID    string `json:"call_id"`
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
	CallType  string `json:"call_type,omitempty"` // audio | video
}

// Client — одно WebSocket соединение
type Client struct {
	conn         *websocket.Conn
	send         chan []byte
	userID       string
	deviceID     string
	handler      *WSHandler
	activeCallID string
	cancel       context.CancelFunc // отменяет Redis subscriber
}

// WSHandler — менеджер всех соединений + Redis pub/sub
type WSHandler struct {
	db    *database.DB
	redis *rdb.Client

	// userID → список клиентов (один юзер, несколько устройств)
	users map[string][]*Client
	mu    sync.RWMutex
}

func NewWSHandler(db *database.DB) *WSHandler {
	return &WSHandler{
		db:    db,
		users: make(map[string][]*Client),
	}
}

// NewWSHandlerWithRedis — использовать этот конструктор для кластерного режима
func NewWSHandlerWithRedis(db *database.DB, redis *rdb.Client) *WSHandler {
	return &WSHandler{
		db:    db,
		redis: redis,
		users: make(map[string][]*Client),
	}
}

// ─── Handle ──────────────────────────────────────────────────────────────────

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

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		conn:     conn,
		send:     make(chan []byte, 256),
		userID:   userID,
		deviceID: deviceID,
		handler:  h,
		cancel:   cancel,
	}

	h.register(client)
	log.Printf("🟢 ws connected: user=%s device=%s total_users=%d",
		userID, deviceID, h.userCount())

	// Запускаем Redis subscriber для этого юзера
	if h.redis != nil {
		go h.subscribeRedis(ctx, client)
	}

	// Уведомляем других что юзер онлайн
	h.publishPresence(userID, EventOnline)

	go client.writePump()
	client.readPump()

	// После отключения
	cancel()
}

// ─── Redis pub/sub ────────────────────────────────────────────────────────────

// redisChannel — канал для конкретного юзера
func redisUserChannel(userID string) string {
	return "ws:user:" + userID
}

// redisChatChannel — канал для чата
func redisChatChannel(chatID string) string {
	return "ws:chat:" + chatID
}

// subscribeRedis — слушает Redis канал и доставляет сообщения локальному клиенту
func (h *WSHandler) subscribeRedis(ctx context.Context, client *Client) {
	channel := redisUserChannel(client.userID)
	pubsub := h.redis.Subscribe(ctx, channel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			select {
			case client.send <- []byte(msg.Payload):
			default:
				log.Printf("⚠️ ws redis: send buffer full for user=%s", client.userID)
			}
		case <-ctx.Done():
			return
		}
	}
}

// publishToUser — публикует сообщение в Redis канал юзера
// Доставка происходит на любом инстансе где юзер подключён
func (h *WSHandler) publishToUser(userID string, msg *WSMessage) {
	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}

	if h.redis != nil {
		// Кластерный режим — через Redis
		if err := h.redis.Publish(context.Background(), redisUserChannel(userID), raw); err != nil {
			log.Printf("⚠️ ws redis publish user=%s: %v", userID, err)
		}
	} else {
		// Fallback — локальная доставка (single-instance)
		h.localSendToUser(userID, raw)
	}
}

// publishToChat — публикует сообщение в Redis канал чата
func (h *WSHandler) publishToChat(chatID string, msg *WSMessage, senderID string) {
	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}

	if h.redis != nil {
		if err := h.redis.Publish(context.Background(), redisChatChannel(chatID), raw); err != nil {
			log.Printf("⚠️ ws redis publish chat=%s: %v", chatID, err)
		}
	} else {
		h.localBroadcastToChat(chatID, raw, senderID)
	}
}

// publishPresence — онлайн/офлайн статус
func (h *WSHandler) publishPresence(userID, eventType string) {
	msg := &WSMessage{
		Type: eventType,
		From: userID,
		TS:   time.Now().UnixMilli(),
	}
	raw, _ := json.Marshal(msg)

	if h.redis != nil {
		// Публикуем в глобальный presence канал
		h.redis.Publish(context.Background(), "ws:presence", raw)
	} else {
		h.localBroadcastAll(raw, userID)
	}
}

// ─── Локальная доставка (fallback без Redis) ──────────────────────────────────

func (h *WSHandler) localSendToUser(userID string, raw []byte) {
	h.mu.RLock()
	clients := h.users[userID]
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- raw:
		default:
			log.Printf("⚠️ send buffer full for user=%s", userID)
		}
	}
}

func (h *WSHandler) localBroadcastToChat(chatID string, raw []byte, senderID string) {
	_ = chatID // TODO Block 6: фильтровать по участникам чата
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

func (h *WSHandler) localBroadcastAll(raw []byte, excludeUserID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for uid, clients := range h.users {
		if uid == excludeUserID {
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

// ─── Register / Unregister ───────────────────────────────────────────────────

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

// ─── Read / Write pumps ───────────────────────────────────────────────────────

func (c *Client) readPump() {
	defer func() {
		c.handler.unregister(c)
		c.handler.publishPresence(c.userID, EventOffline)
		c.conn.Close()
		log.Printf("🔴 ws disconnected: user=%s", c.userID)
	}()

	c.conn.SetReadLimit(64 * 1024)
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
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ─── Router ───────────────────────────────────────────────────────────────────

func (h *WSHandler) route(sender *Client, msg *WSMessage) {
	switch msg.Type {

	case EventMessage:
		if msg.ChatID != "" {
			h.publishToChat(msg.ChatID, msg, sender.userID)
		}

	case EventTyping:
		if msg.ChatID != "" {
			h.publishToChat(msg.ChatID, msg, sender.userID)
		}

	case EventRead:
		if msg.To != "" {
			h.publishToUser(msg.To, msg)
		}

	case EventCallOffer, EventCallAnswer, EventCallReject,
		EventCallHangup, EventCallICE, EventCallRinging:
		if msg.To != "" {
			h.publishToUser(msg.To, msg)
		}
	case EventScreenShareStart, EventScreenShareStop, EventScreenShareOffer:
		if msg.To != "" {
			h.publishToUser(msg.To, msg)
		} else if msg.ChatID != "" {
			h.publishToChat(msg.ChatID, msg, sender.userID)
		}

	case EventHandRaise, EventHandLower, EventSpeaking:
		if msg.ChatID != "" {
			h.publishToChat(msg.ChatID, msg, sender.userID)
		}

	default:
		log.Printf("⚠️ unknown event type: %s from user=%s", msg.Type, sender.userID)
	}
}

// ─── Public API (используется другими хендлерами) ────────────────────────────

// sendToUser — публичный метод для отправки из других хендлеров
func (h *WSHandler) sendToUser(userID string, msg *WSMessage) {
	h.publishToUser(userID, msg)
}

// IsUserOnline — проверить онлайн ли юзер на этом инстансе
func (h *WSHandler) IsUserOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients, ok := h.users[userID]
	return ok && len(clients) > 0
}

// SendSignal — отправить WebRTC сигнал (используется в call_handler)
func (h *WSHandler) SendSignal(toUserID string, msg *WSMessage) {
	h.publishToUser(toUserID, msg)
}

// broadcastToChat — публичный метод для отправки из message handler
func (h *WSHandler) broadcastToChat(chatID string, msg *WSMessage, senderID string) {
	h.publishToChat(chatID, msg, senderID)
}

// broadcastPresence — публичный метод
func (h *WSHandler) broadcastPresence(userID, eventType string) {
	h.publishPresence(userID, eventType)
}

// Добавить в конец internal/handler/ws.go

// StartChatSubscriber — запускается один раз при старте.
// Подписывается на паттерн chat:* и рассылает события
// всем онлайн участникам батчами по 100.
func (h *WSHandler) StartChatSubscriber(ctx context.Context) {
	if h.redis == nil {
		log.Println("⚠️ ws: Redis not configured, chat subscriber disabled")
		return
	}

	// PSubscribe слушает все каналы по паттерну chat:*
	pubsub := h.redis.PSubscribe(ctx, "chat:*")
	defer pubsub.Close()

	log.Println("✅ ws: chat subscriber started (batch fan-out)")

	ch := pubsub.Channel()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// Извлекаем chatID из имени канала "chat:{chatID}"
			chatID := strings.TrimPrefix(msg.Channel, "chat:")
			if chatID == "" {
				continue
			}
			h.fanOutChatEvent(chatID, []byte(msg.Payload))

		case <-ctx.Done():
			log.Println("🛑 ws: chat subscriber stopped")
			return
		}
	}
}

// fanOutChatEvent — рассылает raw payload всем онлайн
// участникам чата батчами по batchSize.
func (h *WSHandler) fanOutChatEvent(chatID string, raw []byte) {
	// TODO Block 6: фильтровать по участникам чата chatID
	_ = chatID
	const batchSize = 100

	h.mu.RLock()
	// Собираем всех онлайн клиентов
	var targets []*Client
	for _, clients := range h.users {
		targets = append(targets, clients...)
	}
	h.mu.RUnlock()

	if len(targets) == 0 {
		return
	}

	// Рассылаем батчами чтобы не блокировать mutex надолго
	for i := 0; i < len(targets); i += batchSize {
		end := i + batchSize
		if end > len(targets) {
			end = len(targets)
		}
		batch := targets[i:end]

		for _, c := range batch {
			select {
			case c.send <- raw:
			default:
				// буфер переполнен — пропускаем этого клиента
				log.Printf("⚠️ ws fan-out: buffer full user=%s", c.userID)
			}
		}
	}
}

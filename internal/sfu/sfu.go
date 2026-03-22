package sfu

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// SFUClient — клиент для общения с mediasoup SFU
type SFUClient struct {
	sfuURL string
	mu     sync.RWMutex
	conns  map[string]*websocket.Conn // roomId:peerId -> conn
}

func NewSFUClient(sfuURL string) *SFUClient {
	return &SFUClient{
		sfuURL: sfuURL,
		conns:  make(map[string]*websocket.Conn),
	}
}

type SFUMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Connect — подключить peer к комнате SFU
func (c *SFUClient) Connect(roomID, peerID string) (*websocket.Conn, error) {
	u, err := url.Parse(c.sfuURL)
	if err != nil {
		return nil, fmt.Errorf("invalid sfu url: %w", err)
	}
	u.Scheme = "ws"
	u.Path = "/"
	q := u.Query()
	q.Set("roomId", roomID)
	q.Set("peerId", peerID)
	u.RawQuery = q.Encode()

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(u.String(), http.Header{})
	if err != nil {
		return nil, fmt.Errorf("sfu connect failed: %w", err)
	}

	key := roomID + ":" + peerID
	c.mu.Lock()
	c.conns[key] = conn
	c.mu.Unlock()

	return conn, nil
}

// Disconnect — отключить peer от комнаты
func (c *SFUClient) Disconnect(roomID, peerID string) {
	key := roomID + ":" + peerID
	c.mu.Lock()
	conn, ok := c.conns[key]
	if ok {
		conn.Close()
		delete(c.conns, key)
	}
	c.mu.Unlock()
}

// Send — отправить сообщение в SFU
func (c *SFUClient) Send(conn *websocket.Conn, msgType string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	msg := SFUMessage{Type: msgType, Data: raw}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}

// GetRouterRtpCapabilities — получить RTP capabilities роутера
func (c *SFUClient) GetRouterRtpCapabilities(conn *websocket.Conn) (json.RawMessage, error) {
	if err := c.Send(conn, "getRouterRtpCapabilities", nil); err != nil {
		return nil, err
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// CreateTransport — создать WebRTC transport
func (c *SFUClient) CreateTransport(conn *websocket.Conn, direction string) (json.RawMessage, error) {
	if err := c.Send(conn, "createTransport", map[string]string{"direction": direction}); err != nil {
		return nil, err
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// BuildConnURL — собрать URL для прямого подключения клиента к SFU
func (c *SFUClient) BuildConnURL(roomID, peerID string) string {
	u, _ := url.Parse(c.sfuURL)
	u.Scheme = "ws"
	q := u.Query()
	q.Set("roomId", roomID)
	q.Set("peerId", peerID)
	u.RawQuery = q.Encode()
	return u.String()
}

// IsAvailable — проверить доступность SFU
func (c *SFUClient) IsAvailable() bool {
	u, err := url.Parse(c.sfuURL)
	if err != nil {
		return false
	}
	u.Scheme = "http"
	u.Path = "/health"
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
package handler

import (
	"encoding/json"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// ─── Опкоды ───────────────────────────────────────────────────────────────────
// Первый байт каждого бинарного фрейма — опкод.
// Клиент смотрит на него чтобы понять тип и способ декодирования.

const (
	OpMessage  byte = 0x01 // обычное сообщение чата
	OpTyping   byte = 0x02 // индикатор печати
	OpRead     byte = 0x03 // прочитано
	OpPresence byte = 0x04 // онлайн/офлайн
	OpCall     byte = 0x05 // WebRTC сигналинг
	OpPing     byte = 0x06 // ping (клиент → сервер)
	OpPong     byte = 0x07 // pong (сервер → клиент)
	OpError    byte = 0x08 // ошибка
	OpSystem   byte = 0x09 // системное уведомление
)

// eventToOpcode — маппинг строковых типов событий на опкоды
var eventToOpcode = map[string]byte{
	EventMessage: OpMessage,
	EventTyping:  OpTyping,
	EventRead:    OpRead,
	EventOnline:  OpPresence,
	EventOffline: OpPresence,

	EventCallOffer:   OpCall,
	EventCallAnswer:  OpCall,
	EventCallReject:  OpCall,
	EventCallHangup:  OpCall,
	EventCallICE:     OpCall,
	EventCallRinging: OpCall,

	EventScreenShareStart: OpCall,
	EventScreenShareStop:  OpCall,
	EventScreenShareOffer: OpCall,

	EventHandRaise: OpSystem,
	EventHandLower: OpSystem,
	EventSpeaking:  OpSystem,
}

// ─── Encode / Decode ──────────────────────────────────────────────────────────

// encodeBinary кодирует WSMessage в бинарный фрейм:
// [1 байт опкод][msgpack payload]
func encodeBinary(msg *WSMessage) ([]byte, error) {
	opcode, ok := eventToOpcode[msg.Type]
	if !ok {
		opcode = OpSystem
	}

	body, err := msgpack.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("msgpack encode: %w", err)
	}

	frame := make([]byte, 1+len(body))
	frame[0] = opcode
	copy(frame[1:], body)
	return frame, nil
}

// decodeBinary декодирует бинарный фрейм обратно в WSMessage
// Используется при получении бинарных сообщений от клиента
func decodeBinary(data []byte) (*WSMessage, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("frame too short")
	}

	// опкод в первом байте (пока не используем для роутинга — тип есть в payload)
	_ = data[0]

	var msg WSMessage
	if err := msgpack.Unmarshal(data[1:], &msg); err != nil {
		return nil, fmt.Errorf("msgpack decode: %w", err)
	}
	return &msg, nil
}

// decodeAuto — клиент может слать и JSON и binary
// Определяем по первому байту: если < 0x20 — бинарный фрейм, иначе JSON
func decodeAuto(data []byte) (*WSMessage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty frame")
	}

	// Первый байт < 0x20 (пробела) — это опкод, значит бинарный фрейм
	if data[0] < 0x20 {
		return decodeBinary(data)
	}

	// Иначе JSON (обратная совместимость)
	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	return &msg, nil
}

// marshalWSMessage — сериализует для отправки через Redis pub/sub
// Redis всегда текстовый, поэтому там JSON
func marshalWSMessage(msg *WSMessage) ([]byte, error) {
	return json.Marshal(msg)
}
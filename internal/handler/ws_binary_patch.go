package handler

import (
	"context"
	"encoding/json"
	"log"
)

// subscribeRedisBinary — получает JSON из Redis, перекодирует в binary для клиента
func (h *WSHandler) subscribeRedisBinary(ctx context.Context, client *Client) {
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

			// Redis хранит JSON — декодируем
			var wsMsg WSMessage
			if err := json.Unmarshal([]byte(msg.Payload), &wsMsg); err != nil {
				// Если не JSON — отправляем как есть
				select {
				case client.send <- []byte(msg.Payload):
				default:
					log.Printf("⚠️ ws redis: buffer full user=%s", client.userID)
				}
				continue
			}

			// Перекодируем в binary фрейм
			frame, err := encodeBinary(&wsMsg)
			if err != nil {
				log.Printf("⚠️ ws encode binary user=%s: %v", client.userID, err)
				continue
			}

			select {
			case client.send <- frame:
			default:
				log.Printf("⚠️ ws redis: buffer full user=%s", client.userID)
			}

		case <-ctx.Done():
			return
		}
	}
}

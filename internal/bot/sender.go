package bot

import (
	"context"
	"encoding/json"
	"log/slog"
)

// WSSender — отправляет сообщения через WebSocket hub
// Реализует интерфейс Sender
type WSSender struct {
	log *slog.Logger
}

func NewWSSender(log *slog.Logger) Sender {
	return &WSSender{log: log}
}

func (s *WSSender) SendMessage(_ context.Context, botToken string, chatID int64, text string, opts *SendOpts) error {
	// TODO: подключить к реальному WebSocket hub в Блоке 12
	// Сейчас логируем — когда будет фронтенд подключим доставку
	s.log.Info("bot: send message",
		"chat_id", chatID,
		"text", text,
	)
	return nil
}

func (s *WSSender) AnswerCallback(_ context.Context, botToken string, callbackID string, text string) error {
	s.log.Info("bot: answer callback",
		"callback_id", callbackID,
		"text", text,
	)
	return nil
}

func (s *WSSender) AnswerInline(_ context.Context, botToken string, queryID string, results []InlineResult) error {
	data, _ := json.Marshal(results)
	s.log.Info("bot: answer inline",
		"query_id", queryID,
		"results", string(data),
	)
	return nil
}
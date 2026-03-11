package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	db "messenger/internal/db/sqlc"
	rdb "messenger/internal/redis"
	"time"

	"github.com/google/uuid"
)

type MessageStore struct {
	q   *db.Queries
	rdb *rdb.Client
}

func (s *MessageStore) GetByID(ctx context.Context, id uuid.UUID) (db.Message, error) {
	return s.q.GetMessageByID(ctx, id)
}
func NewMessageStore(sqlDb *sql.DB, redis *rdb.Client) *MessageStore {
	return &MessageStore{
		q:   db.New(sqlDb),
		rdb: redis,
	}
}
func (s *MessageStore) Send(ctx context.Context, arg db.CreateMessageParams) (db.Message, error) {
	msg, err := s.q.CreateMessage(ctx, arg)
	if err != nil {
		return db.Message{}, fmt.Errorf("create message: %w", err)
	}
	_ = s.publishMessage(ctx, msg)
	return msg, nil
}
func (s *MessageStore) GetChatMessages(ctx context.Context, chatID, userID uuid.UUID, limit, offset int32) ([]db.Message, error) {
	return s.q.GetChatMessages(ctx, db.GetChatMessagesParams{
		ChatID: chatID,
		Limit:  limit,
		UserID: userID,
		Offset: offset,
	})
}

func (s *MessageStore) Edit(ctx context.Context, id uuid.UUID, content string, format sql.NullString) (db.Message, error) {
	msg, err := s.q.EditMessage(ctx, db.EditMessageParams{
		ID:      id,
		Content: content,
		Format:  format,
	})
	if err != nil {
		return db.Message{}, fmt.Errorf("edit message: %w", err)
	}
	_ = s.publishEvent(ctx, msg.ChatID, "message.edited", msg)
	return msg, nil
}

func (s *MessageStore) DeleteForAll(ctx context.Context, id, chatID uuid.UUID) error {
	if err := s.q.DeleteMessageForAll(ctx, id); err != nil {
		return fmt.Errorf("delete for all:%w", err)
	}
	_ = s.publishEvent(ctx, chatID, "message.deleted", map[string]any{
		"message_id": id,
		"chat_id":    chatID,
	})
	return nil
}

func (s *MessageStore) DeleteForMe(ctx context.Context, messageID, userID uuid.UUID) error {
	return s.q.DeleteMessageForMe(ctx, db.DeleteMessageForMeParams{
		MessageID: messageID,
		UserID:    userID,
	})
}

func (s *MessageStore) Forward(ctx context.Context, messageID, toChatID, senderID uuid.UUID) (db.Message, error) {
	msg, err := s.q.ForwardMessage(ctx, db.ForwardMessageParams{
		ID:       messageID,
		ChatID:   toChatID,
		SenderID: senderID,
	})
	if err != nil {
		return db.Message{}, fmt.Errorf("forward message: %w", err)
	}
	_ = s.publishMessage(ctx, msg)
	return msg, nil
}

func (s *MessageStore) Pin(ctx context.Context, messageID, chatID uuid.UUID) error {
	if err := s.q.PinMessage(ctx, messageID); err != nil {
		return fmt.Errorf("pin message: %w", err)
	}
	_ = s.publishEvent(ctx, chatID, "message.pinned", map[string]any{
		"message_id": messageID,
	})
	return nil
}

func (s *MessageStore) Unpin(ctx context.Context, messageID, chatID uuid.UUID) error {
	if err := s.q.UnpinMessage(ctx, messageID); err != nil {
		return fmt.Errorf("unpin message: %w", err)
	}
	_ = s.publishEvent(ctx, chatID, "message.unpinned", map[string]any{
		"message_id": messageID,
	})
	return nil
}

func (s *MessageStore) GetPinned(ctx context.Context, chatID uuid.UUID) ([]db.Message, error) {
	return s.q.GetPinnedMessages(ctx, chatID)
}

func (s *MessageStore) AddReaction(ctx context.Context, messageID, userID uuid.UUID, emoji string) error {
	if err := s.q.AddReaction(ctx, db.AddReactionParams{
		MessageID: messageID,
		UserID:    userID,
		Emoji:     emoji,
	}); err != nil {
		return fmt.Errorf("add reaction: %w", err)
	}
	msg, _ := s.q.GetMessageByID(ctx, messageID)
	_ = s.publishEvent(ctx, msg.ChatID, "message.reaction", map[string]any{
		"message_id": messageID,
		"user_id":    userID,
		"emoji":      emoji,
		"action":     "add",
	})
	return nil
}

func (s *MessageStore) RemoveReactions(ctx context.Context, messageID, userID uuid.UUID, emoji string) error {
	return s.q.RemoveReaction(ctx, db.RemoveReactionParams{
		MessageID: messageID,
		UserID:    userID,
		Emoji:     emoji,
	})
}

func (s *MessageStore) GetReactions(ctx context.Context, messageID uuid.UUID) ([]db.GetMessageReactionsRow, error) {
	return s.q.GetMessageReactions(ctx, messageID)
}

func (s *MessageStore) MarkDelivered(ctx context.Context, messageID, userID uuid.UUID) error {
	return s.q.UpdateMessageDelivered(ctx, db.UpdateMessageDeliveredParams{
		MessageID: messageID,
		UserID:    userID,
	})
}

func (s *MessageStore) MarkRead(ctx context.Context, messageID, userID uuid.UUID) error {
	if err := s.q.UpdateMessageRead(ctx, db.UpdateMessageReadParams{
		MessageID: messageID,
		UserID:    userID,
	}); err != nil {
		return err
	}
	msg, _ := s.q.GetMessageByID(ctx, messageID)
	_ = s.publishEvent(ctx, msg.ChatID, "message.read", map[string]any{
		"message_id": messageID,
		"user_id":    userID,
	})
	return nil
}

func (s *MessageStore) MarkChatRead(ctx context.Context, chatID, userID uuid.UUID) error {
	return s.q.MarkChatMessagesRead(ctx, db.MarkChatMessagesReadParams{
		ChatID: chatID,
		UserID: userID,
	})
}

func (s *MessageStore) SetTyping(ctx context.Context, chatID, userID uuid.UUID) error {
	key := fmt.Sprintf("typing:%s:%s", chatID, userID)
	if err := s.rdb.Set(ctx, key, "1", 5*time.Second); err != nil {
		return err
	}
	_ = s.publishEvent(ctx, chatID, "typing.start", map[string]any{
		"chat_id": chatID,
		"user_id": userID,
	})
	return nil
}

func (s *MessageStore) StopTyping(ctx context.Context, chatID, userID uuid.UUID) error {
	key := fmt.Sprintf("typing:%s:%s", chatID, userID)
	return s.rdb.Delete(ctx, key)
}

func (s *MessageStore) IsTyping(ctx context.Context, chatID, userID uuid.UUID) (bool, error) {
	key := fmt.Sprintf("typing:%s:%s", chatID, userID)
	return s.rdb.Exists(ctx, key)
}

func (s *MessageStore) Save(ctx context.Context, userID, messageID uuid.UUID) error {
	return s.q.SaveMessage(ctx, db.SaveMessageParams{
		UserID:    userID,
		MessageID: messageID,
	})
}

func (s *MessageStore) Unsave(ctx context.Context, userID, messageID uuid.UUID) error {
	return s.q.UnsaveMessage(ctx, db.UnsaveMessageParams{
		UserID:    userID,
		MessageID: messageID,
	})
}

func (s *MessageStore) GetSaved(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.Message, error) {
	return s.q.GetSavedMessages(ctx, db.GetSavedMessagesParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
}

func (s *MessageStore) SetReminder(ctx context.Context, userID, messageID uuid.UUID, remindAt time.Time) (db.MessageReminder, error) {
	return s.q.SetMessageReminder(ctx, db.SetMessageReminderParams{
		UserID:    userID,
		MessageID: messageID,
		RemindAt:  remindAt,
	})
}

func (s *MessageStore) GetScheduled(ctx context.Context) ([]db.Message, error) {
	return s.q.GetScheduledMessages(ctx)
}

func (s *MessageStore) SendScheduled(ctx context.Context, id uuid.UUID) error {
	return s.q.SendScheduledMessage(ctx, id)
}

func (s *MessageStore) Search(ctx context.Context, chatID uuid.UUID, query string, limit, offset int32) ([]db.Message, error) {
	return s.q.SearchMessages(ctx, db.SearchMessagesParams{
		ChatID:     chatID,
		Query:      query,
		PageSize:   limit,
		PageOffset: offset,
	})
}

func (s *MessageStore) CreateQuickReply(ctx context.Context, userID uuid.UUID, shortcut, text string) (db.QuickReply, error) {
	return s.q.CreateQuickReply(ctx, db.CreateQuickReplyParams{
		UserID:   userID,
		Shortcut: shortcut,
		Text:     text,
	})
}

func (s *MessageStore) GetQuickReplies(ctx context.Context, userID uuid.UUID) ([]db.QuickReply, error) {
	return s.q.GetQuickReplies(ctx, userID)
}

func (s *MessageStore) DeleteQuickReply(ctx context.Context, id, userID uuid.UUID) error {
	return s.q.DeleteQuickReply(ctx, db.DeleteQuickReplyParams{
		ID:     id,
		UserID: userID,
	})
}

func (s *MessageStore) publishMessage(ctx context.Context, msg db.Message) error {
	return s.publishEvent(ctx, msg.ChatID, "message.new", msg)
}

func (s *MessageStore) publishEvent(ctx context.Context, chatID uuid.UUID, event string, payload any) error {
	data, err := json.Marshal(map[string]any{
		"event":   event,
		"payload": payload,
	})

	if err != nil {
		return err
	}
	return s.rdb.Publish(ctx, fmt.Sprintf("chat:%s", chatID), data)
}

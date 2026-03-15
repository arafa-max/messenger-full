package bot

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	db "messenger/internal/db/sqlc"
)

// Update — входящий update от клиента
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
	InlineQuery   *InlineQuery   `json:"inline_query,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      User   `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
	Voice     *Voice `json:"voice,omitempty"`
	Photo     *Photo `json:"photo,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

type InlineQuery struct {
	ID     string `json:"id"`
	From   User   `json:"from"`
	Query  string `json:"query"`
	Offset string `json:"offset"`
}

type User struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // private, group, channel
}

type Voice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
}

type Photo struct {
	FileID string `json:"file_id"`
}

// Sender — интерфейс для отправки ответов
type Sender interface {
	SendMessage(ctx context.Context, botToken string, chatID int64, text string, opts *SendOpts) error
	AnswerCallback(ctx context.Context, botToken string, callbackID string, text string) error
	AnswerInline(ctx context.Context, botToken string, queryID string, results []InlineResult) error
}

type SendOpts struct {
	ReplyMarkup *ReplyMarkup `json:"reply_markup,omitempty"`
	ParseMode   string       `json:"parse_mode,omitempty"`
}

type ReplyMarkup struct {
	InlineKeyboard [][]InlineButton `json:"inline_keyboard,omitempty"`
	Keyboard       [][]KeyboardBtn  `json:"keyboard,omitempty"`
	RemoveKeyboard bool             `json:"remove_keyboard,omitempty"`
}

type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type KeyboardBtn struct {
	Text string `json:"text"`
}

type InlineResult struct {
	Type                string              `json:"type"`
	ID                  string              `json:"id"`
	Title               string              `json:"title"`
	InputMessageContent InputMessageContent `json:"input_message_content"`
}

type InputMessageContent struct {
	MessageText string `json:"message_text"`
}

// Dispatcher — роутит updates по типу
type Dispatcher struct {
	queries *db.Queries
	sender  Sender
	log     *slog.Logger
}

func NewDispatcher(queries *db.Queries, sender Sender, log *slog.Logger) *Dispatcher {
	return &Dispatcher{
		queries: queries,
		sender:  sender,
		log:     log,
	}
}

// Dispatch — главная точка входа, вызывается из webhook handler
func (d *Dispatcher) Dispatch(ctx context.Context, bot db.Bot, raw json.RawMessage) {
	var u Update
	if err := json.Unmarshal(raw, &u); err != nil {
		d.log.Error("dispatcher: failed to parse update", "err", err)
		return
	}

	switch {
	case u.CallbackQuery != nil:
		d.handleCallback(ctx, bot, u.CallbackQuery)

	case u.InlineQuery != nil:
		d.handleInline(ctx, bot, u.InlineQuery)

	case u.Message != nil && strings.HasPrefix(u.Message.Text, "/"):
		d.handleCommand(ctx, bot, u.Message)

	case u.Message != nil:
		d.handleMessage(ctx, bot, u.Message)
	}
}

// handleCommand — обрабатывает /start, /help и кастомные команды
func (d *Dispatcher) handleCommand(ctx context.Context, bot db.Bot, msg *Message) {
	parts := strings.SplitN(msg.Text, " ", 2)
	cmd := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	// Убираем @botname если есть: /start@mybot → start
	if idx := strings.Index(cmd, "@"); idx != -1 {
		cmd = cmd[:idx]
	}

	switch cmd {
	case "start":
		_ = d.sender.SendMessage(ctx, bot.Token, msg.Chat.ID,
			"👋 Hello! I'm "+bot.Name+". Use /help to see available commands.",
			nil,
		)

	case "help":
		// Берём команды из БД
		commands, err := d.queries.GetBotCommands(ctx, bot.ID)
		if err != nil || len(commands) == 0 {
			_ = d.sender.SendMessage(ctx, bot.Token, msg.Chat.ID,
				"No commands available.", nil)
			return
		}
		var sb strings.Builder
		sb.WriteString("Available commands:\n\n")
		for _, c := range commands {
			sb.WriteString("/" + c.Command + " — " + c.Description + "\n")
		}
		_ = d.sender.SendMessage(ctx, bot.Token, msg.Chat.ID, sb.String(), nil)

	default:
		// Кастомная команда — ищем в БД (для будущего расширения)
		d.log.Info("dispatcher: unknown command", "cmd", cmd, "bot", bot.Username)
		_ = d.sender.SendMessage(ctx, bot.Token, msg.Chat.ID,
			"Unknown command. Use /help.", nil)
	}
}

// handleCallback — обрабатывает нажатие inline кнопки
func (d *Dispatcher) handleCallback(ctx context.Context, bot db.Bot, cb *CallbackQuery) {
	d.log.Info("dispatcher: callback", "data", cb.Data, "from", cb.From.Username)

	switch cb.Data {
	case "about":
		_ = d.sender.AnswerCallback(ctx, bot.Token, cb.ID, "ℹ️ "+bot.Name)
		_ = d.sender.SendMessage(ctx, bot.Token, cb.Message.Chat.ID,
			bot.Description, nil)

	default:
		// Передаём data наружу — разработчик бота обработает сам
		_ = d.sender.AnswerCallback(ctx, bot.Token, cb.ID, "")
	}
}

// handleInline — обрабатывает @bot запросы
func (d *Dispatcher) handleInline(ctx context.Context, bot db.Bot, q *InlineQuery) {
	d.log.Info("dispatcher: inline query", "query", q.Query, "from", q.From.Username)

	// Базовый ответ — эхо запроса
	results := []InlineResult{
		{
			Type:  "article",
			ID:    "1",
			Title: q.Query,
			InputMessageContent: InputMessageContent{
				MessageText: q.Query,
			},
		},
	}

	_ = d.sender.AnswerInline(ctx, bot.Token, q.ID, results)
}

// handleMessage — обрабатывает обычные сообщения
func (d *Dispatcher) handleMessage(ctx context.Context, bot db.Bot, msg *Message) {
	d.log.Info("dispatcher: message", "text", msg.Text, "from", msg.From.Username)

	// Если AI включён — передаём в AI handler (заглушка пока)
	if bot.IsAiEnabled {
		_ = d.sender.SendMessage(ctx, bot.Token, msg.Chat.ID,
			"🤖 AI response coming soon...", nil)
		return
	}

	// Дефолт — эхо
	if msg.Text != "" {
		_ = d.sender.SendMessage(ctx, bot.Token, msg.Chat.ID,
			msg.Text, nil)
	}
}

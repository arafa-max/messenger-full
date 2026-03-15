
package ai

import (
    "context"
    "regexp"
    "strings"
    "unicode/utf8"
)

type LLMClient interface {
    SmartReply(ctx context.Context, messages []Message) ([]string, error)
    Summarize(ctx context.Context, messages []Message) (string, error)
    Moderate(ctx context.Context, text string) (ModResult, error)
}

type Message struct {
    Text string
    // Намеренно нет UserID, имён — в промпт не передаём личные данные
}

type ModResult struct {
    IsSpam  bool
    IsToxic bool
    Score   float64
}

// sanitize — чистим текст перед отправкой в LLM
func sanitize(text string, maxChars int) string {
    // 1. Обрезаем до лимита
    if utf8.RuneCountInString(text) > maxChars {
        runes := []rune(text)
        text = string(runes[:maxChars])
    }

    // 2. Убираем телефоны (не должны уходить в LLM)
    phoneRe := regexp.MustCompile(`(\+?\d[\d\s\-]{7,}\d)`)
    text = phoneRe.ReplaceAllString(text, "[phone]")

    // 3. Убираем email
    emailRe := regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
    text = emailRe.ReplaceAllString(text, "[email]")

    // 4. Убираем URL (могут содержать токены в query params)
    urlRe := regexp.MustCompile(`https?://\S+`)
    text = urlRe.ReplaceAllString(text, "[url]")

    // 5. Trim пробелы
    return strings.TrimSpace(text)
}

// sanitizeMessages — чистим список сообщений
func sanitizeMessages(messages []Message, maxChars int) []Message {
    result := make([]Message, 0, len(messages))
    for _, m := range messages {
        clean := sanitize(m.Text, maxChars)
        if clean != "" {
            result = append(result, Message{Text: clean})
        }
    }
    return result
}
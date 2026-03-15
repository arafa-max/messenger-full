package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type OllamaLLM struct {
    baseURL        string
    smartModel     string
    summaryModel   string
    maxPromptChars int
    client         *http.Client
}

func NewOllamaLLM(baseURL, smartModel, summaryModel string, maxChars, timeoutSec int) LLMClient {
    return &OllamaLLM{
        baseURL:        baseURL,
        smartModel:     smartModel,
        summaryModel:   summaryModel,
        maxPromptChars: maxChars,
        client: &http.Client{
            Timeout: time.Duration(timeoutSec) * time.Second,
            // Только localhost — наружу не ходим
            Transport: &http.Transport{
                DisableKeepAlives: false,
            },
        },
    }
}

type ollamaRequest struct {
    Model  string `json:"model"`
    Prompt string `json:"prompt"`
    Stream bool   `json:"stream"`
    Options ollamaOptions `json:"options"`
}

type ollamaOptions struct {
    Temperature float64 `json:"temperature"`
    NumPredict  int     `json:"num_predict"`
}

type ollamaResponse struct {
    Response string `json:"response"`
    Done     bool   `json:"done"`
}

func (o *OllamaLLM) call(ctx context.Context, model, prompt string, maxTokens int, temp float64) (string, error) {
    // Проверяем что URL локальный — защита от случайной отправки наружу
    if !strings.HasPrefix(o.baseURL, "http://localhost") &&
       !strings.HasPrefix(o.baseURL, "http://127.0.0.1") &&
       !strings.HasPrefix(o.baseURL, "http://10.") {
        return "", fmt.Errorf("ollama URL must be local: %s", o.baseURL)
    }

    req := ollamaRequest{
        Model:  model,
        Prompt: prompt,
        Stream: false,
        Options: ollamaOptions{
            Temperature: temp,
            NumPredict:  maxTokens,
        },
    }

    body, err := json.Marshal(req)
    if err != nil {
        return "", err
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
        o.baseURL+"/api/generate", bytes.NewReader(body))
    if err != nil {
        return "", err
    }
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := o.client.Do(httpReq)
    if err != nil {
        return "", fmt.Errorf("ollama unavailable: %w", err)
    }
    defer resp.Body.Close()

    var result ollamaResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    return strings.TrimSpace(result.Response), nil
}

func (o *OllamaLLM) SmartReply(ctx context.Context, messages []Message) ([]string, error) {
    // Санитизация — убираем личные данные
    clean := sanitizeMessages(messages, o.maxPromptChars)
    if len(clean) == 0 {
        return []string{"Окей", "Понял", "Позже"}, nil
    }

    // Берём только последнее сообщение — минимум данных
    lastMsg := clean[len(clean)-1].Text

    prompt := fmt.Sprintf(
        `Предложи ровно 3 коротких варианта ответа на сообщение.
Только варианты, без объяснений, каждый с новой строки.
Сообщение: %s`, lastMsg)

    response, err := o.call(ctx, o.smartModel, prompt, 60, 0.7)
    if err != nil {
        // Fallback — не ломаем UX
        return []string{"Окей", "Понял", "Позже"}, nil
    }

    replies := parseReplies(response)
    if len(replies) == 0 {
        return []string{"Окей", "Понял", "Позже"}, nil
    }
    return replies, nil
}

func (o *OllamaLLM) Summarize(ctx context.Context, messages []Message) (string, error) {
    if o.summaryModel == "" {
        return "Резюме недоступно.", nil
    }

    clean := sanitizeMessages(messages, o.maxPromptChars*5)

    // Берём максимум 50 сообщений
    if len(clean) > 50 {
        clean = clean[len(clean)-50:]
    }

    var sb strings.Builder
    for _, m := range clean {
        sb.WriteString("- ")
        sb.WriteString(m.Text)
        sb.WriteString("\n")
    }

    prompt := fmt.Sprintf(
        `Сделай краткое резюме переписки в 3-4 предложениях.
Только суть, без лишних слов.
Переписка:
%s`, sb.String())

    response, err := o.call(ctx, o.summaryModel, prompt, 200, 0.3)
    if err != nil {
        return "Резюме временно недоступно.", nil
    }

    return response, nil
}

func (o *OllamaLLM) Moderate(ctx context.Context, text string) (ModResult, error) {
    clean := sanitize(text, 500)
    if clean == "" {
        return ModResult{}, nil
    }

    prompt := fmt.Sprintf(
        `Определи является ли текст спамом или токсичным.
Ответь ТОЛЬКО в формате JSON: {"spam":false,"toxic":false}
Текст: %s`, clean)

    response, err := o.call(ctx, o.smartModel, prompt, 20, 0.1)
    if err != nil {
        return ModResult{}, nil // При ошибке — пропускаем
    }

    var result struct {
        Spam  bool `json:"spam"`
        Toxic bool `json:"toxic"`
    }
    // Ищем JSON в ответе
    start := strings.Index(response, "{")
    end := strings.LastIndex(response, "}")
    if start != -1 && end != -1 {
        json.Unmarshal([]byte(response[start:end+1]), &result)
    }

    return ModResult{
        IsSpam:  result.Spam,
        IsToxic: result.Toxic,
    }, nil
}

// parseReplies — парсим 3 варианта из ответа LLM
func parseReplies(response string) []string {
    lines := strings.Split(response, "\n")
    var replies []string
    for _, line := range lines {
        line = strings.TrimSpace(line)
        // Убираем нумерацию типа "1.", "1)", "-"
        line = regexp.MustCompile(`^[\d\.\)\-\*]+\s*`).ReplaceAllString(line, "")
        line = strings.TrimSpace(line)
        if line != "" && utf8.RuneCountInString(line) <= 100 {
            replies = append(replies, line)
        }
        if len(replies) == 3 {
            break
        }
    }
    return replies
}
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Translator interface {
	Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error)
	DetectLanguage(ctx context.Context, text string) (string, error)
}

type StubTranslator struct{}

func (s *StubTranslator) Translate(_ context.Context, text, _, _ string) (string, error) {
	return "", fmt.Errorf("translation not available")
}

func (s *StubTranslator) DetectLanguage(_ context.Context, _ string) (string, error) {
	return "en", nil
}

type LibreTranslateClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewLibreTranslateClient(url, apiKey string, timeoutSec int) Translator {
	if url == "" {
		return &StubTranslator{}
	}
	return &LibreTranslateClient{
		baseURL: url,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

type translateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"`
	Target string `json:"target"`
	APIKey string `json:"api_key,omitempty"`
}

type translateResponse struct {
	TranslatedText string `json:"translatedText"`
	Error          string `json:"error,omitempty"`
}

type detectRequest struct {
	Q      string `json:"q"`
	APIKey string `json:"api_key,omitempty"`
}

type detectResponse struct {
	Language   string  `json:"language"`
	Confidence float64 `json:"confidence"`
}

func (c *LibreTranslateClient) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	// Проверяем что URL локальный
	if !strings.HasPrefix(c.baseURL, "http://localhost") &&
		!strings.HasPrefix(c.baseURL, "http://127.0.0.1") &&
		!strings.HasPrefix(c.baseURL, "http://10.") {
		return "", fmt.Errorf("LibreTranslate URL must be local: %s", c.baseURL)
	}

	// Санитизируем текст
	text = sanitize(text, 1000)
	if text == "" {
		return "", fmt.Errorf("empty text")
	}

	// auto = автоопределение исходного языка
	if sourceLang == "" {
		sourceLang = "auto"
	}

	req := translateRequest{
		Q:      text,
		Source: sourceLang,
		Target: targetLang,
		APIKey: c.apiKey,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/translate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("LibreTranslate unavailable: %w", err)
	}
	defer resp.Body.Close()

	var result translateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Error != "" {
		return "", fmt.Errorf("LibreTranslate error: %s", result.Error)
	}

	return result.TranslatedText, nil
}

func (c *LibreTranslateClient) DetectLanguage(ctx context.Context, text string) (string, error) {
	text = sanitize(text, 500)
	if text == "" {
		return "en", nil
	}

	req := detectRequest{
		Q:      text,
		APIKey: c.apiKey,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "en", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/detect", bytes.NewReader(body))
	if err != nil {
		return "en", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "en", fmt.Errorf("LibreTranslate unavailable: %w", err)
	}
	defer resp.Body.Close()

	var results []detectResponse
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return "en", err
	}

	if len(results) == 0 {
		return "en", nil
	}

	return results[0].Language, nil
}
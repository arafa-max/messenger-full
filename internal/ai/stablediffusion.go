package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ImageGenerator interface {
	Generate(ctx context.Context, prompt string) ([]byte, error)
}

// StubImageGenerator — заглушка когда SD не настроен
type StubImageGenerator struct{}

func (s *StubImageGenerator) Generate(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("image generation not available")
}

// SDClient — клиент к AUTOMATIC1111
type SDClient struct {
	baseURL string
	client  *http.Client
}

func NewSDClient(url string, timeoutSec int) ImageGenerator {
	if url == "" {
		return &StubImageGenerator{}
	}
	return &SDClient{
		baseURL: url,
		client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

type sdRequest struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
	Steps          int    `json:"steps"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	CfgScale       float64 `json:"cfg_scale"`
	SamplerName    string  `json:"sampler_name"`
}

type sdResponse struct {
	Images []string `json:"images"` // base64
}

func (c *SDClient) Generate(ctx context.Context, prompt string) ([]byte, error) {
	// Проверяем что URL локальный
	if !strings.HasPrefix(c.baseURL, "http://localhost") &&
		!strings.HasPrefix(c.baseURL, "http://127.0.0.1") &&
		!strings.HasPrefix(c.baseURL, "http://10.") {
		return nil, fmt.Errorf("SD URL must be local: %s", c.baseURL)
	}

	// Санитизируем промпт
	prompt = sanitize(prompt, 300)
	if prompt == "" {
		return nil, fmt.Errorf("empty prompt")
	}

	req := sdRequest{
		Prompt:         prompt,
		NegativePrompt: "nsfw, violence, gore, ugly, blurry, bad anatomy",
		Steps:          20,   // баланс качество/скорость
		Width:          512,
		Height:         512,
		CfgScale:       7.0,
		SamplerName:    "Euler a",
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/sdapi/v1/txt2img", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("SD unavailable: %w", err)
	}
	defer resp.Body.Close()

	var result sdResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Images) == 0 {
		return nil, fmt.Errorf("SD returned no images")
	}

	// Декодируем base64 → bytes
	imgBytes, err := base64.StdEncoding.DecodeString(result.Images[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return imgBytes, nil
}
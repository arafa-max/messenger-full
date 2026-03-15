package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type TTSClient interface {
	Synthesize(ctx context.Context, text string, language string) ([]byte, error)
}

type StubTTS struct{}

func (s *StubTTS) Synthesize(_ context.Context, _ string, _ string) ([]byte, error) {
	return nil, fmt.Errorf("TTS not available")
}

type PiperClient struct {
	baseURL string
	client  *http.Client
}

func NewPiperClient(url string, timeoutSec int) TTSClient {
	if url == "" {
		return &StubTTS{}
	}
	return &PiperClient{
		baseURL: url,
		client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

func (c *PiperClient) Synthesize(ctx context.Context, text string, language string) ([]byte, error) {
	if !strings.HasPrefix(c.baseURL, "http://localhost") &&
		!strings.HasPrefix(c.baseURL, "http://127.0.0.1") &&
		!strings.HasPrefix(c.baseURL, "http://10.") {
		return nil, fmt.Errorf("piper URL must be local: %s", c.baseURL)
	}

	text = sanitize(text, 500)
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}

	body, err := json.Marshal(map[string]any{
		"text":     text,
		"language": language,
		"speed":    1.0,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/tts", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("piper unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("piper error: %d", resp.StatusCode)
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return audioBytes, nil
}
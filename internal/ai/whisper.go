package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type TranscribeResult struct {
	Text         string  `json:"text"`
	Language     string  `json:"language"`
	LanguageProb float64 `json:"language_prob"`
	Duration     float64 `json:"duration"`
}

type Transcriber interface {
	Transcribe(ctx context.Context, audioData []byte, filename string) (TranscribeResult, error)
}

type StubTranscriber struct{}

func (s *StubTranscriber) Transcribe(_ context.Context, _ []byte, _ string) (TranscribeResult, error) {
	return TranscribeResult{}, fmt.Errorf("transcription not available")
}

type WhisperClient struct {
	baseURL string
	client  *http.Client
}

func NewWhisperClient(url string, timeoutSec int) Transcriber {
	if url == "" {
		return &StubTranscriber{}
	}
	return &WhisperClient{
		baseURL: url,
		client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

type whisperResponse struct {
	Text         string  `json:"text"`
	Language     string  `json:"language"`
	LanguageProb float64 `json:"language_prob"`
	Duration     float64 `json:"duration"`
}

func (c *WhisperClient) Transcribe(ctx context.Context, audioData []byte, filename string) (TranscribeResult, error) {
	if !strings.HasPrefix(c.baseURL, "http://localhost") &&
		!strings.HasPrefix(c.baseURL, "http://127.0.0.1") &&
		!strings.HasPrefix(c.baseURL, "http://10.") {
		return TranscribeResult{}, fmt.Errorf("whisper URL must be local: %s", c.baseURL)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return TranscribeResult{}, err
	}
	if _, err := io.Copy(part, bytes.NewReader(audioData)); err != nil {
		return TranscribeResult{}, err
	}
	writer.Close()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/transcribe", &buf)
	if err != nil {
		return TranscribeResult{}, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return TranscribeResult{}, fmt.Errorf("whisper unavailable: %w", err)
	}
	defer resp.Body.Close()

	var result whisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return TranscribeResult{}, err
	}

	text := strings.TrimSpace(result.Text)
	if text == "" {
		return TranscribeResult{}, fmt.Errorf("empty transcription")
	}

	return TranscribeResult{
		Text:         text,
		Language:     result.Language,
		LanguageProb: result.LanguageProb,
		Duration:     result.Duration,
	}, nil
}
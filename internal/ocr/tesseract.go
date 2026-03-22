package ocr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client — обёртка над tesseract CLI
type Client struct {
	bin  string // путь к tesseract (по умолчанию "tesseract")
	lang string // язык(и), например "rus+eng"
	timeout time.Duration
}

func NewClient(lang string) *Client {
	if lang == "" {
		lang = "rus+eng"
	}
	return &Client{
		bin:     "tesseract",
		lang:    lang,
		timeout: 30 * time.Second,
	}
}

// Available проверяет что tesseract установлен
func (c *Client) Available() bool {
	_, err := exec.LookPath(c.bin)
	return err == nil
}

// ExtractText запускает OCR на переданных байтах изображения
// Поддерживает: JPEG, PNG, TIFF, BMP
func (c *Client) ExtractText(ctx context.Context, imageData []byte, ext string) (string, error) {
	if !c.Available() {
		return "", fmt.Errorf("tesseract not found in PATH")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Пишем во временный файл — tesseract не умеет читать из stdin
	tmpDir := os.TempDir()
	tmpIn := filepath.Join(tmpDir, fmt.Sprintf("ocr_in_%d%s", time.Now().UnixNano(), ext))
	tmpOutBase := filepath.Join(tmpDir, fmt.Sprintf("ocr_out_%d", time.Now().UnixNano()))
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOutBase + ".txt")

	if err := os.WriteFile(tmpIn, imageData, 0644); err != nil {
		return "", fmt.Errorf("write tmp image: %w", err)
	}

	// tesseract <input> <output_base> -l rus+eng
	// выходной файл будет <output_base>.txt
	cmd := exec.CommandContext(ctx, c.bin,
		tmpIn,
		tmpOutBase,
		"-l", c.lang,
		"--psm", "3", // автоопределение ориентации и режима сегментации
		"--oem", "3", // LSTM + legacy движок
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tesseract: %w — %s", err, stderr.String())
	}

	// Читаем результат
	out, err := os.ReadFile(tmpOutBase + ".txt")
	if err != nil {
		return "", fmt.Errorf("read ocr output: %w", err)
	}

	text := strings.TrimSpace(string(out))
	return text, nil
}
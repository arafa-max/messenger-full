package worker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	db "messenger/internal/db/sqlc"
)

func (w *Worker) indexMedia(ctx context.Context) {
	if w.sqlDB == nil {
		return
	}

	rows, err := w.sqlDB.QueryContext(ctx, `
		SELECT m.id, m.type, m.object_key
		FROM media m
		WHERE m.status = 'processed'
		  AND NOT EXISTS (
		      SELECT 1 FROM media_search_index msi WHERE msi.media_id = m.id
		  )
		  AND m.type IN ('image','voice','audio','video')
		LIMIT 20
	`)
	if err != nil {
		log.Printf("Worker/index: query: %v", err)
		return
	}
	defer rows.Close()

	type mediaRow struct {
		ID        string
		Type      db.MediaType
		ObjectKey string
	}

	var items []mediaRow
	for rows.Next() {
		var r mediaRow
		if err := rows.Scan(&r.ID, &r.Type, &r.ObjectKey); err != nil {
			continue
		}
		items = append(items, r)
	}

	for _, m := range items {
		if err := w.indexOne(ctx, m.ID, m.Type, m.ObjectKey); err != nil {
			log.Printf("Worker/index: %s: %v", m.ID, err)
		}
	}
}

func (w *Worker) indexOne(ctx context.Context, mediaID string, mediaType db.MediaType, objectKey string) error {
	switch mediaType {
	case db.MediaTypeImage:
		return w.indexImageOCR(ctx, mediaID, objectKey)
	case db.MediaTypeVoice, db.MediaTypeAudio:
		return w.indexAudioWhisper(ctx, mediaID, objectKey)
	case db.MediaTypeVideo:
		return w.indexVideoWhisper(ctx, mediaID, objectKey)
	default:
		return nil
	}
}

func (w *Worker) indexImageOCR(ctx context.Context, mediaID, objectKey string) error {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return w.saveSearchIndex(ctx, mediaID, "", "ocr")
	}

	data, err := w.storage.Download(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	tmpIn := filepath.Join(os.TempDir(), fmt.Sprintf("ocr_%s%s", mediaID, filepath.Ext(objectKey)))
	tmpOutBase := filepath.Join(os.TempDir(), fmt.Sprintf("ocr_%s", mediaID))
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOutBase + ".txt")

	if err := os.WriteFile(tmpIn, data, 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}

	cmd := exec.CommandContext(ctx, "tesseract", tmpIn, tmpOutBase, "-l", "rus+eng", "--oem", "3", "--psm", "3")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tesseract: %w — %s", err, string(out))
	}

	txtData, err := os.ReadFile(tmpOutBase + ".txt")
	if err != nil {
		return fmt.Errorf("read ocr result: %w", err)
	}

	return w.saveSearchIndex(ctx, mediaID, strings.TrimSpace(string(txtData)), "ocr")
}

func (w *Worker) indexAudioWhisper(ctx context.Context, mediaID, objectKey string) error {
	whisperURL := os.Getenv("WHISPER_URL")
	if whisperURL == "" {
		return nil
	}

	data, err := w.storage.Download(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	text, err := callWhisperAPI(ctx, whisperURL, data, filepath.Base(objectKey))
	if err != nil {
		return fmt.Errorf("whisper: %w", err)
	}

	return w.saveSearchIndex(ctx, mediaID, strings.TrimSpace(text), "whisper")
}

func (w *Worker) indexVideoWhisper(ctx context.Context, mediaID, objectKey string) error {
	whisperURL := os.Getenv("WHISPER_URL")
	if whisperURL == "" {
		return nil
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil
	}

	data, err := w.storage.Download(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("download video: %w", err)
	}

	tmpVideo := filepath.Join(os.TempDir(), fmt.Sprintf("vidx_%s%s", mediaID, filepath.Ext(objectKey)))
	tmpAudio := filepath.Join(os.TempDir(), fmt.Sprintf("audx_%s.mp3", mediaID))
	defer os.Remove(tmpVideo)
	defer os.Remove(tmpAudio)

	if err := os.WriteFile(tmpVideo, data, 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", tmpVideo,
		"-vn", "-acodec", "libmp3lame", "-q:a", "4",
		tmpAudio,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %w — %s", err, string(out))
	}

	audioData, err := os.ReadFile(tmpAudio)
	if err != nil {
		return fmt.Errorf("read audio: %w", err)
	}

	text, err := callWhisperAPI(ctx, whisperURL, audioData, "audio.mp3")
	if err != nil {
		return fmt.Errorf("whisper video: %w", err)
	}

	return w.saveSearchIndex(ctx, mediaID, strings.TrimSpace(text), "whisper")
}

func callWhisperAPI(ctx context.Context, baseURL string, data []byte, filename string) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, bytes.NewReader(data)); err != nil {
		return "", err
	}
	mw.WriteField("language", "ru")
	mw.WriteField("task", "transcribe")
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/asr", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return strings.TrimSpace(string(body)), nil
	}
	return result.Text, nil
}

func (w *Worker) saveSearchIndex(ctx context.Context, mediaID, text, source string) error {
	var messageID sql.NullString
	w.sqlDB.QueryRowContext(ctx,
		`SELECT id::text FROM messages WHERE media_id = $1 LIMIT 1`, mediaID,
	).Scan(&messageID)

	_, err := w.sqlDB.ExecContext(ctx, `
		INSERT INTO media_search_index (id, media_id, message_id, content, source, lang)
		VALUES ($1, $2, $3, $4, $5, 'ru')
		ON CONFLICT DO NOTHING
	`,
		uuid.New().String(),
		mediaID,
		nullableString(messageID),
		text,
		source,
	)
	if err != nil {
		return fmt.Errorf("insert index: %w", err)
	}

	log.Printf("Worker/index: indexed media %s via %s (%d chars)", mediaID, source, len(text))
	return nil
}

func nullableString(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}
package worker

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"log"
	"strings"
  "os"
    "os/exec"
    "path/filepath"
	db "messenger/internal/db/sqlc"

	"github.com/disintegration/imaging"
	"github.com/sqlc-dev/pqtype"
)

func (w *Worker) processMedia(ctx context.Context) {
	items, err := w.q.GetPendingMedia(ctx)
	if err != nil {
		log.Printf("Worker: getPendingMedia error: %v", err)
		return
	}
	for _, m := range items {
		if err := w.processSingleMedia(ctx, m); err != nil {
			log.Printf("Worker: processMedia %s error: %v", m.ID, err)
			_ = w.q.SetMediaFailed(ctx, m.ID)
		}
	}
}

func (w *Worker) processSingleMedia(ctx context.Context, m db.Medium) error {
	switch m.Type {
	case db.MediaTypeImage:
		return w.processImage(ctx, m)
	case db.MediaTypeAudio:
		return w.processAudio(ctx, m)
	default:
		return w.q.SetMediaProcessed(ctx, m.ID)
	}
}

func (w *Worker) processImage(ctx context.Context, m db.Medium) error {
	data, err := w.storage.Download(ctx, m.ObjectKey)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	// Сжимаем если > 1920px
	if img.Bounds().Dx() > 1920 {
		img = imaging.Resize(img, 1920, 0, imaging.Lanczos)
	}

	// Сохраняем сжатый оригинал
	origKey := jpgKey(m.ObjectKey)
	origData, err := encodeJPEG(img, 85)
	if err != nil {
		return fmt.Errorf("encode orig: %w", err)
	}
	if err := w.storage.Upload(ctx, origKey, bytes.NewReader(origData), int64(len(origData)), "image/jpeg"); err != nil {
		return fmt.Errorf("upload orig: %w", err)
	}

	// Thumbnail 320x320
	thumb := imaging.Thumbnail(img, 320, 320, imaging.Lanczos)
	thumbData, err := encodeJPEG(thumb, 75)

	var thumbKey sql.NullString
	if err != nil {
		log.Printf("Worker: encode thumb %s: %v", m.ID, err)
	} else {
		key := "thumb/" + origKey
		if err := w.storage.Upload(ctx, key, bytes.NewReader(thumbData), int64(len(thumbData)), "image/jpeg"); err != nil {
			log.Printf("Worker: upload thumb %s: %v", m.ID, err)
		} else {
			thumbKey = sql.NullString{String: key, Valid: true}
		}
	}

	// Удаляем оригинал если ключ изменился (был .png/.gif и т.д.)
	if origKey != m.ObjectKey {
		if err := w.storage.DeleteObject(ctx, m.ObjectKey); err != nil {
			log.Printf("Worker: delete original %s: %v", m.ObjectKey, err)
		}
	}

	return w.q.SetMediaProcessedWithThumb(ctx, db.SetMediaProcessedWithThumbParams{
		ID:       m.ID,
		ThumbKey: thumbKey,
	})
}

func (w *Worker) processAudio(ctx context.Context, m db.Medium) error {
	data, err := w.storage.Download(ctx, m.ObjectKey)
	if err != nil {
		return fmt.Errorf("download audio: %w", err)
	}

	waveform, err := generateWaveform(data, 64)
	if err != nil {
		log.Printf("Worker: waveform %s: %v", m.ID, err)
		return w.q.SetMediaProcessed(ctx, m.ID)
	}

	return w.q.SetMediaProcessedWithWaveform(ctx, db.SetMediaProcessedWithWaveformParams{
		ID: m.ID,
		Waveform: pqtype.NullRawMessage{
			RawMessage: waveform,
			Valid:      true,
		},
	})
}

func jpgKey(key string) string {
	if idx := strings.LastIndex(key, "."); idx != -1 {
		return key[:idx] + ".jpg"
	}
	return key + ".jpg"
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func generateWaveform(data []byte, points int) ([]byte, error) {
	if len(data) < points*2 {
		return nil, fmt.Errorf("audio too short")
	}

	result := make([]byte, points)
	chunkSize := len(data) / points

	for i := 0; i < points; i++ {
		chunk := data[i*chunkSize : (i+1)*chunkSize]
		var sum float64
		for _, b := range chunk {
			v := float64(b) - 128
			sum += v * v
		}
		rms := sum / float64(len(chunk))
		normalized := rms / 128.0 * 255.0
		if normalized > 255 {
			normalized = 255
		}
		result[i] = byte(normalized)
	}

	return result, nil
}
func (w *Worker) processVideo(ctx context.Context, m db.Medium) error {
	// Скачиваем оригинал
	data, err := w.storage.Download(ctx, m.ObjectKey)
	if err != nil {
		return fmt.Errorf("download video: %w", err)
	}

	// Пишем во временный файл
	tmpIn := fmt.Sprintf("/tmp/%s_in%s", m.ID, filepath.Ext(m.ObjectKey))
	tmpOut := fmt.Sprintf("/tmp/%s_out.mp4", m.ID)
	tmpThumb := fmt.Sprintf("/tmp/%s_thumb.jpg", m.ID)
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOut)
	defer os.Remove(tmpThumb)

	if err := os.WriteFile(tmpIn, data, 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}

	// Транскодинг в 720p mp4
	cmd := exec.Command("ffmpeg", "-y",
		"-i", tmpIn,
		"-vf", "scale=-2:720",
		"-c:v", "libx264",
		"-crf", "23",
		"-preset", "fast",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		tmpOut,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg transcode: %w — %s", err, string(out))
	}

	// Thumbnail из первого кадра
	thumbCmd := exec.Command("ffmpeg", "-y",
		"-i", tmpIn,
		"-ss", "00:00:01",
		"-vframes", "1",
		"-vf", "scale=320:-2",
		tmpThumb,
	)
	thumbCmd.Run()

	// Загружаем транскодированное видео
	videoData, err := os.ReadFile(tmpOut)
	if err != nil {
		return fmt.Errorf("read output: %w", err)
	}

	videoKey := strings.TrimSuffix(m.ObjectKey, filepath.Ext(m.ObjectKey)) + "_720p.mp4"
	if err := w.storage.Upload(ctx, videoKey, bytes.NewReader(videoData), int64(len(videoData)), "video/mp4"); err != nil {
		return fmt.Errorf("upload video: %w", err)
	}

	// Загружаем thumbnail если есть
	var thumbKey sql.NullString
	if thumbData, err := os.ReadFile(tmpThumb); err == nil {
		key := "thumb/" + videoKey + ".jpg"
		if err := w.storage.Upload(ctx, key, bytes.NewReader(thumbData), int64(len(thumbData)), "image/jpeg"); err == nil {
			thumbKey = sql.NullString{String: key, Valid: true}
		}
	}

	return w.q.SetMediaProcessedWithThumb(ctx, db.SetMediaProcessedWithThumbParams{
		ID:       m.ID,
		ThumbKey: thumbKey,
	})
}

func (w *Worker) cleanupExpiredMedia(ctx context.Context) {
	items, err := w.q.GetPendingExpiredMedia(ctx)
	if err != nil {
		log.Printf("Worker: getExpiredMedia error: %v", err)
		return
	}

	for _, m := range items {
		if err := w.storage.DeleteObject(ctx, m.ObjectKey); err != nil {
			log.Printf("Worker: delete expired %s: %v", m.ObjectKey, err)
			continue
		}
		// Удаляем thumbnail если есть
		if m.ThumbKey.Valid && m.ThumbKey.String != "" {
			w.storage.DeleteObject(ctx, m.ThumbKey.String)
		}
		w.q.DeleteMedia(ctx, m.ID)
		log.Printf("Worker: deleted expired media %s", m.ID)
	}
}


package worker

import (
	"context"
	"database/sql"
	"log"
	"path/filepath"
	"strings"

	db "messenger/internal/db/sqlc"

	"github.com/google/uuid"
)

// runOCRAsync запускает OCR в фоне, не блокирует обработку медиа
func (w *Worker) runOCRAsync(ctx context.Context, m db.Medium, imageData []byte) {
	if w.ocr == nil || !w.ocr.Available() {
		return
	}

	ext := strings.ToLower(filepath.Ext(m.ObjectKey))
	if ext == "" {
		ext = ".jpg"
	}

	text, err := w.ocr.ExtractText(ctx, imageData, ext)
	if err != nil {
		log.Printf("Worker OCR %s: %v", m.ID, err)
		return
	}
	if text == "" {
		return
	}

	if err := w.q.CreateMediaSearchIndex(ctx, db.CreateMediaSearchIndexParams{
		MediaID:   m.ID,
		Content:   text,
		Source:    "ocr",
		Lang:      sql.NullString{String: "ru", Valid: true},
		MessageID: uuid.NullUUID{},
	}); err != nil {
		log.Printf("Worker OCR index %s: %v", m.ID, err)
	} else {
		log.Printf("Worker OCR indexed %s (%d chars)", m.ID, len(text))
	}
}
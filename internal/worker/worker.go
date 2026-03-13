package worker

import (
	"context"
	"log"
	db "messenger/internal/db/sqlc"
	"messenger/internal/storage"
	"time"
)

type Worker struct {
	q       *db.Queries
	storage *storage.MinIOClient
}

func New(q *db.Queries, s *storage.MinIOClient) *Worker {
	return &Worker{q: q, storage: s}
}
func (w *Worker) Start(ctx context.Context) {
	log.Println("Worker started")

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// run at start
	w.run(ctx)

	for {
		select {
		case <-ticker.C:
			w.run(ctx)
		case <-ctx.Done():
			log.Println("Worker stopped")
			return

		}
	}
}
func (w *Worker) run(ctx context.Context) {
	w.deleteExpiredMessages(ctx)
	w.sendScheduledMessage(ctx)
	w.sendReminders(ctx)
	w.processMedia(ctx) // ← новое
}

func (w *Worker) deleteExpiredMessages(ctx context.Context) {
	err := w.q.DeleteExpiredMessages(ctx)
	if err != nil {
		log.Printf("Worker: deleteExpiredMessages error: %v", err)
		return
	}
	log.Println("Worker: expired messages deleted")
}
func (w *Worker) sendScheduledMessage(ctx context.Context) {
	messages, err := w.q.GetScheduledMessages(ctx)
	if err != nil {
		log.Printf("Worker: getScheduledMessages error: %v", err)
		return
	}
	for _, msg := range messages {
		err := w.q.SendScheduledMessage(ctx, msg.ID)
		if err != nil {
			log.Printf("Worker: sendScheduledMessage error: %v", err)
			continue
		}
		log.Printf("Worker: scheduled message %s sent", msg.ID)

	}
}
func (w *Worker) sendReminders(ctx context.Context) {
	reminders, err := w.q.GetPendingReminders(ctx)
	if err != nil {
		log.Printf("Worker: getPendingReminders error: %v", err)
		return
	}
	for _, r := range reminders {
		// TODO: send notification through Websocket
		log.Printf("Worker: reminder %s for user %s", r.ID, r.UserID)

		err := w.q.MarkReminderSent(ctx, r.ID)
		if err != nil {
			log.Printf("Worker: sendScheduledMessage error: %v", err)
		}

	}
}

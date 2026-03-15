package stories

import (
	"context"
	"log/slog"
	"time"

	db "messenger/internal/db/sqlc"
)

// Scheduler — фоновая горутина, архивирует stories у которых истёк expires_at.
// Запускается в main.go: go stories.NewScheduler(queries, logger).Start(ctx)
type Scheduler struct {
	q        *db.Queries
	interval time.Duration
	logger   *slog.Logger
}

func NewScheduler(q *db.Queries, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		q:        q,
		interval: 5 * time.Minute,
		logger:   logger,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.logger.Info("stories scheduler started", "interval", s.interval)

	s.run(ctx) // первый прогон сразу

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("stories scheduler stopped")
			return
		case <-ticker.C:
			s.run(ctx)
		}
	}
}

func (s *Scheduler) run(ctx context.Context) {
	if err := s.q.CleanupExpiredStories(ctx); err != nil {
		s.logger.Error("stories cleanup failed", "error", err)
	}
}
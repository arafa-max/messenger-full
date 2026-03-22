package sentry

import (
	"time"

	"github.com/getsentry/sentry-go"
)

// Init инициализирует Sentry.
// dsn берётся из конфига (env SENTRY_DSN).
// Если DSN пустой — Sentry просто не активируется, ошибок нет.
func Init(dsn, env, version string) error {
	if dsn == "" {
		return nil // локальная разработка — не нужен
	}

	return sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: env,     // "production" / "staging"
		Release:     version, // версия из git tag (main.Version)

		// Трассировка производительности — 10% запросов
		// На бесплатном плане не превысим лимит
		TracesSampleRate: 0.1,

		// Перехватываем паники автоматически
		AttachStacktrace: true,

		// Не отправляем личные данные пользователей (GDPR)
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Чистим заголовок Authorization
			if event.Request != nil {
				delete(event.Request.Headers, "Authorization")
				delete(event.Request.Headers, "Cookie")
			}
			return event
		},
	})
}

// Flush ждёт отправки всех pending событий.
// Вызывать при graceful shutdown.
func Flush() {
	sentry.Flush(2 * time.Second)
}

// CaptureError отправляет ошибку в Sentry вручную.
func CaptureError(err error) {
	if err != nil {
		sentry.CaptureException(err)
	}
}

// CaptureMessage отправляет сообщение в Sentry.
func CaptureMessage(msg string) {
	sentry.CaptureMessage(msg)
}
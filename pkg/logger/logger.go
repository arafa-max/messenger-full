package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init(env string) {
	zerolog.TimeFieldFormat = time.RFC3339

	if env == "production" {
		// JSON логи для продакшна (Grafana/Loki читает)
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	} else {
		// Красивые логи для разработки
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}).With().Timestamp().Caller().Logger()
	}
}

func Get() zerolog.Logger {
	return log.Logger
}
func Info(msg string, args ...any) {
	event := log.Info()
	for i := 0; i+1 < len(args); i += 2 {
		event = event.Any(args[i].(string), args[i+1])
	}
	event.Msg(msg)
}

func Error(msg string, args ...any) {
	event := log.Error()
	for i := 0; i+1 < len(args); i += 2 {
		event = event.Any(args[i].(string), args[i+1])
	}
	event.Msg(msg)
}

func Warn(msg string, args ...any) {
	event := log.Warn()
	for i := 0; i+1 < len(args); i += 2 {
		event = event.Any(args[i].(string), args[i+1])
	}
	event.Msg(msg)
}

func Debug(msg string, args ...any) {
	event := log.Debug()
	for i := 0; i+1 < len(args); i += 2 {
		event = event.Any(args[i].(string), args[i+1])
	}
	event.Msg(msg)
}

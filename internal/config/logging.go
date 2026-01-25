package config

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func NewLogger(cfg LoggingConfig) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano

	level, err := zerolog.ParseLevel(strings.ToLower(cfg.Level))
	if err != nil {
		level = zerolog.InfoLevel
	}

	var output io.Writer = os.Stdout
	if strings.EqualFold(cfg.Format, "console") {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	logger := zerolog.New(output).Level(level).With().Timestamp().Logger()
	log.Logger = logger
	return logger
}

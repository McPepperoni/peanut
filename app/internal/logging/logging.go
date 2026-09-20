package logging

import (
	"io"
	"log/slog"
)

func New(output io.Writer) *slog.Logger {
	if output == nil {
		output = io.Discard
	}
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func Nop() *slog.Logger {
	return New(io.Discard)
}

func Normalize(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return Nop()
	}
	return logger
}

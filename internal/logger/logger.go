package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func New(logDir string) (*slog.Logger, error) {
	dayDir := filepath.Join(logDir, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	file, err := os.OpenFile(
		filepath.Join(dayDir, time.Now().Format("15-04-05")+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o644,
	)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, file), &slog.HandlerOptions{Level: level})), nil
}

package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/log"
)

func getLoggerFilePath(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}

	if logPath := os.Getenv("DBT_LS_LOG"); logPath != "" {
		return logPath, nil
	}

	return "", fmt.Errorf("Can't find a log file path. Should default to file")
}

func getLoggerWriter(flag string) (io.Writer, error) {
	filename, err := getLoggerFilePath(flag)
	if err != nil {
		home, _ := os.UserHomeDir()
		filename = home + "/.local/state/nvim/dbt-lsp.log"
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o666)
	if err != nil {
		return nil, fmt.Errorf("Error creating the logger: %s", err)
	}

	return file, nil
}

func GetLogger(file, logLevel string) (*log.Logger, error) {
	w, err := getLoggerWriter(file)
	if err != nil {
		return nil, err
	}

	var level log.Level
	switch logLevel {
	case "debug":
		level = log.DebugLevel
	case "info":
		level = log.InfoLevel
	default:
		level = log.InfoLevel
	}

	logger := log.NewWithOptions(w, log.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
		Level:           level,
		Prefix:          "[dbt_ls]",
	})

	return logger, nil
}

package logger

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	charm "github.com/charmbracelet/log"
)

const TraceLevel charm.Level = -8

type Logger struct {
	*charm.Logger
	filePath string
	close    func() error
}

func GetLogger(logDir, logLevel string) (*Logger, error) {
	file, path, err := openSessionLogFile(logDir)
	if err != nil {
		return nil, err
	}

	base := charm.NewWithOptions(file, charm.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
		Level:           parseLevel(logLevel),
		Prefix:          "[dbt_ls]",
		CallerOffset:    1,
	})

	styles := charm.DefaultStyles()
	styles.Levels[TraceLevel] = lipgloss.NewStyle().
		SetString("TRCE").
		Bold(true).
		MaxWidth(4).
		Foreground(lipgloss.Color("240"))
	base.SetStyles(styles)

	logger := &Logger{
		Logger:   base,
		filePath: path,
		close:    file.Close,
	}

	cleanupOldLogsAsync(filepath.Dir(path), path)
	return logger, nil
}

func parseLevel(raw string) charm.Level {
	switch strings.ToLower(raw) {
	case "trace":
		return TraceLevel
	case "debug":
		return charm.DebugLevel
	case "info", "":
		return charm.InfoLevel
	case "warn", "warning":
		return charm.WarnLevel
	case "error":
		return charm.ErrorLevel
	case "fatal":
		return charm.FatalLevel
	default:
		return charm.InfoLevel
	}
}

func (l *Logger) Trace(msg interface{}, keyvals ...interface{}) {
	l.Log(TraceLevel, msg, keyvals...)
}

func (l *Logger) Tracef(format string, args ...interface{}) {
	l.Log(TraceLevel, fmt.Sprintf(format, args...))
}

func (l *Logger) Close() error {
	if l == nil || l.close == nil {
		return nil
	}
	return l.close()
}

func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.filePath
}

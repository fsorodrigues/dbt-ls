package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	logFilePrefix   = "dbt-ls-"
	logFileSuffix   = ".log"
	retentionMaxAge = 14 * 24 * time.Hour
	retentionMax    = 20
)

func getLoggerDir(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}

	if logDir := os.Getenv("DBT_LS_LOG_DIR"); logDir != "" {
		return logDir, nil
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("can't resolve home directory: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(stateHome, "dbt-ls", "logs"), nil
}

func newSessionLogPath(dir string, now time.Time, pid int) string {
	filename := fmt.Sprintf("%s%s-%d%s", logFilePrefix, now.Format("20060102-150405"), pid, logFileSuffix)
	return filepath.Join(dir, filename)
}

func openSessionLogFile(dirFlag string) (*os.File, string, error) {
	dir, err := getLoggerDir(dirFlag)
	if err != nil {
		return nil, "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", fmt.Errorf("error creating log directory %s: %w", dir, err)
	}

	path := newSessionLogPath(dir, time.Now(), os.Getpid())
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("error creating logger file %s: %w", path, err)
	}

	return file, path, nil
}

func cleanupOldLogsAsync(dir, currentPath string) {
	go cleanupOldLogs(dir, currentPath, time.Now())
}

func cleanupOldLogs(dir, currentPath string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type logEntry struct {
		path    string
		modTime time.Time
	}

	logs := make([]logEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, logFilePrefix) || !strings.HasSuffix(name, logFileSuffix) {
			continue
		}

		path := filepath.Join(dir, name)
		if path == currentPath {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}
		logs = append(logs, logEntry{path: path, modTime: info.ModTime()})
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].modTime.After(logs[j].modTime)
	})

	for i, log := range logs {
		tooOld := now.Sub(log.modTime) > retentionMaxAge
		tooMany := i >= retentionMax
		if tooOld || tooMany {
			_ = os.Remove(log.path)
		}
	}
}

package main

import (
	"fmt"
	"os"
	"time"
	"github.com/charmbracelet/log"
)

func getLogger(filename string) (*log.Logger, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o666)
	if err != nil {
		return nil, fmt.Errorf("Error creating the logger: %s", err)
	}

	logger := log.NewWithOptions(file, log.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
		Level:           log.DebugLevel,
		Prefix:          "[dbt_lsp]",
	})

	return logger, nil
}

func main() {
	logger, err := getLogger("/home/felipperodrigues/downloads/log.txt")
	if err != nil {
		logger.Error(err)
		panic(err)
	}
	logger.Info("dbt LSP started")
}

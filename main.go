package main

import (
	"bufio"
	"context"
	"flag"
	"os"

	"dbt_ls/analysis"
	"dbt_ls/logger"
	"dbt_ls/rpc"
	"dbt_ls/server"

	"github.com/charmbracelet/log"
)

func main() {
	var logFileFlag string
	var logLevel string
	srv := server.Server{}
	flag.StringVar(&logFileFlag, "log-file", "", "Path to log file")
	flag.StringVar(&logLevel, "log-level", "", "Set log level")
	flag.Parse()

	logger, err := logger.GetLogger(logFileFlag, logLevel)
	if err != nil {
		log.Fatal(err)
	}
	srv.Logger = logger
	srv.Logger.Info("dbt LSP started")

	writer := os.Stdout
	srv.Logger.Debug("Writer started")

	ctx, cancel := context.WithCancel(context.Background())
	srv.Cancel = cancel
	defer cancel()

	projectWatcher, err := analysis.NewWatcher("project", "./models", logger)
	if err != nil {
		srv.Logger.Fatalf("Error starting the projectWatcher. %s", err)
	}
	// ensures the watcher is closed, even if it has to be reinitialized by the
	// WatchProject function error handling
	defer projectWatcher.HandleAsyncClose(logger)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(rpc.Split)
	srv.Logger.Debug("Scanner started")

	state := analysis.NewState(logger, writer, projectWatcher)
	srv.Logger.Debug("Server State initialized")

	go state.WatchProject(ctx)
	go state.DrainNotifications(ctx)

	logger.Debug("Scanning Stdin for incoming messages")
	for scanner.Scan() {
		msg := scanner.Bytes()
		method, contents, err := rpc.DecodeMsg(msg)
		if err != nil {
			srv.Logger.Errorf("Can't parse RPC message: %s", err)
			continue
		}

		if ok := srv.Dispatch(state, method, contents); !ok {
			srv.Logger.Debugf("Ignoring unsupported method: %s", method)
		}
	}
	logger.Debug("Stopped Stdin scan")

	if err := scanner.Err(); err != nil {
		srv.Logger.Fatalf("Error scanning stdin: %s", err)
	}
}

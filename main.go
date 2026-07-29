package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/fsorodrigues/dbt-ls/analysis"
	"github.com/fsorodrigues/dbt-ls/logger"
	"github.com/fsorodrigues/dbt-ls/rpc"
	"github.com/fsorodrigues/dbt-ls/server"
)

func main() {
	os.Exit(run())
}

func run() int {
	var logDirFlag string
	var logLevel string
	srv := server.Server{}
	flag.StringVar(&logDirFlag, "log-dir", "", "Path to log directory")
	flag.StringVar(&logLevel, "log-level", "", "Set log level")
	flag.Parse()

	logger, err := logger.GetLogger(logDirFlag, logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating logger: %s\n", err)
		return 1
	}
	defer func() {
		if err := logger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing logger: %s\n", err)
		}
	}()

	srv.Logger = logger
	srv.Logger.Info("dbt LSP started")
	srv.Logger.Infof("Log file: %s", logger.Path())

	writer := os.Stdout
	srv.Logger.Debug("Writer started")

	ctx, cancel := context.WithCancel(context.Background())
	srv.Ctx = ctx
	srv.Cancel = cancel
	defer cancel()

	projectWatcher, err := analysis.NewWatcher("project", logger)
	if err != nil {
		srv.Logger.Errorf("Error starting the projectWatcher. %s", err)
		return 1
	}
	// ensures the watcher is closed, even if it has to be reinitialized by the
	// WatchProject function error handling
	defer projectWatcher.HandleAsyncClose(logger)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(rpc.Split)
	srv.Logger.Debug("Scanner started")

	state := analysis.NewState(logger, writer, projectWatcher)
	srv.Logger.Debug("Server State initialized")

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
		if srv.ExitCode != nil {
			break
		}
	}
	logger.Debug("Stopped Stdin scan")

	if err := scanner.Err(); err != nil {
		srv.Logger.Errorf("Error scanning stdin: %s", err)
		return 1
	}

	if srv.ExitCode != nil {
		return *srv.ExitCode
	}
	return 0
}

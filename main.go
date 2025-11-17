package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"dbt_lsp/analysis"
	"dbt_lsp/logger"
	"dbt_lsp/lsp"
	"dbt_lsp/rpc"

	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
)

func handleMessage(logger *log.Logger, state analysis.State, method string, contents []byte) {
	logger.Infof("Received message with method: %s", method)

	switch method {
	case "initialize":
		var request lsp.InitializeRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Errorf("Couldn't unmarshal contents for InitializeRequest: %s", err)
			panic(err)
		}

		logger.Infof("InitializeRequest. Client: %s %s", request.Params.ClientInfo.Name, request.Params.ClientInfo.Version)
		state.Root = request.Params.WorkspaceFolders
		roots := make([]string, 0, len(state.Root))
		for _, r := range state.Root {
			dir := fmt.Sprintf("%s/models", r.Name)
			roots = append(roots, dir)
		}
		state.ScanAndWatchDirs(roots)

		msg := lsp.NewInitializeResponse(request.ID)
		response, err := rpc.EncodeMsg(msg)
		if err != nil {
			logger.Errorf("Couldn't encode InitializeResponse: %s", err)
			panic(err)
		}

		state.Writer.Write([]byte(response))

		logger.Infof("Sent InitializeResponse id: %d", request.ID)

	case "textDocument/didOpen":
		var request lsp.DidOpenTextDocumentNotification
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Errorf("Couldn't unmarshal contents for DidOpenTextDocumentNotification: %s", err)
			panic(err)
		}

		logger.Infof("DidOpenTextDocumentNotification. %s", request.Params.TextDocument.URI)
		state.OpenDocument(request.Params.TextDocument.URI, request.Params.TextDocument.Text)

	case "textDocument/didChange":
		var request lsp.DidChangeTextDocumentNotification
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Errorf("Couldn't unmarshal contents for DidChangeTextDocumentNotification: %s", err)
			panic(err)
		}

		logger.Infof("DidChangeTextDocumentNotification. %s %v", request.Params.TextDocument.URI, request.Params.ContentChanges)
		for _, change := range request.Params.ContentChanges {
			logger.Debugf("Received change notification: %s", contents)
			state.UpdateDocument(request.Params.TextDocument.URI, change)
		}

	case "textDocument/willSave":
		var request lsp.WillSaveTextDocumentNotification
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Errorf("Couldn't unmarshal contents for WillSaveTextDocumentNotification: %s", err)
			panic(err)
		}

		logger.Infof("DidOpenTextDocumentNotification. %s %s", request.Params.TextDocument.URI, request.Params.TextDocument.Text)

	case "textDocument/completion":
		var request lsp.CompletionRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Errorf("Couldn't unmarshal contents for CompletionRequest: %s", err)
			panic(err)
		}

		logger.Infof("CompletionRequest. %s Line: %d, Char: %d", request.Params.TextDocument.URI, request.Params.Position.Line, request.Params.Position.Character)

		msg := state.TextDocumentCodeCompletion(request.ID, request.Params)
		response, err := rpc.EncodeMsg(msg)
		if err != nil {
			logger.Errorf("Couldn't rpc encode the CompletionResponse message: %s", err)
		}

		logger.Infof("CompletionResponse. %s", response)

		state.Writer.Write([]byte(response))
	}
}

func main() {
	var logFileFlag string
	flag.StringVar(&logFileFlag, "log-file", "", "Path to log file")
	flag.Parse()

	logger, err := logger.GetLogger(logFileFlag)
	if err != nil {
		log.Fatal(err)
	}
	logger.Info("dbt LSP started")

	writer := os.Stdout
	logger.Debug("Writer started")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Errorf("Error starting the Watcher. %s", err)
		log.Fatal(err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(rpc.Split)
	logger.Debug("Scanner started")

	state := analysis.NewState(logger, writer, watcher)
	logger.Debug("Server State initialized")

	// ensures the watcher is closed, even if it has to be reinitialized by the
	// WatchProject function error handling
	defer func() {
		if state.Watcher != nil {
			watcher.Close()
		}
	}()

	go state.WatchProject()

	logger.Debug("Scanning...")
	for scanner.Scan() {
		msg := scanner.Bytes()
		method, contents, err := rpc.DecodeMsg(msg)
		if err != nil {
			logger.Error(err)
			panic(err)
		}
		handleMessage(logger, state, method, contents)
	}
}

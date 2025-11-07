package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"dbt_lsp/analysis"
	"dbt_lsp/lsp"
	"dbt_lsp/rpc"

	"github.com/charmbracelet/log"
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

		logger.Infof("DidOpenTextDocumentNotification. %s %s", request.Params.TextDocument.URI, request.Params.TextDocument.Text)
		state.OpenDocument(request.Params.TextDocument.URI, request.Params.TextDocument.Text)

	case "textDocument/didChange":
		var request lsp.DidChangeTextDocumentNotification
		if err := json.Unmarshal(contents, &request); err != nil {
			logger.Errorf("Couldn't unmarshal contents for DidChangeTextDocumentNotification: %s", err)
			panic(err)
		}

		logger.Infof("DidChangeTextDocumentNotification. %s %s", request.Params.TextDocument.URI, request.Params.ContentChanges)
		for _, change := range request.Params.ContentChanges {
			state.OpenDocument(request.Params.TextDocument.URI, change.Text)
		}
	}
}

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

	writer := os.Stdout
	logger.Debug("Writer started")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(rpc.Split)


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

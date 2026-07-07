package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"dbt_ls/analysis"
	"dbt_ls/logger"
	"dbt_ls/lsp"
	"dbt_ls/rpc"

	"github.com/charmbracelet/log"
)

func isStateful(method string) bool {
	switch method {
	case "initialize",
		"textDocument/didOpen",
		"textDocument/didChange",
		"textDocument/willSave":
		return true
	default:
		return false
	}
}

func (p *InitProgram) handleStatelessEnvelope(state *analysis.State, envelope lsp.Envelope) {
	p.handleEnvelope(state, envelope)
}

func (p *InitProgram) sendErrorResponse(
	state *analysis.State,
	id int,
	code int,
	message string,
	context string,
) error {
	response, err := rpc.EncodeMsg(lsp.NewErrorResponse(id, code, message))
	if err != nil {
		return fmt.Errorf("couldn't encode %s error response: %w", context, err)
	}

	if _, err := state.Writer.Write([]byte(response)); err != nil {
		return fmt.Errorf("couldn't write %s error response: %w", context, err)
	}
	return nil
}

func (p *InitProgram) handleEnvelope(state *analysis.State, envelope lsp.Envelope) {
	switch envelope.Message.(type) {
	case lsp.InitializeRequest:
		msgIn := envelope.Message.(lsp.InitializeRequest)
		clientName, clientVersion := "unknown", ""
		if msgIn.Params.ClientInfo != nil {
			clientName = msgIn.Params.ClientInfo.Name
			clientVersion = msgIn.Params.ClientInfo.Version
		}
		p.Logger.Infof("InitializeRequest. Client: %s %s", clientName, clientVersion)

		if err := p.checkRoot(state, msgIn); err != nil {
			panic(err)
		}

		if err := p.parseRootURI(state, msgIn); err != nil {
			p.Logger.Error(err)
			return
		}

		state.ServerActive = false
		if len(state.RootPaths) > 0 {
			rootPath := state.RootPaths[0]
			if ok := state.IsDbtProject(rootPath); ok {
				p.Logger.Infof("Root status: %t", ok)
				state.ServerActive = true
			}

			if state.ServerActive {
				if err := state.ScanRootPath(rootPath); err != nil {
					p.Logger.Errorf("Error scanning workspace root %s: %s", rootPath, err)
				}
			}
		}
		p.Logger.Debugf("InitializeRequest. ServerActive status: %t", state.ServerActive)

		msgOut := lsp.NewInitializeResponse(msgIn.ID)
		response, err := rpc.EncodeMsg(msgOut)
		if err != nil {
			err := fmt.Errorf("couldn't encode InitializeResponse: %s", err)
			p.Logger.Error(err)
			p.Logger.Error("Closing dbt-ls")
			panic(err)
		}

		if _, err := state.Writer.Write([]byte(response)); err != nil {
			p.Logger.Errorf("couldn't write InitializeResponse: %s", err)
		}
		p.Logger.Infof("sent InitializeResponse id: %d", msgIn.ID)

	case lsp.DidOpenTextDocumentNotification:
		if !state.ServerActive {
			return
		}

		msgIn := envelope.Message.(lsp.DidOpenTextDocumentNotification)
		p.Logger.Debugf("DidOpenTextDocumentNotification. %s", msgIn.Params.TextDocument.URI)
		state.OpenDocument(
			msgIn.Params.TextDocument.URI,
			msgIn.Params.TextDocument.Text,
			msgIn.Params.TextDocument.Version,
		)

	case lsp.DidChangeTextDocumentNotification:
		if !state.ServerActive {
			return
		}

		msgIn := envelope.Message.(lsp.DidChangeTextDocumentNotification)
		p.Logger.Debugf(
			"DidChangeTextDocumentNotification. %s %v",
			msgIn.Params.TextDocument.URI,
			msgIn.Params.ContentChanges,
		)
		for _, change := range msgIn.Params.ContentChanges {
			p.Logger.Debugf("Received change notification: %s", envelope.Contents)
			state.UpdateDocument(
				msgIn.Params.TextDocument.URI,
				change,
				msgIn.Params.TextDocument.Version,
			)
		}

	case lsp.WillSaveTextDocumentNotification:
		if !state.ServerActive {
			return
		}

		msgIn := envelope.Message.(lsp.WillSaveTextDocumentNotification)
		// TODO
		p.Logger.Debugf(
			"WillSaveTextDocumentNotification. %s %s",
			msgIn.Params.TextDocument.URI,
			msgIn.Params.TextDocument.Text,
		)

	case lsp.CompletionRequest:
		msgIn := envelope.Message.(lsp.CompletionRequest)

		p.Logger.Debugf(
			"CompletionRequest. %s Line: %d, Char: %d",
			msgIn.Params.TextDocument.URI,
			msgIn.Params.Position.Line,
			msgIn.Params.Position.Character,
		)

		msg := state.TextDocumentCodeCompletion(msgIn.ID, msgIn.Params)
		response, err := rpc.EncodeMsg(msg)
		if err != nil {
			p.Logger.Errorf("Couldn't rpc encode the CompletionResponse message: %s", err)
		}

		p.Logger.Debugf("CompletionResponse. %s", response)

		state.Writer.Write([]byte(response))

	case lsp.DefinitionRequest:
		msgIn := envelope.Message.(lsp.DefinitionRequest)

		p.Logger.Debugf(
			"DefinitionRequest. %s Line: %d, Char: %d",
			msgIn.Params.TextDocument.URI,
			msgIn.Params.Position.Line,
			msgIn.Params.Position.Character,
		)

		msg := state.TextDocumentGoToDefinition(msgIn.ID, msgIn.Params)
		response, err := rpc.EncodeMsg(msg)
		if err != nil {
			p.Logger.Errorf("Couldn't rpc encode the CompletionResponse message: %s", err)
		}

		p.Logger.Debugf("CompletionResponse. %s", response)
		state.Writer.Write([]byte(response))
	}
}

func (p *InitProgram) parseEnvelope(method string, contents []byte) (lsp.Envelope, error) {
	p.Logger.Infof("Received message with method: %s", method)
	var envelope lsp.Envelope
	envelope.Method = method
	envelope.Contents = contents

	switch method {
	case "initialize":
		var request lsp.InitializeRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			p.Logger.Errorf("Couldn't unmarshal contents for InitializeRequest: %s", err)
			return lsp.Envelope{}, err
		}

		p.Logger.Debugf("Created envelope for request ID %d, method %s", request.ID, method)
		envelope = lsp.Envelope{
			Message: request,
		}
	case "textDocument/didOpen":
		var notification lsp.DidOpenTextDocumentNotification
		if err := json.Unmarshal(contents, &notification); err != nil {
			p.Logger.Errorf(
				"Couldn't unmarshal contents for DidOpenTextDocumentNotification: %s",
				err,
			)
			return lsp.Envelope{}, err
		}

		p.Logger.Debugf("Created envelope for notification method %s", method)
		envelope = lsp.Envelope{
			Message: notification,
		}
	case "textDocument/didChange":
		var notification lsp.DidChangeTextDocumentNotification
		if err := json.Unmarshal(contents, &notification); err != nil {
			p.Logger.Errorf(
				"Couldn't unmarshal contents for DidChangeTextDocumentNotification: %s",
				err,
			)
			return lsp.Envelope{}, err
		}

		p.Logger.Debugf("Created envelope for notification method %s", method)
		envelope = lsp.Envelope{
			Message: notification,
		}
	case "textDocument/willSave":
		var notification lsp.WillSaveTextDocumentNotification
		if err := json.Unmarshal(contents, &notification); err != nil {
			p.Logger.Errorf(
				"Couldn't unmarshal contents for WillSaveTextDocumentNotification: %s",
				err,
			)
			return lsp.Envelope{}, err
		}

		p.Logger.Debugf("Created envelope for notification method %s", method)
		envelope = lsp.Envelope{
			Message: notification,
		}
	case "textDocument/completion":
		var request lsp.CompletionRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			p.Logger.Errorf("Couldn't unmarshal contents for CompletionRequest: %s", err)
			return lsp.Envelope{}, err
		}

		p.Logger.Debugf("Created envelope for notification method %s", method)
		envelope = lsp.Envelope{
			Message: request,
		}

	case "textDocument/definition":
		var request lsp.DefinitionRequest
		if err := json.Unmarshal(contents, &request); err != nil {
			p.Logger.Errorf("Couldn't unmarshal contents for DefinitionRequest: %s", err)
			return lsp.Envelope{}, err
		}

		p.Logger.Debugf("Created envelope for notification method %s", method)
		envelope = lsp.Envelope{
			Message: request,
		}
	}

	return envelope, nil
}

func main() {
	pgm := InitProgram{}
	flag.StringVar(&pgm.logFileFlag, "log-file", "", "Path to log file")
	flag.StringVar(&pgm.logLevel, "log-level", "", "Set log level")
	flag.Parse()

	logger, err := logger.GetLogger(pgm.logFileFlag, pgm.logLevel)
	if err != nil {
		log.Fatal(err)
	}
	pgm.Logger = logger
	pgm.Logger.Info("dbt LSP started")

	writer := os.Stdout
	pgm.Logger.Debug("Writer started")

	projectWatcher, err := analysis.NewWatcher("project", "./models", logger)
	if err != nil {
		pgm.Logger.Fatalf("Error starting the projectWatcher. %s", err)
	}
	// ensures the watcher is closed, even if it has to be reinitialized by the
	// WatchProject function error handling
	defer projectWatcher.HandleAsyncClose(logger)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(rpc.Split)
	pgm.Logger.Debug("Scanner started")

	state := analysis.NewState(logger, writer, projectWatcher)
	pgm.Logger.Debug("Server State initialized")

	go state.WatchProject()
	go state.DrainNotifications()

	logger.Debug("Scanning Stdin for incoming messages")
	for scanner.Scan() {
		msg := scanner.Bytes()
		method, contents, err := rpc.DecodeMsg(msg)
		if err != nil {
			pgm.Logger.Errorf("Can't parse RPC message: %s", err)
			continue
		}

		envelope, err := pgm.parseEnvelope(method, contents)
		if err != nil {
			pgm.Logger.Errorf("Can't create envelope: %s", err)
			continue
		}

		// Synchronous for state-changing, async for stateless
		if isStateful(method) {
			pgm.handleEnvelope(state, envelope)
		} else {
			go pgm.handleStatelessEnvelope(state, envelope)
		}
	}
}

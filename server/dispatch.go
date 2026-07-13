package server

import (
	"encoding/json"
	"fmt"

	"dbt_ls/analysis"
	"dbt_ls/lsp"
	"dbt_ls/rpc"
)

type methodHandler struct {
	newMessage func() lsp.ClientMessage
	handle     func(*Server, *analysis.State, lsp.ClientMessage, []byte)
	stateful   bool
}

var methodHandlers = map[string]methodHandler{
	"initialize": {
		newMessage: func() lsp.ClientMessage { return &lsp.InitializeRequest{} },
		handle:     handleInitialize,
		stateful:   true,
	},
	"shutdown": {
		newMessage: func() lsp.ClientMessage { return &lsp.ShutdownRequest{} },
		handle:     handleShutdown,
		stateful:   true,
	},
	"exit": {
		newMessage: func() lsp.ClientMessage { return &lsp.ExitNotification{} },
		handle:     handleExit,
		stateful:   true,
	},
	"textDocument/didOpen": {
		newMessage: func() lsp.ClientMessage { return &lsp.DidOpenTextDocumentNotification{} },
		handle:     handleDidOpen,
		stateful:   true,
	},
	"textDocument/didChange": {
		newMessage: func() lsp.ClientMessage { return &lsp.DidChangeTextDocumentNotification{} },
		handle:     handleDidChange,
		stateful:   true,
	},
	"textDocument/willSave": {
		newMessage: func() lsp.ClientMessage { return &lsp.WillSaveTextDocumentNotification{} },
		handle:     handleWillSave,
		stateful:   true,
	},
	"textDocument/completion": {
		newMessage: func() lsp.ClientMessage { return &lsp.CompletionRequest{} },
		handle:     handleCompletion,
		stateful:   false,
	},
	"textDocument/definition": {
		newMessage: func() lsp.ClientMessage { return &lsp.DefinitionRequest{} },
		handle:     handleDefinition,
		stateful:   false,
	},
}

func (s *Server) Dispatch(state *analysis.State, method string, contents []byte) bool {
	handler, ok := methodHandlers[method]
	if !ok {
		return false
	}

	if handler.stateful {
		s.handleMethod(state, method, contents, handler)
	} else {
		go s.handleMethod(state, method, contents, handler)
	}

	return true
}

func (s *Server) handleMethod(
	state *analysis.State,
	method string,
	contents []byte,
	handler methodHandler,
) {
	s.Logger.Debugf("Received message with method: %s", method)
	msg := handler.newMessage()
	if err := json.Unmarshal(contents, msg); err != nil {
		s.Logger.Errorf("Couldn't unmarshal contents for %s: %s", method, err)
		return
	}

	s.Logger.Debugf("Dispatching method %s", method)
	handler.handle(s, state, msg, contents)
}

func (s *Server) sendErrorResponse(
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

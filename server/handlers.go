package server

import (
	"fmt"

	"github.com/fsorodrigues/dbt-ls/analysis"
	"github.com/fsorodrigues/dbt-ls/lsp"
	"github.com/fsorodrigues/dbt-ls/rpc"
)

func handleInitialize(s *Server, state *analysis.State, raw lsp.ClientMessage, _ []byte) {
	msgIn := raw.(*lsp.InitializeRequest)
	clientName, clientVersion := "unknown", ""
	if msgIn.Params.ClientInfo != nil {
		clientName = msgIn.Params.ClientInfo.Name
		clientVersion = msgIn.Params.ClientInfo.Version
	}
	s.Logger.Infof("InitializeRequest. Client: %s %s", clientName, clientVersion)

	if err := s.checkRoot(state, *msgIn); err != nil {
		panic(err)
	}

	if err := s.parseRootURI(state, *msgIn); err != nil {
		s.Logger.Error(err)
		return
	}

	state.SetServerActive(false)
	if len(state.RootPaths) > 0 {
		rootPath := state.RootPaths[0]
		project, err := state.ParseDbtConfig(rootPath)
		if err != nil {
			s.Logger.Errorf("Unable to initialize dbt project: %s", err)
			state.SetProjectRoot(rootPath)
			state.NotifyProject("dbt-ls: no dbt project identified; server inactive")
		} else {
			state.ActivateProject(rootPath, state.GetProjectConfigPath(rootPath), project)
			s.Logger.Info("Root status: true")
		}

		state.AddWatchDir(rootPath)
		s.startBackground(state)
	}
	s.Logger.Debugf("InitializeRequest. ServerActive status: %t", state.IsServerActive())

	msgOut := lsp.NewInitializeResponse(msgIn.ID)
	response, err := rpc.EncodeMsg(msgOut)
	if err != nil {
		err := fmt.Errorf("couldn't encode InitializeResponse: %s", err)
		s.Logger.Error(err)
		s.Logger.Error("Closing dbt-ls")
		panic(err)
	}

	if _, err := state.Writer.Write([]byte(response)); err != nil {
		s.Logger.Errorf("couldn't write InitializeResponse: %s", err)
	}
	s.Logger.Infof("sent InitializeResponse id: %d", msgIn.ID)
}

func handleShutdown(s *Server, state *analysis.State, raw lsp.ClientMessage, _ []byte) {
	msgIn := raw.(*lsp.ShutdownRequest)

	s.startShutdown(state)
	s.Logger.Debugf("ShutdownRequest. ServerActive status: %t", state.IsServerActive())

	msgOut := lsp.NewShutdownResponse(msgIn.ID)
	response, err := rpc.EncodeMsg(msgOut)
	if err != nil {
		err := fmt.Errorf("couldn't encode ShutdownResponse: %s", err)
		s.Logger.Error(err)
		return
	}

	if _, err := state.Writer.Write([]byte(response)); err != nil {
		s.Logger.Errorf("couldn't write ShutdownResponse %s", err)
	}
	s.Logger.Infof("sent ShutdownResponse id: %d", msgIn.ID)
}

func handleExit(s *Server, state *analysis.State, _ lsp.ClientMessage, contents []byte) {
	if s.Cancel != nil {
		s.Cancel()
	}
	state.ProjectWatcher.Close()

	if state.ShutdownRequested {
		s.requestExit(0)
		return
	}
	s.requestExit(1)
}

func handleDidOpen(s *Server, state *analysis.State, raw lsp.ClientMessage, _ []byte) {
	if !state.IsServerActive() {
		return
	}

	msgIn := raw.(*lsp.DidOpenTextDocumentNotification)
	s.Logger.Debugf("DidOpenTextDocumentNotification. %s", msgIn.Params.TextDocument.URI)
	state.OpenDocument(
		msgIn.Params.TextDocument.URI,
		msgIn.Params.TextDocument.Text,
		msgIn.Params.TextDocument.Version,
	)
}

func handleDidChange(s *Server, state *analysis.State, raw lsp.ClientMessage, contents []byte) {
	if !state.IsServerActive() {
		return
	}

	msgIn := raw.(*lsp.DidChangeTextDocumentNotification)
	s.Logger.Tracef(
		"DidChangeTextDocumentNotification. %s %v",
		msgIn.Params.TextDocument.URI,
		msgIn.Params.ContentChanges,
	)
	for _, change := range msgIn.Params.ContentChanges {
		s.Logger.Tracef("Received change notification: %s", contents)
		state.UpdateDocument(
			msgIn.Params.TextDocument.URI,
			change,
			msgIn.Params.TextDocument.Version,
		)
	}
}

func handleWillSave(s *Server, state *analysis.State, raw lsp.ClientMessage, _ []byte) {
	if !state.IsServerActive() {
		return
	}

	msgIn := raw.(*lsp.WillSaveTextDocumentNotification)
	// TODO
	s.Logger.Tracef(
		"WillSaveTextDocumentNotification. %s %s",
		msgIn.Params.TextDocument.URI,
		msgIn.Params.TextDocument.Text,
	)
}

func handleCompletion(s *Server, state *analysis.State, raw lsp.ClientMessage, _ []byte) {
	msgIn := raw.(*lsp.CompletionRequest)

	s.Logger.Tracef(
		"CompletionRequest. %s Line: %d, Char: %d",
		msgIn.Params.TextDocument.URI,
		msgIn.Params.Position.Line,
		msgIn.Params.Position.Character,
	)

	msg := state.TextDocumentCodeCompletion(msgIn.ID, msgIn.Params)
	response, err := rpc.EncodeMsg(msg)
	if err != nil {
		s.Logger.Errorf("Couldn't rpc encode the CompletionResponse message: %s", err)
	}

	s.Logger.Tracef("CompletionResponse. %s", response)
	state.Writer.Write([]byte(response))
}

func handleDefinition(s *Server, state *analysis.State, raw lsp.ClientMessage, _ []byte) {
	msgIn := raw.(*lsp.DefinitionRequest)

	s.Logger.Tracef(
		"DefinitionRequest. %s Line: %d, Char: %d",
		msgIn.Params.TextDocument.URI,
		msgIn.Params.Position.Line,
		msgIn.Params.Position.Character,
	)

	msg := state.TextDocumentGoToDefinition(msgIn.ID, msgIn.Params)
	response, err := rpc.EncodeMsg(msg)
	if err != nil {
		s.Logger.Errorf("Couldn't rpc encode the CompletionResponse message: %s", err)
	}

	s.Logger.Tracef("CompletionResponse. %s", response)
	state.Writer.Write([]byte(response))
}

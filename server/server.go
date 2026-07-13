package server

import (
	"context"
	"errors"
	"fmt"

	"dbt_ls/analysis"
	"dbt_ls/logger"
	"dbt_ls/lsp"
)

type Server struct {
	Logger   *logger.Logger
	Cancel   context.CancelFunc
	ExitCode *int
}

func (s *Server) checkRoot(state *analysis.State, msgIn lsp.InitializeRequest) error {
	state.Root = msgIn.Params.WorkspaceFolders

	if len(state.Root) > 1 {
		message := "dbt-ls does not support multi-root workspaces"
		s.Logger.Error(message)
		if err := s.sendErrorResponse(
			state,
			msgIn.ID,
			lsp.ErrorCodeInvalidParams,
			message,
			"initialize",
		); err != nil {
			s.Logger.Error(err)
			s.Logger.Error("Closing dbt-ls")
			return err
		}
		return errors.New(message)
	}

	return nil
}

func (s *Server) parseRootURI(state *analysis.State, msgIn lsp.InitializeRequest) error {
	state.RootPaths = make([]string, len(state.Root))
	for i, r := range state.Root {
		path, err := analysis.WorkspacePath(r.URI)
		if err != nil {
			message := fmt.Sprintf("Failed to parse workspace folder URI %q: %v", r.URI, err)
			s.Logger.Error(message)
			if err := s.sendErrorResponse(
				state,
				msgIn.ID,
				lsp.ErrorCodeInvalidParams,
				message,
				"initialize",
			); err != nil {
				s.Logger.Error(err)
				s.Logger.Error("Closing dbt-ls")
				return err
			}
			return err
		}
		state.RootPaths[i] = path
		s.Logger.Infof("Root: %s (%s)", r.Name, path)
	}
	return nil
}

func (s *Server) startShutdown(state *analysis.State) {
	state.Shutdown()
	if s.Cancel != nil {
		s.Cancel()
	}
}

func (s *Server) requestExit(code int) {
	s.ExitCode = &code
	if s.Cancel != nil {
		s.Cancel()
	}
}

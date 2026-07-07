package main

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/log"

	"dbt_ls/analysis"
	"dbt_ls/lsp"
)

type InitProgram struct {
	logFileFlag string
	logLevel    string
	Logger      *log.Logger
}

type Program interface {
	handleEnvelope()
	handleStatelessEnvelope()
	parseEnvelope(string, []byte) lsp.Envelope
	checkRoot(*analysis.State, lsp.InitializeRequest) error
	parseRootURI(*analysis.State, lsp.InitializeRequest) error
}

func (p *InitProgram) checkRoot(state *analysis.State, msgIn lsp.InitializeRequest) error {
	state.Root = msgIn.Params.WorkspaceFolders

	if len(state.Root) > 1 {
		message := "dbt-ls does not support multi-root workspaces"
		p.Logger.Error(message)
		if err := p.sendErrorResponse(
			state,
			msgIn.ID,
			lsp.ErrorCodeInvalidParams,
			message,
			"initialize",
		); err != nil {
			p.Logger.Error(err)
			p.Logger.Error("Closing dbt-ls")
			return err
		}
		return errors.New(message)
	}

	return nil
}

func (p *InitProgram) parseRootURI(state *analysis.State, msgIn lsp.InitializeRequest) error {
	state.RootPaths = make([]string, len(state.Root))
	for i, r := range state.Root {
		path, err := analysis.WorkspacePath(r.URI)
		if err != nil {
			message := fmt.Sprintf("Failed to parse workspace folder URI %q: %v", r.URI, err)
			p.Logger.Error(message)
			if err := p.sendErrorResponse(
				state,
				msgIn.ID,
				lsp.ErrorCodeInvalidParams,
				message,
				"initialize",
			); err != nil {
				p.Logger.Error(err)
				p.Logger.Error("Closing dbt-ls")
				return err
			}
			return err
		}
		state.RootPaths[i] = path
		p.Logger.Infof("Root: %s (%s)", r.Name, path)
	}
	return nil
}

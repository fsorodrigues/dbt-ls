package analysis

import (
	"fmt"
	"strings"

	"github.com/fsorodrigues/dbt-ls/jinja"
	"github.com/fsorodrigues/dbt-ls/lsp"
)

func (s *State) TextDocumentGoToDefinition(
	id int,
	params lsp.DefinitionParams,
) lsp.DefinitionResponse {
	response := lsp.DefinitionResponse{
		Response: lsp.Response{
			RPC: "2.0",
			ID:  &id,
		},
	}

	refEnabled := s.IsRefDefinitionEnabled()
	macrosEnabled := s.IsMacrosDefinitionEnabled()

	s.Logger.Debugf("Snapshotting text document: %s", params.TextDocument.URI)
	snap, ok := s.Snapshot(params.TextDocument.URI)
	if !ok {
		s.Logger.Errorf("Could not snapshot: %s", params.TextDocument.URI)
		return response
	}

	cursor := snap.Offset(params.Position)
	ctx := jinja.Classify(snap.Text, cursor)
	name := string(snap.Text[ctx.Start:ctx.End])

	if name == "" {
		s.Logger.Tracef("GoTo Ctx: %+v", ctx)
		s.Logger.Errorf("GoTo Definition request name is empty: %s", name)
		return response
	}

	switch ctx.Role {
	case jinja.RoleStringArg:
		if !refEnabled {
			s.Logger.Tracef("RefsDefinition not enabled: %t", refEnabled)
			return response
		} else {
			s.Logger.Tracef("Ref GoTo Ctx: %+v", ctx)
			s.Logger.Debugf("Ref GoTo Definition request name: %s", name)

			model, ok := s.DbtModels.Get(strings.ToLower(name))
			if ok {
				s.Logger.Tracef("Model found: %s", model)
				response.Result = &lsp.DefinitionLocation{
					TextDocumentIdentifier: lsp.TextDocumentIdentifier{
						URI: constructPathAsUri(model),
					},
					Range: lsp.TextDocumentPositionRange{
						Start: lsp.TextDocumentPosition{Line: 0, Character: 0},
						End:   lsp.TextDocumentPosition{Line: 0, Character: 0},
					},
				}
			} else {
				s.Logger.Errorf("Did not find model: %s", name)
			}
		}
	case jinja.RoleValue:
		if !macrosEnabled {
			s.Logger.Tracef("MacrosDefinition not enabled: %t", refEnabled)
			return response
		} else {
			s.Logger.Tracef("Macro GoTo Ctx: %+v", ctx)
			s.Logger.Debugf("Macro GoTo Definition request name: %s", name)

			s.DbtMacrosMu.Lock()
			macro, ok := s.DbtMacros.Get(strings.ToLower(name))
			s.DbtMacrosMu.Unlock()

			if ok && !macro.Builtin && macro.File != "" {
				s.Logger.Tracef("Macro found: %+v", macro)
				response.Result = &lsp.DefinitionLocation{
					TextDocumentIdentifier: lsp.TextDocumentIdentifier{
						URI: constructPathAsUri(macro.File),
					},
					Range: lsp.TextDocumentPositionRange{
						Start: lsp.TextDocumentPosition{Line: macro.Line, Character: 0},
						End:   lsp.TextDocumentPosition{Line: 0, Character: 0},
					},
				}
			} else {
				s.Logger.Errorf("Did not find macro: %s", name)
			}
		}
	}

	return response
}

func constructPathAsUri(s string) string {
	return fmt.Sprintf("file://%s", s)
}

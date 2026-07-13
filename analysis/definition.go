package analysis

import (
	"fmt"
	"strings"

	"dbt_ls/lsp"
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
	if !s.ServerActive {
		return response
	}

	doc, ok := s.Documents[params.TextDocument.URI]
	if !ok || doc == nil {
		s.Logger.Errorf("Definition requested for unopened document: %s", params.TextDocument.URI)
		return response
	}
	line := getLine(doc.Data, params.Position.Line)
	s.Logger.Tracef("Looking for prefix with model reference in line %s", line)
	modelRef, check := extractModelRefUnderCursor(string(line), params.Position)

	if check {
		s.Logger.Tracef("Found model reference %s in line", modelRef)
		model, ok := s.DbtModels.Get(strings.ToLower(modelRef))
		if ok {
			response.Result = &lsp.DefinitionLocation{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{
					URI: constructRefAsUri(model),
				},
				Range: lsp.TextDocumentPositionRange{
					Start: lsp.TextDocumentPosition{Line: 0, Character: 0},
					End:   lsp.TextDocumentPosition{Line: 0, Character: 0},
				},
			}
		} else {
			s.Logger.Errorf("No model with prefix %s found", modelRef)
		}

	} else {
		s.Logger.Errorf("No ref prefix found on line: %s", line)
	}

	return response
}

func constructRefAsUri(s string) string {
	return fmt.Sprintf("file://%s", s)
}

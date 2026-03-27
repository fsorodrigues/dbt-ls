package analysis

import (
	"fmt"
	"regexp"
	"strings"

	"dbt_ls/lsp"
)

func (s *State) createRefResponse(lineContent string, params lsp.CompletionParams, response *lsp.CompletionResponse) {
	modelRef, check := extractModelRefUnderCursor(lineContent, params.Position)
	s.Logger.Debugf("Check: %t. Search: %s. Models: %+v", check, modelRef, s.DbtModels)

	if check {
		models := s.DbtModels.KeysWithPrefix(modelRef)
		s.Logger.Debugf("Found: %s", models)
		for _, modKey := range models {
			modVal, ok := s.DbtModels.Get(modKey)
			if !ok {
				s.Logger.Error("Error getting value from Trie")
			}
			response.Result.Items = append(response.Result.Items, lsp.CompletionItem{
				Label:         modKey,
				Kind:          18,
				Detail:        "dbt Model",
				Documentation: modVal,
				TextEdit: lsp.CompletionTextEdit{
					Range: lsp.TextDocumentPositionRange{
						Start: params.Position,
						End:   params.Position,
					},
					NewText: strings.TrimPrefix(modKey, modelRef),
				},
			})
		}
		s.Logger.Debugf("TextDocumentCodeCompletion (Ref) ready. Contains %d items", len(response.Result.Items))
	} else {
		s.Logger.Debugf("Cannot parse line contents for Ref completion: %s", lineContent)
	}
}

func NewCompletionResponse(id int) *lsp.CompletionResponse {
	return &lsp.CompletionResponse{
		Response: lsp.Response{
			RPC: "2.0",
			ID:  &id,
		},
		Result: lsp.CompletionList{
			IsIncomplete: true,
			Items:        []lsp.CompletionItem{},
		},
	}
}

func (s *State) TextDocumentCodeCompletion(id int, params lsp.CompletionParams) lsp.CompletionResponse {
	doc := s.Documents[params.TextDocument.URI]
	line := getLine(doc.Data, params.Position.Line)
	response := NewCompletionResponse(id)
	completionType, err := parseCompletionType(line)

	if err == nil {
		s.Logger.Debugf("Completion Type: %s", completionType)
		switch completionType {
		case "ref":
			s.createRefResponse(line, params, response)
		}
	}

	return *response
}

var (
	sourceRe = regexp.MustCompile(`\s*\bsource\s*\(`)
	refRe    = regexp.MustCompile(`\s*\bref\s*\(`)
)

func parseCompletionType(lineContent string) (string, error) {
	if sourceRe.MatchString(lineContent) {
		return "source", nil
	}
	if refRe.MatchString(lineContent) {
		return "ref", nil
	}

	return "", fmt.Errorf(
		"Cannot determine what type of completion request this should be. Line %s",
		lineContent,
	)
}

func extractRefPrefix(text string) (string, bool) {
	// Matches ref("prefix or ref('prefix
	re := regexp.MustCompile(`.*ref\(['"]([a-zA-Z0-9_\-]*)`)
	match := re.FindStringSubmatch(text)
	if match == nil {
		return "", false
	}

	return match[1], true
}

func extractModelRefUnderCursor(lineContent string, position lsp.TextDocumentPosition) (string, bool) {
	// Group 1: Matches content inside single quotes
	// Group 2: Matches content inside double quotes
	re := regexp.MustCompile(`ref\s*\(\s*(?:'([^']+)'|"([^"]+)")\s*\)`)
	matches := re.FindAllStringSubmatchIndex(lineContent, -1)

	var modelName string
	foundMatch := false

	// match indices reference:
	// match[0], match[1]: Start/End of full match
	// match[2], match[3]: Start/End of group 1 (single quotes, -1 if no match)
	// match[4], match[5]: Start/End of group 2 (double quotes, -1 if no match)
	for _, match := range matches {
		start, end := match[0], match[1]

		// Check if cursor is within the bounds of this ref(...) call
		if start <= position.Character && position.Character <= end {
			if match[2] != -1 {
				modelName = lineContent[match[2]:match[3]]
			} else if match[4] != -1 {
				modelName = lineContent[match[4]:match[5]]
			}
			foundMatch = true
			break
		}
	}

	return modelName, foundMatch
}

func constructRefAsUri(s string) string {
	return fmt.Sprintf("file://%s", s)
}

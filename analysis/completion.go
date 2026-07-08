package analysis

import (
	"fmt"
	"regexp"
	"strings"

	"dbt_ls/lsp"
)

type SourceCompletionContext struct {
	Kind        string
	SourceName  string
	TablePrefix string
}

func (s *State) createRefResponse(lineContent string, params lsp.CompletionParams, response *lsp.CompletionResponse) {
	modelRef, check := extractModelRefUnderCursor(lineContent, params.Position)
	s.Logger.Debugf("Check: %t. Search: %s. Models: %+v", check, modelRef, s.DbtModels)

	if check {
		models := s.DbtModels.KeysWithPrefix(strings.ToLower(modelRef))
		s.Logger.Debugf("Found: %s", models)
		for _, modKey := range models {
			modVal, ok := s.DbtModels.Get(modKey)
			if !ok {
				s.Logger.Error("Error getting value from Trie")
			}
			response.Result.Items = append(response.Result.Items, lsp.CompletionItem{
				Label:         modKey,
				Kind:          lsp.CompletionItemKindReference,
				Detail:        "dbt Model",
				Documentation: modVal,
				TextEdit: lsp.CompletionTextEdit{
					Range: lsp.TextDocumentPositionRange{
						Start: lsp.TextDocumentPosition{
							Line:      params.Position.Line,
							Character: params.Position.Character - len(modelRef),
						},
						End: params.Position,
					},
					NewText: modKey,
				},
			})
		}
		s.Logger.Debugf("TextDocumentCodeCompletion (Ref) ready. Contains %d items", len(response.Result.Items))
	} else {
		s.Logger.Debugf("Cannot parse line contents for Ref completion: %s", lineContent)
	}
}

func (s *State) createSourceResponse(lineContent string, params lsp.CompletionParams, response *lsp.CompletionResponse) {
	ctx, check := extractSourceContextUnderCursor(lineContent, params.Position)
	s.Logger.Debugf("Source context: %+v, check: %t", ctx, check)
	if !check {
		s.Logger.Debugf("Cannot parse line contents for Source completion: %s", lineContent)
		return
	}

	s.DbtConfigMu.Lock()
	defer s.DbtConfigMu.Unlock()

	if !s.SourcesValid {
		s.Logger.Debugf("Source config is invalid; skipping source completion")
		return
	}

	switch ctx.Kind {
	case "source_name":
		prefix := ctx.SourceName
		for _, name := range sourceNamesWithPrefix(s.DbtConfig, prefix) {
			response.Result.Items = append(response.Result.Items, lsp.CompletionItem{
				Label:  name,
				Kind:   lsp.CompletionItemKindReference,
				Detail: "dbt Source",
				TextEdit: lsp.CompletionTextEdit{
					Range: lsp.TextDocumentPositionRange{
						Start: lsp.TextDocumentPosition{
							Line:      params.Position.Line,
							Character: params.Position.Character - len(prefix),
						},
						End: params.Position,
					},
					NewText: name,
				},
			})
		}
	case "table_name":
		src := sourceByName(s.DbtConfig, ctx.SourceName)
		if src == nil {
			s.Logger.Debugf("No source named %q found for table completion", ctx.SourceName)
			return
		}
		prefix := ctx.TablePrefix
		for _, tblName := range tableNamesWithPrefix(src, prefix) {
			response.Result.Items = append(response.Result.Items, lsp.CompletionItem{
				Label:  tblName,
				Kind:   lsp.CompletionItemKindReference,
				Detail: "dbt Source Table",
				TextEdit: lsp.CompletionTextEdit{
					Range: lsp.TextDocumentPositionRange{
						Start: lsp.TextDocumentPosition{
							Line:      params.Position.Line,
							Character: params.Position.Character - len(prefix),
						},
						End: params.Position,
					},
					NewText: tblName,
				},
			})
		}
	}

	s.Logger.Debugf("TextDocumentCodeCompletion (Source) ready. Contains %d items", len(response.Result.Items))
}

func sourceNamesWithPrefix(cfg DbtConfig, prefix string) []string {
	var names []string
	for name := range cfg.Sources {
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			names = append(names, name)
		}
	}
	return names
}

func sourceByName(cfg DbtConfig, name string) *DbtConfigSource {
	src, ok := cfg.Sources[name]
	if !ok {
		return nil
	}
	return src
}

func tableNamesWithPrefix(src *DbtConfigSource, prefix string) []string {
	var names []string
	for name := range src.Tables {
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			names = append(names, name)
		}
	}
	return names
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
	response := NewCompletionResponse(id)
	if !s.ServerActive {
		return *response
	}

	doc, ok := s.Documents[params.TextDocument.URI]
	if !ok || doc == nil {
		s.Logger.Errorf("Completion requested for unopened document: %s", params.TextDocument.URI)
		return *response
	}
	line := getLine(doc.Data, params.Position.Line)
	completionType, err := parseCompletionType(line)

	if err == nil {
		s.Logger.Debugf("Completion Type: %s", completionType)
		switch completionType {
		case "ref":
			s.createRefResponse(line, params, response)
		case "source":
			s.createSourceResponse(line, params, response)
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

// extractSourceContextUnderCursor returns the completion context for a
// source(...) call, determining whether the cursor is positioned on the
// source-name argument or the table-name argument.
func extractSourceContextUnderCursor(lineContent string, position lsp.TextDocumentPosition) (SourceCompletionContext, bool) {
	// Matches: source( <arg1> , <arg2> )
	// We look for the cursor being inside arg1 (source name) or arg2 (table name).
	re := regexp.MustCompile(`source\s*\(\s*(?:'([^']*)'|"([^"]*)")\s*(?:,\s*(?:'([^']*)'|"([^"]*)")\s*)?\)`)
	matches := re.FindAllStringSubmatchIndex(lineContent, -1)

	for _, match := range matches {
		fullStart, fullEnd := match[0], match[1]
		if position.Character < fullStart || position.Character > fullEnd {
			continue
		}

		// Determine source name bounds (group 1 single / group 2 double)
		srcStart, srcEnd := match[2], match[3]
		if srcStart == -1 {
			srcStart, srcEnd = match[4], match[5]
		}

		// Determine table name bounds (group 3 single / group 4 double)
		tblStart, tblEnd := match[6], match[7]
		if tblStart == -1 {
			tblStart, tblEnd = match[8], match[9]
		}

		if srcStart != -1 && position.Character >= srcStart && position.Character <= srcEnd {
			return SourceCompletionContext{
				Kind:       "source_name",
				SourceName: lineContent[srcStart:position.Character],
			}, true
		}

		if tblStart != -1 && position.Character >= tblStart && position.Character <= tblEnd {
			var srcName string
			if match[2] != -1 {
				srcName = lineContent[match[2]:match[3]]
			} else if match[4] != -1 {
				srcName = lineContent[match[4]:match[5]]
			}
			return SourceCompletionContext{
				Kind:        "table_name",
				SourceName:  srcName,
				TablePrefix: lineContent[tblStart:position.Character],
			}, true
		}
	}

	// Fallback: try partial match (source name not yet closed)
	rePartial := regexp.MustCompile(`source\s*\(\s*['"]([^'"]*)$`)
	partialMatch := rePartial.FindStringSubmatchIndex(lineContent[:position.Character])
	if partialMatch != nil {
		return SourceCompletionContext{
			Kind:       "source_name",
			SourceName: lineContent[partialMatch[2]:position.Character],
		}, true
	}

	// Fallback: table name partial (source name closed, comma seen, table not closed)
	rePartialTable := regexp.MustCompile(`source\s*\(\s*(?:'([^']*)'|"([^"]*)")\s*,\s*['"]([^'"]*)$`)
	partialTableMatch := rePartialTable.FindStringSubmatchIndex(lineContent[:position.Character])
	if partialTableMatch != nil {
		srcName := ""
		if partialTableMatch[2] != -1 {
			srcName = lineContent[partialTableMatch[2]:partialTableMatch[3]]
		} else if partialTableMatch[4] != -1 {
			srcName = lineContent[partialTableMatch[4]:partialTableMatch[5]]
		}
		tablePrefix := lineContent[partialTableMatch[6]:position.Character]
		return SourceCompletionContext{
			Kind:        "table_name",
			SourceName:  srcName,
			TablePrefix: tablePrefix,
		}, true
	}

	return SourceCompletionContext{}, false
}

package analysis

import (
	"strings"

	"github.com/fsorodrigues/dbt-ls/jinja"
	"github.com/fsorodrigues/dbt-ls/lsp"
)

func extractModelRefUnderCursor(a string, b lsp.TextDocumentPosition) (string, bool) {
	return "a", false
}

func (s *State) createRefResponse(
	snapshot Snapshot,
	ctx jinja.Context,
	response *lsp.CompletionResponse,
) {
	s.Logger.Tracef("Ref search prefix: %s", ctx.Prefix)
	models := s.DbtModels.KeysWithPrefix(strings.ToLower(ctx.Prefix))

	if len(models) > 0 {
		s.Logger.Debugf("Found %d models: %+v", len(models), models)

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
					Range:   snapshot.Range(ctx.Start, ctx.End),
					NewText: modKey,
				},
			})
		}
		s.Logger.Debugf(
			"TextDocumentCodeCompletion (Ref) ready. Contains %d items",
			len(response.Result.Items),
		)
	} else {
		s.Logger.Debugf("No models found for prefix %s", ctx.Prefix)
		s.Logger.Trace("Context: %+v", ctx)
	}
}

func (s *State) createSourceResponse(
	snapshot Snapshot,
	ctx jinja.Context,
	response *lsp.CompletionResponse,
) {
	s.DbtConfigMu.RLock()
	defer s.DbtConfigMu.RUnlock()

	switch ctx.ArgIndex {
	case 0:
		s.Logger.Tracef("Source Name search prefix: %s", ctx.Prefix)
		s.createSourceNameResponse(snapshot, ctx, response)
	case 1:
		s.Logger.Tracef("Source Table search prefix: %s", ctx.Prefix)
		s.createSourceTableResponse(snapshot, ctx, response)
	}
}

// createSourceNameResponse completes the first argument of source(), i.e.
// the source name, e.g. source('src|').
func (s *State) createSourceNameResponse(
	snapshot Snapshot,
	ctx jinja.Context,
	response *lsp.CompletionResponse,
) {
	sources := sourceNamesWithPrefix(s.DbtConfig, ctx.Prefix)
	if len(sources) > 0 {
		s.Logger.Debugf("Found %d sources", len(sources))
		s.Logger.Tracef("Sources: %+v", sources)
		for _, name := range sources {
			response.Result.Items = append(response.Result.Items, lsp.CompletionItem{
				Label:  name,
				Kind:   lsp.CompletionItemKindReference,
				Detail: "dbt Source",
				TextEdit: lsp.CompletionTextEdit{
					Range:   snapshot.Range(ctx.Start, ctx.End),
					NewText: name,
				},
			})
		}
	} else {
		s.Logger.Debugf("No sources found for prefix %s", ctx.Prefix)
	}
}

// createSourceTableResponse completes the second argument of source(), i.e.
// the table name, e.g. source('src', 'tbl|'). It resolves the enclosing
// source from the first argument, captured in ctx.PreviousArgs.
func (s *State) createSourceTableResponse(
	snapshot Snapshot,
	ctx jinja.Context,
	response *lsp.CompletionResponse,
) {
	if len(ctx.PreviousArgs) == 0 || ctx.PreviousArgs[0] == "" {
		s.Logger.Debugf("No source name available for table completion")
		return
	}

	src := sourceByName(s.DbtConfig, ctx.PreviousArgs[0])
	if src == nil {
		s.Logger.Debugf("No source named %q found for table completion", ctx.PreviousArgs[0])
		return
	}

	tables := tableNamesWithPrefix(src, ctx.Prefix)
	if len(tables) > 0 {
		s.Logger.Debugf("Found %d Tables", len(tables))
		s.Logger.Tracef("Tables: %+v", tables)
		for _, tblName := range tables {
			response.Result.Items = append(response.Result.Items, lsp.CompletionItem{
				Label:  tblName,
				Kind:   lsp.CompletionItemKindReference,
				Detail: "dbt Source Table",
				TextEdit: lsp.CompletionTextEdit{
					Range:   snapshot.Range(ctx.Start, ctx.End),
					NewText: tblName,
				},
			})
		}
	} else {
		s.Logger.Debugf("No tables found for prefix %s", ctx.Prefix)
		s.Logger.Trace("Context: %+v", ctx)
	}
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

func (s *State) TextDocumentCodeCompletion(
	id int,
	params lsp.CompletionParams,
) lsp.CompletionResponse {
	response := NewCompletionResponse(id)
	if !s.IsServerActive() {
		return *response
	}

	snap, ok := s.Snapshot(params.TextDocument.URI)
	if !ok {
		s.Logger.Errorf("Completion requested for unopened document: %s", params.TextDocument.URI)
		return *response
	}

	cursor := snap.Offset(params.Position)
	ctx := jinja.Classify(snap.Text, cursor)
	s.Logger.Tracef("Completion context: %+v", ctx)

	switch ctx.Role {
	case jinja.RoleStringArg:
		switch ctx.Callee {
		case "ref":
			if s.IsRefCompletionEnabled() {
				s.createRefResponse(snap, ctx, response)
			}
		case "source":
			if s.IsSourceCompletionEnabled() {
				s.createSourceResponse(snap, ctx, response)
			}
		}
	}

	return *response
}

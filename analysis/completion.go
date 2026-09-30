package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fsorodrigues/dbt-ls/dbt"
	"github.com/fsorodrigues/dbt-ls/jinja"
	"github.com/fsorodrigues/dbt-ls/lsp"
	"github.com/fsorodrigues/dbt-ls/utils"
)

// maxCompletionItems caps how many candidates we return in one response.
// When a result set is truncated, IsIncomplete is set so the client knows to
// re-query as the user keeps typing, instead of filtering a partial list
// itself.
const maxCompletionItems = 50

func (s *State) createRefResponse(
	snapshot Snapshot,
	ctx jinja.Context,
	response *lsp.CompletionResponse,
) {
	s.Logger.Tracef("Ref search prefix: %s", ctx.Prefix)
	models := s.DbtModels.KeysWithPrefix(strings.ToLower(ctx.Prefix))
	if len(models) > maxCompletionItems {
		models = models[:maxCompletionItems]
		response.Result.IsIncomplete = true
	}

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

func (s *State) createMacroResponse(
	snapshot Snapshot,
	ctx jinja.Context,
	response *lsp.CompletionResponse,
) {
	prefix := strings.ToLower(ctx.Prefix)
	s.Logger.Tracef("Macro search prefix: %s", ctx.Prefix)

	items, projectNames := s.projectMacroCompletionItems(snapshot, ctx, prefix)
	items = append(
		items,
		builtinMacroCompletionItems(snapshot, ctx, prefix, projectNames)...,
	)

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortText == items[j].SortText {
			return items[i].Label < items[j].Label
		}
		return items[i].SortText < items[j].SortText
	})

	if len(items) > maxCompletionItems {
		items = items[:maxCompletionItems]
		response.Result.IsIncomplete = true
	}

	response.Result.Items = append(response.Result.Items, items...)

	s.Logger.Debugf(
		"Macro completion ready. Contains %d items",
		len(response.Result.Items),
	)
}

// projectMacroCompletionItems builds completion items for macros declared in
// the project, inserting a call with argument names. It also returns the set
// of lower-cased macro names found, so builtinMacroCompletionItems can skip
// names the project has overridden.
func (s *State) projectMacroCompletionItems(
	snapshot Snapshot,
	ctx jinja.Context,
	prefix string,
) ([]lsp.CompletionItem, map[string]struct{}) {
	s.DbtMacrosMu.Lock()
	defer s.DbtMacrosMu.Unlock()

	keys := s.DbtMacros.KeysWithPrefix(prefix)
	items := make([]lsp.CompletionItem, 0, len(keys))
	names := make(map[string]struct{}, len(keys))

	for _, macKey := range keys {
		macVal, ok := s.DbtMacros.Get(macKey)
		if !ok {
			s.Logger.Errorf("Could not get macro %q from trie", macKey)
			continue
		}

		names[strings.ToLower(macKey)] = struct{}{}

		args := ""
		if len(macVal.Args) > 0 {
			argNames := utils.Map(
				macVal.Args,
				func(v dbt.Arg) string { return v.Name },
			)
			args = strings.Join(argNames, ", ")
		}

		newText := fmt.Sprintf("%s(%s)", macKey, args)
		items = append(items, lsp.CompletionItem{
			Label:            macKey,
			Kind:             lsp.CompletionItemKindFunction,
			Detail:           macVal.Signature(),
			Documentation:    macVal.File,
			SortText:         macroSortText(macKey),
			FilterText:       macKey,
			InsertText:       newText,
			InsertTextFormat: lsp.InsertTextFormatPlainText,
			TextEdit: lsp.CompletionTextEdit{
				Range:   snapshot.Range(ctx.Start, ctx.End),
				NewText: newText,
			},
		})
	}

	return items, names
}

// builtinMacroCompletionItems builds completion items for dbt's built-in
// Jinja context members matching prefix, skipping any name the project has
// already declared. Built-ins insert only their bare name: some are values
// rather than callables, so an argument placeholder would not be valid.
func builtinMacroCompletionItems(
	snapshot Snapshot,
	ctx jinja.Context,
	prefix string,
	projectNames map[string]struct{},
) []lsp.CompletionItem {
	builtins := dbt.BuiltinsWithPrefix(prefix)
	items := make([]lsp.CompletionItem, 0, len(builtins))

	for _, builtin := range builtins {
		if _, exists := projectNames[strings.ToLower(builtin.Name)]; exists {
			continue
		}

		items = append(items, lsp.CompletionItem{
			Label:            builtin.Name,
			Kind:             lsp.CompletionItemKindFunction,
			Detail:           builtin.Signature(),
			Documentation:    builtin.Doc,
			SortText:         "1" + builtin.Name,
			FilterText:       builtin.Name,
			InsertText:       builtin.Name,
			InsertTextFormat: lsp.InsertTextFormatPlainText,
			TextEdit: lsp.CompletionTextEdit{
				Range:   snapshot.Range(ctx.Start, ctx.End),
				NewText: builtin.Name,
			},
		})
	}

	return items
}

// macroSortText ranks a project's own macros above adapter-dispatch
// implementations and private-convention names, so `default__foo` /
// `snowflake__foo` / `_helper` sink to the bottom of the list without being
// hidden entirely. LSP clients sort lexicographically on this field.
func macroSortText(name string) string {
	switch {
	case strings.HasPrefix(name, "_"):
		return "3" + name
	case strings.Contains(name, "__"):
		return "3" + name
	default:
		return "0" + name
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
	if len(sources) > maxCompletionItems {
		sources = sources[:maxCompletionItems]
		response.Result.IsIncomplete = true
	}
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
	if len(tables) > maxCompletionItems {
		tables = tables[:maxCompletionItems]
		response.Result.IsIncomplete = true
	}
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
			IsIncomplete: false,
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
	case jinja.RoleValue:
		if s.IsMacrosEnabled() {
			s.createMacroResponse(snap, ctx, response)
		}
	}

	return *response
}

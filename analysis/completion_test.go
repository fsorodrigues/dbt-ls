package analysis

import (
	"fmt"
	"testing"

	"github.com/fsorodrigues/dbt-ls/dbt"
	"github.com/fsorodrigues/dbt-ls/jinja"
	"github.com/fsorodrigues/dbt-ls/lsp"
)

func testSnapshotAndCtx(prefix string) (Snapshot, jinja.Context) {
	src := []rune(prefix)
	return Snapshot{Text: src}, jinja.Context{
		Prefix: prefix,
		Start:  0,
		End:    len(src),
	}
}

func completionItem(
	t *testing.T,
	items []lsp.CompletionItem,
	label string,
) lsp.CompletionItem {
	t.Helper()

	for _, item := range items {
		if item.Label == label {
			return item
		}
	}

	t.Fatalf("completion item %q not found", label)
	return lsp.CompletionItem{}
}

func TestCreateMacroResponseNotTruncated(t *testing.T) {
	s := newTestState()
	for _, name := range []string{"my_macro_a", "my_macro_b"} {
		s.DbtMacros.Put(name, dbt.Macro{Name: name})
	}

	snap, ctx := testSnapshotAndCtx("my_")
	response := NewCompletionResponse(1)
	s.createMacroResponse(snap, ctx, response)

	if len(response.Result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(response.Result.Items))
	}
	if response.Result.IsIncomplete {
		t.Error("expected IsIncomplete to be false when nothing was truncated")
	}
}

func TestCreateMacroResponseTruncated(t *testing.T) {
	s := newTestState()
	for i := 0; i < maxCompletionItems+10; i++ {
		name := fmt.Sprintf("macro_%03d", i)
		s.DbtMacros.Put(name, dbt.Macro{Name: name})
	}

	snap, ctx := testSnapshotAndCtx("macro_")
	response := NewCompletionResponse(1)
	s.createMacroResponse(snap, ctx, response)

	if len(response.Result.Items) != maxCompletionItems {
		t.Fatalf("expected %d items, got %d", maxCompletionItems, len(response.Result.Items))
	}
	if !response.Result.IsIncomplete {
		t.Error("expected IsIncomplete to be true when results were truncated")
	}
}

func TestCreateRefResponseTruncated(t *testing.T) {
	s := newTestState()
	for i := 0; i < maxCompletionItems+5; i++ {
		name := fmt.Sprintf("model_%03d", i)
		s.DbtModels.Put(name, "/path/"+name+".sql")
	}

	snap, ctx := testSnapshotAndCtx("model_")
	response := NewCompletionResponse(1)
	s.createRefResponse(snap, ctx, response)

	if len(response.Result.Items) != maxCompletionItems {
		t.Fatalf("expected %d items, got %d", maxCompletionItems, len(response.Result.Items))
	}
	if !response.Result.IsIncomplete {
		t.Error("expected IsIncomplete to be true when results were truncated")
	}
}

func TestNewCompletionResponseDefaultsToComplete(t *testing.T) {
	response := NewCompletionResponse(1)
	if response.Result.IsIncomplete {
		t.Error("expected a fresh CompletionResponse to default IsIncomplete to false")
	}
	if response.Result.Items == nil {
		t.Error("expected Items to be initialized to an empty slice")
	}
}

func TestProjectMacroCompletionItemsInsertsCallWithArgs(t *testing.T) {
	s := newTestState()
	s.DbtMacros.Put("resize", dbt.Macro{
		Name: "resize",
		Args: []dbt.Arg{
			{Name: "column"},
			{Name: "width"},
		},
	})

	snap, ctx := testSnapshotAndCtx("re")
	items, names := s.projectMacroCompletionItems(snap, ctx, "re")

	if _, ok := names["resize"]; !ok {
		t.Fatalf("expected resize in project names, got %v", names)
	}

	resize := completionItem(t, items, "resize")
	if resize.TextEdit.NewText != "resize(column, width)" {
		t.Errorf(
			"project macro insertion = %q, want %q",
			resize.TextEdit.NewText,
			"resize(column, width)",
		)
	}
	if resize.Kind != lsp.CompletionItemKindFunction {
		t.Errorf("expected function kind for project macro, got %d", resize.Kind)
	}
}

func TestBuiltinMacroCompletionItemsInsertsBareName(t *testing.T) {
	snap, ctx := testSnapshotAndCtx("re")
	items := builtinMacroCompletionItems(snap, ctx, "re", map[string]struct{}{})

	if !containsLabel(items, "ref") {
		t.Fatalf("expected ref, got %v", labelsOf(items))
	}
	if !containsLabel(items, "return") {
		t.Fatalf("expected return, got %v", labelsOf(items))
	}

	ref := completionItem(t, items, "ref")
	if ref.TextEdit.NewText != "ref" {
		t.Errorf("builtin insertion = %q, want %q", ref.TextEdit.NewText, "ref")
	}
	if ref.SortText != "1ref" {
		t.Errorf("builtin sort text = %q, want %q", ref.SortText, "1ref")
	}
}

func TestBuiltinMacroCompletionItemsSkipsProjectOverrides(t *testing.T) {
	snap, ctx := testSnapshotAndCtx("ref")
	items := builtinMacroCompletionItems(
		snap, ctx, "ref", map[string]struct{}{"ref": {}},
	)

	if containsLabel(items, "ref") {
		t.Errorf("expected ref to be skipped, got %v", labelsOf(items))
	}
}

func TestCreateMacroResponseIncludesBuiltins(t *testing.T) {
	s := newTestState()
	snap, ctx := testSnapshotAndCtx("re")
	response := NewCompletionResponse(1)

	s.createMacroResponse(snap, ctx, response)

	if !containsLabel(response.Result.Items, "ref") {
		t.Errorf("expected ref, got %v", labelsOf(response.Result.Items))
	}
	if !containsLabel(response.Result.Items, "return") {
		t.Errorf("expected return, got %v", labelsOf(response.Result.Items))
	}
}

func TestCreateMacroResponseProjectMacroOverridesBuiltin(t *testing.T) {
	s := newTestState()
	s.DbtMacros.Put("ref", dbt.Macro{
		Name: "ref",
		File: "/project/macros/ref.sql",
	})

	snap, ctx := testSnapshotAndCtx("ref")
	response := NewCompletionResponse(1)
	s.createMacroResponse(snap, ctx, response)

	count := 0
	for _, item := range response.Result.Items {
		if item.Label == "ref" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one ref completion, got %d", count)
	}

	ref := completionItem(t, response.Result.Items, "ref")
	if ref.Documentation != "/project/macros/ref.sql" {
		t.Errorf(
			"ref documentation = %q, want project file",
			ref.Documentation,
		)
	}
	if ref.TextEdit.NewText != "ref()" {
		t.Errorf(
			"ref insertion = %q, want project macro call form",
			ref.TextEdit.NewText,
		)
	}
	if ref.SortText != "0ref" {
		t.Errorf("ref sort text = %q, want %q", ref.SortText, "0ref")
	}
}

func TestCreateMacroResponseInsertionDiffersByOrigin(t *testing.T) {
	s := newTestState()
	s.DbtMacros.Put("resize", dbt.Macro{
		Name: "resize",
		Args: []dbt.Arg{
			{Name: "column"},
			{Name: "width"},
		},
	})

	snap, ctx := testSnapshotAndCtx("re")
	response := NewCompletionResponse(1)
	s.createMacroResponse(snap, ctx, response)

	resize := completionItem(t, response.Result.Items, "resize")
	if resize.TextEdit.NewText != "resize(column, width)" {
		t.Errorf(
			"project macro insertion = %q, want %q",
			resize.TextEdit.NewText,
			"resize(column, width)",
		)
	}

	ref := completionItem(t, response.Result.Items, "ref")
	if ref.TextEdit.NewText != "ref" {
		t.Errorf(
			"builtin insertion = %q, want %q",
			ref.TextEdit.NewText,
			"ref",
		)
	}
}

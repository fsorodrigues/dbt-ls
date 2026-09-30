package analysis

import (
	"strings"
	"testing"

	"github.com/fsorodrigues/dbt-ls/dbt"
	"github.com/fsorodrigues/dbt-ls/lsp"
)

// markedPosition strips the "|" caret marker from marked and returns the
// resulting text along with the LSP position it marked.
func markedPosition(t *testing.T, marked string) (string, lsp.TextDocumentPosition) {
	t.Helper()
	i := strings.Index(marked, "|")
	if i < 0 {
		t.Fatalf("fixture has no caret marker: %q", marked)
	}
	text := marked[:i] + marked[i+1:]
	offset := len([]rune(marked[:i]))
	snap := Snapshot{Text: []rune(text)}
	return text, snap.Position(offset)
}

func labelsOf(items []lsp.CompletionItem) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}

func containsLabel(items []lsp.CompletionItem, label string) bool {
	for _, item := range items {
		if item.Label == label {
			return true
		}
	}
	return false
}

func requestCompletion(t *testing.T, s *State, marked string) lsp.CompletionResponse {
	t.Helper()
	text, pos := markedPosition(t, marked)

	const uri = "file:///model.sql"
	s.OpenDocument(uri, text, 1)

	return s.TextDocumentCodeCompletion(1, lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
}

func newCompletionTestState() *State {
	s := newTestState()
	s.SetServerActive(true)
	s.setRefsEnabled(true)
	s.setSourcesEnabled(true)
	s.setMacrosEnabled(true)
	return s
}

func TestTextDocumentCodeCompletionMacros(t *testing.T) {
	s := newCompletionTestState()
	s.DbtMacros.Put("cents_to_dollars", dbt.Macro{Name: "cents_to_dollars"})
	s.DbtMacros.Put("grant_select", dbt.Macro{Name: "grant_select"})

	resp := requestCompletion(t, s, "{{ ce|")

	if !containsLabel(resp.Result.Items, "cents_to_dollars") {
		t.Errorf("expected cents_to_dollars in completions, got %v", labelsOf(resp.Result.Items))
	}
	if containsLabel(resp.Result.Items, "grant_select") {
		t.Errorf("did not expect grant_select in completions, got %v", labelsOf(resp.Result.Items))
	}
}

func TestTextDocumentCodeCompletionBuiltins(t *testing.T) {
	s := newCompletionTestState()

	resp := requestCompletion(t, s, "{{ re|")

	if !containsLabel(resp.Result.Items, "ref") {
		t.Errorf("expected ref in completions, got %v", labelsOf(resp.Result.Items))
	}
	if !containsLabel(resp.Result.Items, "return") {
		t.Errorf("expected return in completions, got %v", labelsOf(resp.Result.Items))
	}
}

func TestTextDocumentCodeCompletionRefs(t *testing.T) {
	s := newCompletionTestState()
	s.DbtModels.Put("orders", "/models/orders.sql")
	s.DbtModels.Put("customers", "/models/customers.sql")

	resp := requestCompletion(t, s, "{{ ref('ord|') }}")

	if !containsLabel(resp.Result.Items, "orders") {
		t.Errorf("expected orders in completions, got %v", labelsOf(resp.Result.Items))
	}
	if containsLabel(resp.Result.Items, "customers") {
		t.Errorf("did not expect customers in completions, got %v", labelsOf(resp.Result.Items))
	}
}

func TestTextDocumentCodeCompletionMisclassificationFixed(t *testing.T) {
	s := newCompletionTestState()
	s.DbtModels.Put("orders", "/models/orders.sql")

	// Regression: whole-line regexes used to classify this as "ref" no
	// matter where the cursor was, because `ref(` appears later on the
	// line. The classifier fixes this by looking at the cursor itself.
	resp := requestCompletion(t, s, "{{ dbt_utils.star(from=ref('ord|')) }}")

	if !containsLabel(resp.Result.Items, "orders") {
		t.Errorf("expected orders in completions, got %v", labelsOf(resp.Result.Items))
	}
}

func TestTextDocumentCodeCompletionNoneInsideBindingName(t *testing.T) {
	s := newCompletionTestState()
	s.DbtMacros.Put("clean_column", dbt.Macro{Name: "clean_column"})

	// {% set NAME %} is naming a variable, not calling a macro.
	resp := requestCompletion(t, s, "{% set clean_|")

	if len(resp.Result.Items) != 0 {
		t.Errorf("expected no completions in a binding-name position, got %v", labelsOf(resp.Result.Items))
	}
}

func TestTextDocumentCodeCompletionNoneInsideComment(t *testing.T) {
	s := newCompletionTestState()
	s.DbtMacros.Put("clean_column", dbt.Macro{Name: "clean_column"})

	resp := requestCompletion(t, s, "{# clean_|")

	if len(resp.Result.Items) != 0 {
		t.Errorf("expected no completions inside a comment, got %v", labelsOf(resp.Result.Items))
	}
}

func TestTextDocumentCodeCompletionNoneWhenServerInactive(t *testing.T) {
	s := newCompletionTestState()
	s.SetServerActive(false)
	s.DbtMacros.Put("clean_column", dbt.Macro{Name: "clean_column"})

	resp := requestCompletion(t, s, "{{ clean_|")

	if len(resp.Result.Items) != 0 {
		t.Errorf("expected no completions when server is inactive, got %v", labelsOf(resp.Result.Items))
	}
}

func TestTextDocumentCodeCompletionUnopenedDocument(t *testing.T) {
	s := newCompletionTestState()

	resp := s.TextDocumentCodeCompletion(1, lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: "file:///missing.sql"},
			Position:     lsp.TextDocumentPosition{Line: 0, Character: 0},
		},
	})

	if len(resp.Result.Items) != 0 {
		t.Errorf("expected no completions for an unopened document, got %v", labelsOf(resp.Result.Items))
	}
}

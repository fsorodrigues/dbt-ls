package analysis

import (
	"fmt"
	"testing"

	"github.com/fsorodrigues/dbt-ls/dbt"
	"github.com/fsorodrigues/dbt-ls/jinja"
)

func testSnapshotAndCtx(prefix string) (Snapshot, jinja.Context) {
	src := []rune(prefix)
	return Snapshot{Text: src}, jinja.Context{
		Prefix: prefix,
		Start:  0,
		End:    len(src),
	}
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

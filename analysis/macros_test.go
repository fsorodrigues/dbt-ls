package analysis

import (
	"path/filepath"
	"testing"

	"github.com/fsorodrigues/dbt-ls/dbt"
)

func macroNames(s *State, path string) []string {
	name := filepath.Clean(path)
	return s.DbtMacroFiles[name]
}

func TestAddNewMacroFileIndexesEveryMacro(t *testing.T) {
	s := newTestState()
	file := filepath.Join(t.TempDir(), "utils.sql")
	writeTestFile(t, file, `
{% macro grant_select(schema, role) %}
  grant select on all tables in schema {{ schema }} to {{ role }}
{% endmacro %}

{% macro cents_to_dollars(column_name, precision=2) %}
  round({{ column_name }} / 100, {{ precision }})
{% endmacro %}
`)

	s.AddNewMacroFile(file)

	for _, name := range []string{"grant_select", "cents_to_dollars"} {
		if _, ok := s.DbtMacros.Get(name); !ok {
			t.Errorf("expected macro %q to be indexed", name)
		}
	}

	names := macroNames(s, file)
	if len(names) != 2 {
		t.Fatalf("expected 2 macro names tracked for file, got %v", names)
	}
}

func TestAddNewMacroFileReindexIsIdempotent(t *testing.T) {
	s := newTestState()
	file := filepath.Join(t.TempDir(), "utils.sql")
	writeTestFile(t, file, `
{% macro foo(a) %}{{ a }}{% endmacro %}
{% macro bar(b) %}{{ b }}{% endmacro %}
`)
	s.AddNewMacroFile(file)

	// Re-parse after the file changed to drop "bar" and add "baz".
	writeTestFile(t, file, `
{% macro foo(a) %}{{ a }}{% endmacro %}
{% macro baz(c) %}{{ c }}{% endmacro %}
`)
	s.AddNewMacroFile(file)

	if _, ok := s.DbtMacros.Get("foo"); !ok {
		t.Error("expected foo to still be indexed")
	}
	if _, ok := s.DbtMacros.Get("baz"); !ok {
		t.Error("expected baz to be indexed after re-parse")
	}
	if _, ok := s.DbtMacros.Get("bar"); ok {
		t.Error("expected bar to be removed after re-parse dropped it")
	}

	names := macroNames(s, file)
	if len(names) != 2 {
		t.Fatalf("expected 2 macro names tracked for file after re-parse, got %v", names)
	}
}

func TestRemoveMacroFileFromIndexDropsOnlyItsMacros(t *testing.T) {
	s := newTestState()
	fileA := filepath.Join(t.TempDir(), "a.sql")
	fileB := filepath.Join(t.TempDir(), "b.sql")
	writeTestFile(t, fileA, `{% macro from_a(x) %}{{ x }}{% endmacro %}`)
	writeTestFile(t, fileB, `{% macro from_b(x) %}{{ x }}{% endmacro %}`)

	s.AddNewMacroFile(fileA)
	s.AddNewMacroFile(fileB)

	s.RemoveMacroFileFromIndex(fileA)

	if _, ok := s.DbtMacros.Get("from_a"); ok {
		t.Error("expected from_a to be removed")
	}
	if _, ok := s.DbtMacros.Get("from_b"); !ok {
		t.Error("expected from_b to remain indexed")
	}
}

func TestAddNewMacroFileNameCollisionAcrossFiles(t *testing.T) {
	s := newTestState()
	fileA := filepath.Join(t.TempDir(), "a.sql")
	fileB := filepath.Join(t.TempDir(), "b.sql")
	writeTestFile(t, fileA, `{% macro shared(x) %}{{ x }} from a{% endmacro %}`)
	writeTestFile(t, fileB, `{% macro shared(x) %}{{ x }} from b{% endmacro %}`)

	s.AddNewMacroFile(fileA)
	s.AddNewMacroFile(fileB)

	// The trie stores one value per name; the most recently indexed
	// declaration wins.
	got, ok := s.DbtMacros.Get("shared")
	if !ok {
		t.Fatal("expected shared to be indexed")
	}
	if filepath.Clean(got.File) != filepath.Clean(fileB) {
		t.Fatalf("expected shared to resolve to %s, got %s", fileB, got.File)
	}

	// Removing the file that currently owns the shadowed name drops it
	// entirely, even though fileA also declared a "shared" macro. This
	// documents the current index's collision-handling behaviour.
	s.RemoveMacroFileFromIndex(fileB)
	if _, ok := s.DbtMacros.Get("shared"); ok {
		t.Error("expected shared to be removed once its owning file is removed")
	}
}

func TestAddNewMacroToIndexAndRemove(t *testing.T) {
	s := newTestState()
	s.AddNewMacroToIndex("my_macro", dbt.Macro{Name: "my_macro"})

	if _, ok := s.DbtMacros.Get("my_macro"); !ok {
		t.Fatal("expected my_macro to be indexed")
	}

	s.RemoveMacroFromIndex("my_macro")

	if _, ok := s.DbtMacros.Get("my_macro"); ok {
		t.Error("expected my_macro to be removed")
	}
}

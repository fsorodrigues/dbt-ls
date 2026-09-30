package analysis

import (
	"path/filepath"
	"testing"
)

func TestHandleCreateEventIndexesNewMacroFile(t *testing.T) {
	root := t.TempDir()
	s := newTestState()
	s.SetProjectRoot(root)
	s.MacroRoots = []string{"macros"}
	s.ModelRoots = []string{"models"}
	s.setServerActive(true)

	path := filepath.Join(root, "macros", "utils.sql")
	writeTestFile(t, path, `
{% macro cents_to_dollars(column_name) %}
  round({{ column_name }} / 100, 2)
{% endmacro %}
`)

	s.handleCreateEvent(path)

	if _, ok := s.DbtMacros.Get("cents_to_dollars"); !ok {
		t.Error("expected macro to be indexed after handleCreateEvent")
	}
}

func TestAddNewMacroFileDedupesUnchangedContent(t *testing.T) {
	s := newTestState()
	file := filepath.Join(t.TempDir(), "utils.sql")
	writeTestFile(t, file, `
{% macro grant_select(schema, role) %}
  grant select on all tables in schema {{ schema }} to {{ role }}
{% endmacro %}
`)

	s.AddNewMacroFile(file)
	s.RemoveMacroFromIndex("grant_select") // simulate external removal without touching the hash

	// Re-adding the same unchanged content should be a no-op due to the hash cache.
	s.AddNewMacroFile(file)
	if _, ok := s.DbtMacros.Get("grant_select"); ok {
		t.Error("expected AddNewMacroFile to skip reparsing unchanged content")
	}

	// Changing the content must bypass the dedupe and reindex.
	writeTestFile(t, file, `
{% macro grant_select(schema, role) %}
  grant select on all tables in schema {{ schema }} to {{ role }}
{% endmacro %}
{% macro extra_macro() %}1{% endmacro %}
`)
	s.AddNewMacroFile(file)
	if _, ok := s.DbtMacros.Get("extra_macro"); !ok {
		t.Error("expected AddNewMacroFile to reindex after content changed")
	}
}

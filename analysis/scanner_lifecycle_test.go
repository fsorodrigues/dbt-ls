package analysis

import (
	"path/filepath"
	"testing"
)

func TestScanRootPathIndexesMacros(t *testing.T) {
	root := t.TempDir()
	s := newTestState()
	s.SetProjectRoot(root)
	s.ModelRoots = []string{"models"}
	s.MacroRoots = []string{"macros"}

	writeTestFile(t, filepath.Join(root, "models", "orders.sql"), "select 1")
	writeTestFile(t, filepath.Join(root, "macros", "utils.sql"), `
{% macro cents_to_dollars(column_name) %}
  round({{ column_name }} / 100, 2)
{% endmacro %}
`)

	if err := s.ScanRootPath(root); err != nil {
		t.Fatalf("ScanRootPath: %v", err)
	}

	if _, ok := s.DbtMacros.Get("cents_to_dollars"); !ok {
		t.Error("expected macro cents_to_dollars to be indexed after ScanRootPath")
	}
	if _, ok := s.DbtModels.Get("orders"); !ok {
		t.Error("expected model orders to be indexed after ScanRootPath")
	}
}

func TestResetProjectStateClearsMacros(t *testing.T) {
	s := newTestState()
	file := filepath.Join(t.TempDir(), "utils.sql")
	writeTestFile(t, file, `
{% macro grant_select(schema, role) %}
  grant select on all tables in schema {{ schema }} to {{ role }}
{% endmacro %}
`)
	s.AddNewMacroFile(file)

	if _, ok := s.DbtMacros.Get("grant_select"); !ok {
		t.Fatal("setup: expected macro to be indexed before reset")
	}

	s.resetProjectState()

	if _, ok := s.DbtMacros.Get("grant_select"); ok {
		t.Error("expected macro index to be cleared by resetProjectState")
	}
	if names := s.DbtMacroFiles[filepath.Clean(file)]; names != nil {
		t.Errorf("expected DbtMacroFiles to be cleared by resetProjectState, got %v", names)
	}

	// Re-adding the same file after reset must reindex it even though the
	// content hash was seen before the reset.
	s.AddNewMacroFile(file)
	if _, ok := s.DbtMacros.Get("grant_select"); !ok {
		t.Error("expected macro to be reindexed after resetProjectState")
	}
}

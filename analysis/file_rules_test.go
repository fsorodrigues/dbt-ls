package analysis

import (
	"path/filepath"
	"testing"
)

func TestIsModelAndMacroFileClassifyByFullPath(t *testing.T) {
	root := t.TempDir()
	s := newTestState()
	s.SetProjectRoot(root)
	s.ModelRoots = []string{"models"}
	s.MacroRoots = []string{"macros"}

	modelPath := filepath.Join(root, "models", "x.sql")
	macroPath := filepath.Join(root, "macros", "y.sql")
	ambiguousModel := filepath.Join(root, "models", "staging", "macros_helper.sql")

	if !s.isModelFile(modelPath) {
		t.Errorf("expected %q to be classified as a model file", modelPath)
	}
	if s.isMacroFile(modelPath) {
		t.Errorf("expected %q to not be classified as a macro file", modelPath)
	}

	if !s.isMacroFile(macroPath) {
		t.Errorf("expected %q to be classified as a macro file", macroPath)
	}
	if s.isModelFile(macroPath) {
		t.Errorf("expected %q to not be classified as a model file", macroPath)
	}

	if !s.isModelFile(ambiguousModel) {
		t.Errorf("expected %q to be classified as a model file", ambiguousModel)
	}
	if s.isMacroFile(ambiguousModel) {
		t.Errorf("expected %q to not be classified as a macro file, got misclassified as macro", ambiguousModel)
	}
}

func TestIsModelAndMacroFileRejectBareBasenames(t *testing.T) {
	// Regression: passing a bare basename (no root prefix) must never match,
	// because a basename can never contain the configured root directory.
	root := t.TempDir()
	s := newTestState()
	s.SetProjectRoot(root)
	s.ModelRoots = []string{"models"}
	s.MacroRoots = []string{"macros"}

	if s.isModelFile("x.sql") {
		t.Error("bare basename should not be classified as a model file")
	}
	if s.isMacroFile("y.sql") {
		t.Error("bare basename should not be classified as a macro file")
	}
}

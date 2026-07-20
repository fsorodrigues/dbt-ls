package analysis

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"dbt_ls/logger"

	charm "github.com/charmbracelet/log"
)

func newTestState() *State {
	return NewState(
		&logger.Logger{Logger: charm.New(io.Discard)},
		io.Discard,
		nil,
	)
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseDbtConfigDefaultsModelPaths(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "dbt_project.yml"), "name: test_project\n")

	project, err := newTestState().ParseDbtConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(project.ModelPaths, []string{"models"}) {
		t.Fatalf("expected default model paths, got %v", project.ModelPaths)
	}
}

func TestParseDbtConfigPreservesMultipleModelPaths(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "dbt_project.yml"), "model-paths: [models, custom_models]\n")

	project, err := newTestState().ParseDbtConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(project.ModelPaths, []string{"models", "custom_models"}) {
		t.Fatalf("expected configured model paths, got %v", project.ModelPaths)
	}
}

func TestParseDbtConfigPreservesEmptyModelPaths(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "dbt_project.yml"), "model-paths: []\n")

	project, err := newTestState().ParseDbtConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if project.ModelPaths == nil || len(project.ModelPaths) != 0 {
		t.Fatalf("expected explicit empty model paths, got %#v", project.ModelPaths)
	}
}

func TestParseDbtConfigReturnsNotFoundError(t *testing.T) {
	_, err := newTestState().ParseDbtConfig(t.TempDir())
	if !errors.Is(err, ErrDbtProjectNotFound) {
		t.Fatalf("expected ErrDbtProjectNotFound, got %v", err)
	}
}

func TestScanRootPathIndexesMultipleModelPaths(t *testing.T) {
	root := t.TempDir()
	firstModel := filepath.Join(root, "models", "first.sql")
	secondModel := filepath.Join(root, "custom_models", "second.sql")
	writeTestFile(t, filepath.Join(root, "dbt_project.yml"), "name: test_project\n")
	writeTestFile(t, firstModel, "select 1\n")
	writeTestFile(t, secondModel, "select 2\n")

	state := newTestState()
	state.ModelRoots = []string{"models", "custom_models"}
	if err := state.ScanRootPath(root); err != nil {
		t.Fatal(err)
	}

	if got, ok := state.DbtModels.Get("first"); !ok || got != firstModel {
		t.Fatalf("expected first model %q, got %q", firstModel, got)
	}
	if got, ok := state.DbtModels.Get("second"); !ok || got != secondModel {
		t.Fatalf("expected second model %q, got %q", secondModel, got)
	}
}

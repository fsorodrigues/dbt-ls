package analysis

import (
	"path/filepath"
	"testing"
)

func TestNewStateDisablesCapabilities(t *testing.T) {
	s := newTestState()

	if s.IsServerActive() {
		t.Fatal("new state server is active")
	}
	if s.IsRefCompletionEnabled() {
		t.Fatal("new state ref completion enabled")
	}
	if s.IsSourceCompletionEnabled() {
		t.Fatal("new state source completion enabled")
	}
	if s.IsDefinitionEnabled() {
		t.Fatal("new state definition enabled")
	}
}

func TestActivateProjectEnablesCapabilities(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "dbt_project.yml")
	writeTestFile(t, projectPath, "name: example\nmodel-paths: [models]\n")
	writeTestFile(t, filepath.Join(root, "models", "orders.sql"), "select 1\n")
	if err := os.Mkdir(filepath.Join(root, "macros"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := newTestState()
	project, err := s.ParseDbtConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ActivateProject(root, projectPath, project); err != nil {
		t.Fatal(err)
	}

	if !s.IsServerActive() {
		t.Fatal("server inactive after project activation")
	}
	if !s.IsRefCompletionEnabled() {
		t.Fatal("ref completion disabled after project activation")
	}
	if !s.IsSourceCompletionEnabled() {
		t.Fatal("source completion disabled after project activation")
	}
	if !s.IsDefinitionEnabled() {
		t.Fatal("definition disabled after project activation")
	}
}

func TestSourceConflictDisablesOnlySourceCompletion(t *testing.T) {
	s := newTestState()
	s.setServerActive(true)
	s.enableProjectCapabilities(true)

	dir := t.TempDir()
	s.SetDbtSources(testSource("warehouse", "orders"), filepath.Join(dir, "a.yml"))
	s.SetDbtSources(testSource("warehouse", "orders"), filepath.Join(dir, "b.yml"))

	if s.IsSourceCompletionEnabled() {
		t.Fatal("source completion enabled with source conflict")
	}
	if !s.IsRefCompletionEnabled() {
		t.Fatal("source conflict disabled ref completion")
	}
	if !s.IsDefinitionEnabled() {
		t.Fatal("source conflict disabled definition")
	}
}

func TestDisableProjectCapabilitiesRequiresServerActive(t *testing.T) {
	s := newTestState()
	s.enableProjectCapabilities(true)

	if s.IsRefCompletionEnabled() || s.IsSourceCompletionEnabled() || s.IsDefinitionEnabled() {
		t.Fatal("capability getter ignored inactive server")
	}
}

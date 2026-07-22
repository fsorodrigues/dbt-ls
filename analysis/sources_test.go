package analysis

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"dbt_ls/logger"

	charm "github.com/charmbracelet/log"
)

func testSource(name, table string) DbtSources {
	return DbtSources{Sources: []*DbtSource{{
		Name:     name,
		Database: "db",
		Schema:   "schema",
		Tables:   []*DbtTable{{Name: table}},
	}}}
}

func TestRemoveConfigYamlRemovesSourcesAndHash(t *testing.T) {
	s := newTestState()
	file := filepath.Join(t.TempDir(), "sources.yml")
	s.SetDbtSources(testSource("warehouse", "orders"), file)
	s.configFileHashes[file] = "hash"

	s.RemoveConfigYaml(file)

	if _, ok := s.DbtSourcesByFile[file]; ok {
		t.Fatal("sources for removed file remain")
	}
	if _, ok := s.SourceTableIndex[sourceTableKey{Source: "warehouse", Table: "orders"}]; ok {
		t.Fatal("source table index entry remains")
	}
	if _, ok := s.configFileHashes[file]; ok {
		t.Fatal("hash for removed file remains")
	}
	if len(s.DbtConfig.Sources) != 0 || !s.SourcesValid {
		t.Fatalf("computed source state not cleared: %#v, valid=%t", s.DbtConfig.Sources, s.SourcesValid)
	}
}

func TestRemoveConfigYamlResolvesCollision(t *testing.T) {
	s := newTestState()
	a := filepath.Join(t.TempDir(), "a.yml")
	b := filepath.Join(t.TempDir(), "b.yml")
	s.SetDbtSources(testSource("warehouse", "orders"), a)
	s.SetDbtSources(testSource("warehouse", "orders"), b)
	if s.SourcesValid {
		t.Fatal("expected duplicate source table to be invalid")
	}

	s.RemoveConfigYaml(b)

	if !s.SourcesValid || len(s.SourceFileErrors) != 0 {
		t.Fatalf("collision was not resolved: valid=%t errors=%#v", s.SourcesValid, s.SourceFileErrors)
	}
	if got := s.DbtConfig.Sources["warehouse"].Tables["orders"].SourceFile; got != a {
		t.Fatalf("remaining table source file = %q, want %q", got, a)
	}
}

func TestRemoveConfigYamlUnknownFileIsNoOp(t *testing.T) {
	s := newTestState()
	s.RemoveConfigYaml(filepath.Join(t.TempDir(), "missing.yml"))
	if !s.SourcesValid || len(s.DbtConfig.Sources) != 0 {
		t.Fatal("unknown file changed source state")
	}
}

func TestReconcileProjectDeactivatesAndReactivates(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "dbt_project.yml")
	s := NewState(&logger.Logger{Logger: charm.New(io.Discard)}, io.Discard, nil)
	s.SetProjectRoot(root)
	writeTestFile(t, projectPath, "name: example\nmodel-paths: [models]\n")
	writeTestFile(t, filepath.Join(root, "models", "orders.sql"), "select 1\n")

	s.reconcileProject(root)
	if !s.IsServerActive() {
		t.Fatal("valid project did not activate")
	}
	s.reconcileProject(root)
	select {
	case notif := <-s.NotifCh:
		t.Fatalf("active project reconciliation sent notification: %q", notif.Params.Message)
	default:
	}

	if err := os.Remove(projectPath); err != nil {
		t.Fatal(err)
	}
	s.reconcileProject(root)
	if s.IsServerActive() {
		t.Fatal("missing project did not deactivate")
	}
	if _, ok := s.DbtModels.Get("orders"); ok {
		t.Fatal("models remain after deactivation")
	}

	writeTestFile(t, projectPath, "name: example\nmodel-paths: [models]\n")
	s.reconcileProject(root)
	if !s.IsServerActive() {
		t.Fatal("recreated project did not reactivate")
	}
	if _, ok := s.DbtModels.Get("orders"); !ok {
		t.Fatal("models were not rescanned after reactivation")
	}
}

func TestRepeatedProjectIndexingFailureNotifiesOnce(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "dbt_project.yml"), "name: example\nmodel-paths: [missing]\n")
	s := newTestState()
	s.SetProjectRoot(root)
	s.SetServerActive(true)

	s.reconcileProject(root)
	s.reconcileProject(root)

	select {
	case notif := <-s.NotifCh:
		if notif.Params.Message != "dbt-ls: project could not be indexed: lstat "+filepath.Join(root, "missing")+": no such file or directory" {
			t.Fatalf("unexpected notification: %q", notif.Params.Message)
		}
	default:
		t.Fatal("expected indexing failure notification")
	}
	select {
	case notif := <-s.NotifCh:
		t.Fatalf("repeated indexing failure sent notification: %q", notif.Params.Message)
	default:
	}
}

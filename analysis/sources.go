package analysis

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fsorodrigues/dbt-ls/lsp"
	trie "github.com/zyedidia/generic/trie"
	"go.yaml.in/yaml/v4"
)

func (s *State) SetDbtProject(project DbtProject, file string) {
	s.DbtConfigMu.Lock()
	s.DbtConfig.Name = project.Name
	s.updateDbtSourcesForFileLocked(project.DbtSources, file)
	errs := s.allSourceErrorsLocked()
	s.DbtConfigMu.Unlock()
	s.setSourcesEnabled(len(errs) == 0)

	s.notifySourceState(errs)
}

func (s *State) resetProjectState() {
	s.disableProjectCapabilities()
	s.DbtConfigMu.Lock()
	s.DbtConfig = DbtConfig{}
	s.SourceFileErrors = map[string][]sourceFileError{}
	s.DbtSourcesByFile = map[string]DbtSources{}
	s.SourceTableIndex = map[sourceTableKey]map[string][]sourceDecl{}
	s.DbtConfigMu.Unlock()
	s.ModelRoots = []string{"models"}
	s.MacroRoots = []string{"macros"}

	s.configFileHashesMu.Lock()
	s.configFileHashes = map[string]string{}
	s.configFileHashesMu.Unlock()

	s.DbtModelsMu.Lock()
	s.DbtModels = trie.New[string]()
	s.DbtModelsMu.Unlock()
}

func (s *State) NotifyProject(message string) {
	s.ShowMessage(lsp.MessageTypeError, message)
}

func (s *State) SetDbtSources(sources DbtSources, file string) {
	s.DbtConfigMu.Lock()
	s.updateDbtSourcesForFileLocked(sources, file)
	errs := s.allSourceErrorsLocked()
	s.DbtConfigMu.Unlock()
	s.setSourcesEnabled(len(errs) == 0)

	s.notifySourceState(errs)
}

func (s *State) updateDbtSourcesForFileLocked(sources DbtSources, file string) {
	oldSources := s.DbtSourcesByFile[file]
	affectedKeys := affectedSourceTableKeys(oldSources, sources)

	s.removeSourceDeclsForFileLocked(file)
	s.DbtSourcesByFile[file] = sources
	s.addSourceDeclsForFileLocked(file, sources)
	s.recomputeAffectedSourceTablesLocked(affectedKeys)
}

func affectedSourceTableKeys(oldSources, newSources DbtSources) map[sourceTableKey]struct{} {
	keys := map[sourceTableKey]struct{}{}
	addKeys := func(sources DbtSources) {
		for _, src := range sources.Sources {
			for _, tab := range src.Tables {
				keys[sourceTableKey{Source: src.Name, Table: tab.Name}] = struct{}{}
			}
		}
	}

	addKeys(oldSources)
	addKeys(newSources)
	return keys
}

func (s *State) addSourceDeclsForFileLocked(file string, sources DbtSources) {
	for _, src := range sources.Sources {
		addSourceFileToTables(src.Tables, file)
		for _, tab := range src.Tables {
			key := sourceTableKey{Source: src.Name, Table: tab.Name}
			if s.SourceTableIndex[key] == nil {
				s.SourceTableIndex[key] = map[string][]sourceDecl{}
			}

			s.SourceTableIndex[key][file] = append(s.SourceTableIndex[key][file], sourceDecl{
				Source: src,
				Table:  tab,
				File:   file,
			})
		}
	}
}

func (s *State) removeSourceDeclsForFileLocked(file string) {
	for key, decls := range s.SourceTableIndex {
		delete(decls, file)
		if len(decls) == 0 {
			delete(s.SourceTableIndex, key)
		}
	}
}

func (s *State) recomputeAffectedSourceTablesLocked(keys map[sourceTableKey]struct{}) {
	s.removeSourceErrorsForKeysLocked(keys)
	for key := range keys {
		s.removeDbtConfigTableLocked(key)

		declsByFile := s.SourceTableIndex[key]
		declarationCount := 0
		var onlyDecl sourceDecl
		for _, decls := range declsByFile {
			declarationCount += len(decls)
			if len(decls) == 1 && declarationCount == 1 {
				onlyDecl = decls[0]
			}
		}

		switch declarationCount {
		case 0:
			continue
		case 1:
			s.upsertDbtConfigTableLocked(onlyDecl)
		default:
			s.registerSourceConflictLocked(key, declsByFile)
		}
	}
}

func (s *State) removeSourceErrorsForKeysLocked(keys map[sourceTableKey]struct{}) {
	for file, errs := range s.SourceFileErrors {
		kept := errs[:0]
		for _, err := range errs {
			if _, ok := keys[err.Key]; !ok {
				kept = append(kept, err)
			}
		}
		if len(kept) == 0 {
			delete(s.SourceFileErrors, file)
		} else {
			s.SourceFileErrors[file] = kept
		}
	}
}

func (s *State) removeDbtConfigTableLocked(key sourceTableKey) {
	if s.DbtConfig.Sources == nil {
		return
	}

	src := s.DbtConfig.Sources[key.Source]
	if src == nil {
		return
	}

	delete(src.Tables, key.Table)
	if len(src.Tables) == 0 {
		delete(s.DbtConfig.Sources, key.Source)
	}
}

func (s *State) upsertDbtConfigTableLocked(decl sourceDecl) {
	if s.DbtConfig.Sources == nil {
		s.DbtConfig.Sources = map[string]*DbtConfigSource{}
	}

	src := s.DbtConfig.Sources[decl.Source.Name]
	if src == nil {
		src = &DbtConfigSource{
			Name:     decl.Source.Name,
			Database: decl.Source.Database,
			Schema:   decl.Source.Schema,
			Tables:   map[string]*DbtTable{},
		}
		s.DbtConfig.Sources[decl.Source.Name] = src
	}
	src.Database = decl.Source.Database
	src.Schema = decl.Source.Schema

	src.Tables[decl.Table.Name] = decl.Table
}

func (s *State) registerSourceConflictLocked(key sourceTableKey, decls map[string][]sourceDecl) {
	files := make([]string, 0, len(decls))
	for file := range decls {
		files = append(files, file)
	}
	slices.Sort(files)

	msg := fmt.Sprintf(
		"Table %s.%s already exists. Conflicting files: %s",
		key.Source,
		key.Table,
		strings.Join(files, ", "),
	)

	for _, file := range files {
		s.SourceFileErrors[file] = append(s.SourceFileErrors[file], sourceFileError{
			Key:     key,
			Message: msg,
		})
	}
	s.Logger.Error(msg)
}

func addSourceFileToTables(tables []*DbtTable, file string) {
	for _, tab := range tables {
		tab.SourceFile = file
	}
}

func (s *State) ProcessNewConfigYaml(file string) {
	s.Logger.Tracef("Processing config file (adding step): %s", file)

	data, err := os.ReadFile(file)
	if err != nil {
		s.Logger.Errorf("Error reading config file %s: %s", file, err)
		return
	}

	// Deduplicate: skip if the file content hasn't changed since last parse.
	// Neovim's atomic save fires several fsnotify events per save cycle; all of
	// them read identical bytes, so only the first one should proceed.
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	s.configFileHashesMu.Lock()
	if s.configFileHashes[file] == hash {
		s.configFileHashesMu.Unlock()
		s.Logger.Tracef("Config file %s unchanged (hash match). Skipping reparse.", file)
		return
	}
	s.configFileHashes[file] = hash
	s.configFileHashesMu.Unlock()

	basename := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if basename == "dbt_project" {
		project := DbtProject{}
		s.Logger.Tracef("Unmarshaling dbt_project: %s", file)
		if err := yaml.Unmarshal(data, &project); err != nil {
			s.Logger.Errorf("Error unmarshaling dbt_project %s: %s", file, err)
			return
		}
		s.SetDbtProject(project, file)
	} else {
		sources := DbtSources{}
		s.Logger.Tracef("Unmarshaling yml config file: %s", file)
		if err := yaml.Unmarshal(data, &sources); err != nil {
			s.Logger.Errorf("Error unmarshaling yml config file %s: %s", file, err)
			return
		}
		s.SetDbtSources(sources, file)
	}
}

func (s *State) RemoveConfigYaml(file string) {
	s.Logger.Tracef("Processing config file (removing step): %s", file)

	s.configFileHashesMu.Lock()
	delete(s.configFileHashes, file)
	s.configFileHashesMu.Unlock()

	s.ProjectMu.Lock()
	root := s.ProjectRoot
	activeConfig := s.ProjectConfigPath
	s.ProjectMu.Unlock()
	if root != "" && filepath.Clean(file) == filepath.Clean(activeConfig) {
		s.reconcileProject(root)
		return
	}

	s.DbtConfigMu.Lock()
	oldSources := s.DbtSourcesByFile[file]
	affectedKeys := affectedSourceTableKeys(oldSources, DbtSources{})
	for _, src := range oldSources.Sources {
		for key := range s.SourceTableIndex {
			if key.Source == src.Name {
				affectedKeys[key] = struct{}{}
			}
		}
	}
	s.removeSourceDeclsForFileLocked(file)
	delete(s.DbtSourcesByFile, file)
	delete(s.SourceFileErrors, file)
	s.recomputeAffectedSourceTablesLocked(affectedKeys)
	errs := s.allSourceErrorsLocked()
	s.DbtConfigMu.Unlock()
	s.setSourcesEnabled(len(errs) == 0)
	s.notifySourceState(errs)
}

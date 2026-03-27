package analysis

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"dbt_ls/lsp"

	"github.com/charmbracelet/log"
	rope "github.com/zyedidia/generic/rope"
	trie "github.com/zyedidia/generic/trie"
	"go.yaml.in/yaml/v4"
)

type Document struct {
	Data      *Rope
	EditCount *int
	Version   int
}

type State struct {
	Documents           map[string]*Document
	DbtConfigMu         sync.Mutex
	DbtConfig           DbtConfig
	Root                []lsp.WorkspaceFolder
	DbtModelsMu         sync.Mutex
	DbtModels           *trie.Trie[string]
	DbtModelExtension   string
	DbtConfigExtensions []string
	Logger              *log.Logger
	Writer              io.Writer
	ModelWatcher        DbtWatcher
	ConfigWatcher       DbtWatcher
}

type ScanCallback func(string) error

func NewState(logger *log.Logger, writer io.Writer, modelWatcher, configWatcher *DbtWatcher) *State {
	models := trie.New[string]()

	return &State{
		Documents:           map[string]*Document{},
		DbtModels:           models,
		DbtModelExtension:   ".sql",
		DbtConfigExtensions: []string{".yml", ".yaml"},
		Logger:              logger,
		Writer:              writer,
		ModelWatcher:        *modelWatcher,
		ConfigWatcher:       *configWatcher,
	}
}

func newCounter(x int) *int {
	var count *int
	count = new(int)
	*count = x

	return count
}

func (s *State) AddNewModelToIndex(file string) {
	s.DbtModelsMu.Lock()
	defer s.DbtModelsMu.Unlock()
	s.Logger.Debugf("Adding file: %s", file)
	s.DbtModels.Put(strings.TrimSuffix(filepath.Base(file), s.DbtModelExtension), file)
}

func (s *State) RemoveModelFromIndex(file string) {
	s.DbtModelsMu.Lock()
	defer s.DbtModelsMu.Unlock()
	s.Logger.Debugf("Removing file: %s", file)
	s.DbtModels.Remove(strings.TrimSuffix(filepath.Base(file), s.DbtModelExtension))
}

func (s *State) SetDbtProject(project DbtProject, file string) {
	s.DbtConfigMu.Lock()
	s.DbtConfig.Name = project.Name
	s.assignDbtSourcesLocked(project.DbtSources, file)
	s.DbtConfigMu.Unlock()
}

func (s *State) SetDbtSources(sources DbtSources, file string) {
	s.DbtConfigMu.Lock()
	s.assignDbtSourcesLocked(sources, file)
	s.DbtConfigMu.Unlock()
}

func (s *State) assignDbtSourcesLocked(sources DbtSources, file string) {
	if s.DbtConfig.Sources == nil {
		s.DbtConfig.Sources = make(map[string]*DbtConfigSource)
	}

	s.Logger.Debugf("Adding DbtSources from %s", file)
	for _, src := range sources.Sources {
		s.Logger.Debugf("Attempting to add DbtSource %s from %s", src.Name, file)
		s.mergeDbtSourcesLocked(src, file)
	}
}

func addSourceFileToTables(tables []*DbtTable, file string) {
	for _, tab := range tables {
		tab.SourceFile = file
	}
}

func (s *State) processDbtTablesLocked(srcName string, tables []*DbtTable, file string) {
	existingTables := s.DbtConfig.Sources[srcName].Tables
	for _, tab := range tables {
		existing, ok := existingTables[tab.Name]
		if ok && existing.SourceFile == file {
			s.Logger.Debugf("Table %s from file %s already processed. Ignoring.", tab.Name, file)
		} else if ok {
			s.Logger.Errorf("Table %s already exists. Conflicting files: %s, %s", tab.Name, file, existing.SourceFile)
		} else {
			s.Logger.Debugf("Merging table %s from %s into existing config.", tab.Name, file)
			existingTables[tab.Name] = tab
		}
	}
}

func (s *State) mergeDbtSourcesLocked(src *DbtSource, file string) {
	addSourceFileToTables(src.Tables, file)

	existing, ok := s.DbtConfig.Sources[src.Name]
	if !ok {
		s.Logger.Debugf("Source %s does not exist. Creating", src.Name)

		existing = &DbtConfigSource{
			Name:     src.Name,
			Database: src.Database,
			Schema:   src.Schema,
			Tables:   make(map[string]*DbtTable),
		}
		for _, tab := range src.Tables {
			existing.Tables[tab.Name] = tab
		}

		s.DbtConfig.Sources[src.Name] = existing
	} else {
		s.Logger.Debugf("Source %s already exists. Merging", src.Name)
		s.processDbtTablesLocked(src.Name, src.Tables, file)
	}
}

func (s *State) ProcessNewConfigYaml(file string) {
	s.Logger.Debugf("Processing config file (adding step): %s", file)

	data, err := os.ReadFile(file)
	if err != nil {
		s.Logger.Errorf("Error reading config file %s: %s", file, err)
		return
	}

	basename := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if basename == "dbt_project" {
		project := DbtProject{}
		s.Logger.Debugf("Unmarshaling dbt_project: %s", file)
		yaml.Unmarshal(data, &project)
		s.SetDbtProject(project, file)
	} else {
		sources := DbtSources{}
		s.Logger.Debugf("Unmarshaling yml config file: %s", file)
		yaml.Unmarshal(data, &sources)
		s.SetDbtSources(sources, file)
	}
}

func (s *State) RemoveConfigYaml(file string) {
	s.Logger.Debugf("Processing config file (removing step): %s", file)
}

func (s *State) findFilesRecursive(root string, exts []string) ([]string, error) {
	s.Logger.Debugf("Starting recursive search on %s", root)
	var matchingFiles []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		s.Logger.Debugf("Looking at %s", path)
		if err != nil {
			s.Logger.Errorf("Error while looking at %s: %s", path, err)
			return err // Handle errors during traversal
		}
		if d.IsDir() {
			s.Logger.Debugf("Found dir %s. Adding to Watcher", path)
			s.ModelWatcher.Watcher.Add(path)
			s.ConfigWatcher.Watcher.Add(path)
		} else if slices.Contains(exts, filepath.Ext(path)) {
			s.Logger.Debugf("Found %s file %s. Selected for LSP indexing", filepath.Ext(path), path)
			matchingFiles = append(matchingFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matchingFiles, nil
}

func (s *State) FindModelFilesRecursive(dirPattern string) error {
	s.Logger.Infof(
		"Starting scan. Looking for models with extension %s",
		s.DbtModelExtension,
	)

	models, err := s.findFilesRecursive(dirPattern, []string{s.DbtModelExtension})
	if err != nil {
		s.Logger.Errorf("Error finding model files recursively: %s", err)
		return err
	}

	s.Logger.Debugf("Adding %d models to state", len(models))
	for _, file := range models {
		s.AddNewModelToIndex(file)
	}
	return nil
}

func (s *State) FindConfigFilesRecursive(dirPattern string) error {
	s.Logger.Infof(
		"Starting scan. Looking for configs with extensions %s",
		s.DbtConfigExtensions,
	)
	configs, err := s.findFilesRecursive(dirPattern, s.DbtConfigExtensions)
	if err != nil {
		s.Logger.Errorf("Error finding config files recursively: %s", err)
		return err
	}

	s.Logger.Debugf("Adding %d configs to state", len(configs))
	for _, file := range configs {
		s.ProcessNewConfigYaml(file)
	}
	return nil
}

func (s *State) ScanAndWatchDirs(roots []string, callback ScanCallback) error {
	for _, dir := range roots {
		s.Logger.Debugf("Scanning for models in: %s", dir)
		dirPattern := fmt.Sprintf("%s", dir)

		callback(dirPattern)
	}
	return nil
}

func (s *State) OpenDocument(uri, text string, version int) {
	s.Logger.Infof("Document %s opened", uri)

	s.Documents[uri] = &Document{
		Data:      rope.New([]rune(text)),
		EditCount: newCounter(0),
		Version:   version,
	}
}

func (s *State) applyInsertion(doc *Document, offset int, change lsp.TextDocumentContentChangeEvent) {
	doc.Data.Insert(offset, []rune(change.Text))
	(*doc.EditCount)++
}

func (s *State) applyRemoval(doc *Document, startOffset, endOffset int) {
	doc.Data.Remove(startOffset, endOffset)
	(*doc.EditCount)++
}

func (s *State) applyUpdate(doc *Document, change lsp.TextDocumentContentChangeEvent, version int) {
	if *doc.EditCount > 200 {
		doc.Data.Rebalance()
	}

	startLine := change.Range.Start.Line
	startChar := change.Range.Start.Character
	endLine := change.Range.End.Line
	endChar := change.Range.End.Character

	startOffset := getOffset(doc.Data, startLine, startChar)
	endOffset := getOffset(doc.Data, endLine, endChar)

	switch {
	// Insertion: zero-length range, non-empty text
	case startOffset == endOffset && change.Text != "":
		s.applyInsertion(doc, startOffset, change)

	// Replacement: non-zero-length range, non-empty text
	case startOffset != endOffset && change.Text != "":
		s.applyRemoval(doc, startOffset, endOffset)
		s.applyInsertion(doc, startOffset, change)

	// Deletion: non-zero-length range, empty text
	case startOffset != endOffset && change.Text == "":
		s.applyRemoval(doc, startOffset, endOffset)
	}

	doc.Version = version

	s.Logger.Debugf("New version on Update: %d", doc.Version)
}

func (s *State) UpdateDocument(uri string, change lsp.TextDocumentContentChangeEvent, version int) {
	doc := s.Documents[uri]
	changeContents, err := json.Marshal(change.Range)
	if err != nil {
		s.Logger.Errorf("Error creating json from change.Range: %s", err)
	}

	s.Logger.Debugf("Document %s updated.", uri)
	s.Logger.Debugf("Text: %s", change.Text)
	s.Logger.Debugf("Text: %v", string(changeContents))
	s.applyUpdate(doc, change, version)
}

func (s *State) TextDocumentGoToDefinition(id int, params lsp.DefinitionParams) lsp.DefinitionResponse {
	doc := s.Documents[params.TextDocument.URI]
	line := getLine(doc.Data, params.Position.Line)
	s.Logger.Debugf("Looking for prefix with model reference in line %s", line)
	modelRef, check := extractModelRefUnderCursor(string(line), params.Position)

	response := lsp.DefinitionResponse{
		Response: lsp.Response{
			RPC: "2.0",
			ID:  &id,
		},
	}

	if check {
		s.Logger.Debugf("Found model reference %s in line", modelRef)
		model, ok := s.DbtModels.Get(modelRef)
		if ok {
			response.Result = &lsp.DefinitionLocation{
				TextDocumentIdentifier: lsp.TextDocumentIdentifier{
					URI: constructRefAsUri(model),
				},
				Range: lsp.TextDocumentPositionRange{
					Start: lsp.TextDocumentPosition{Line: 0, Character: 0},
					End:   lsp.TextDocumentPosition{Line: 0, Character: 0},
				},
			}
		} else {
			s.Logger.Errorf("No model with prefix %s found", modelRef)
		}

	} else {
		s.Logger.Errorf("No ref prefix found on line: %s", line)
	}

	return response
}

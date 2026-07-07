package analysis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"dbt_ls/lsp"
	"dbt_ls/rpc"

	"github.com/charmbracelet/log"
	rope "github.com/zyedidia/generic/rope"
	trie "github.com/zyedidia/generic/trie"
	"go.yaml.in/yaml/v4"
)

func WorkspacePath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	path := u.Path
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return path, nil
}

type Document struct {
	Data      *Rope
	EditCount *int
	Version   int
}

type State struct {
	Documents           map[string]*Document
	DbtConfigMu         sync.Mutex
	DbtConfig           DbtConfig
	DbtRoots            []string
	SourcesValid        bool
	SourceFileErrors    map[string][]sourceFileError // keyed by file path; guarded by DbtConfigMu
	NotifCh             chan lsp.ShowMessageNotification
	configFileHashes    map[string]string // file path → sha256 of last-processed content
	configFileHashesMu  sync.Mutex
	DbtSourcesByFile    map[string]DbtSources
	SourceTableIndex    map[sourceTableKey]map[string]sourceDecl
	Root                []lsp.WorkspaceFolder // LSP-provided workspace folders (Name/URI)
	RootPaths           []string              // parsed filesystem paths, index-aligned with Root
	ServerActive        bool
	DbtModelsMu         sync.Mutex
	DbtModels           *trie.Trie[string]
	DbtModelExtension   string
	DbtConfigExtensions []string
	ModelRoot           string
	ConfigRoot          string
	Logger              *log.Logger
	Writer              io.Writer
	ProjectWatcher      *DbtWatcher
	watchedDirs         map[string]struct{}
	watchedDirsMu       sync.Mutex
}

type sourceTableKey struct {
	Source string
	Table  string
}

type sourceDecl struct {
	Source *DbtSource
	Table  *DbtTable
	File   string
}

type sourceFileError struct {
	Key     sourceTableKey
	Message string
}

type ScanCallback func(string) error

func NewState(
	logger *log.Logger,
	writer io.Writer,
	projectWatcher *DbtWatcher,
) *State {
	models := trie.New[string]()

	return &State{
		Documents:           map[string]*Document{},
		SourcesValid:        true,
		SourceFileErrors:    map[string][]sourceFileError{},
		NotifCh:             make(chan lsp.ShowMessageNotification, 16),
		configFileHashes:    map[string]string{},
		DbtSourcesByFile:    map[string]DbtSources{},
		SourceTableIndex:    map[sourceTableKey]map[string]sourceDecl{},
		ServerActive:        false,
		DbtModels:           models,
		DbtModelExtension:   ".sql",
		DbtConfigExtensions: []string{".yml", ".yaml"},
		ModelRoot:           "models",
		ConfigRoot:          ".",
		Logger:              logger,
		Writer:              writer,
		ProjectWatcher:      projectWatcher,
		watchedDirs:         map[string]struct{}{},
	}
}

func (s *State) IsDbtProject(rootPath string) bool {
	for _, ext := range s.DbtConfigExtensions {
		file := fmt.Sprintf("%s/dbt_project%s", rootPath, ext)
		s.Logger.Debugf("Testing for %s", file)
		if _, err := os.Stat(file); err == nil {
			s.Logger.Debugf("Root marker identified. %s found", file)
			return true
		}
	}

	return false
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
	s.DbtModels.Put(
		strings.TrimSuffix(strings.ToLower(filepath.Base(file)), s.DbtModelExtension),
		file,
	)
}

func (s *State) RemoveModelFromIndex(file string) {
	s.DbtModelsMu.Lock()
	defer s.DbtModelsMu.Unlock()
	s.Logger.Debugf("Removing file: %s", file)
	s.DbtModels.Remove(
		strings.TrimSuffix(strings.ToLower(filepath.Base(file)), s.DbtModelExtension),
	)
}

func (s *State) SetDbtProject(project DbtProject, file string) {
	s.DbtConfigMu.Lock()
	s.DbtConfig.Name = project.Name
	s.updateDbtSourcesForFileLocked(project.DbtSources, file)
	errs := s.allSourceErrorsLocked()
	s.SourcesValid = len(errs) == 0
	s.DbtConfigMu.Unlock()

	s.notifySourceState(errs)
}

func (s *State) SetDbtSources(sources DbtSources, file string) {
	s.DbtConfigMu.Lock()
	s.updateDbtSourcesForFileLocked(sources, file)
	errs := s.allSourceErrorsLocked()
	s.SourcesValid = len(errs) == 0
	s.DbtConfigMu.Unlock()

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
				s.SourceTableIndex[key] = map[string]sourceDecl{}
			}

			s.SourceTableIndex[key][file] = sourceDecl{
				Source: src,
				Table:  tab,
				File:   file,
			}
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

		decls := s.SourceTableIndex[key]
		switch len(decls) {
		case 0:
			continue
		case 1:
			for _, decl := range decls {
				s.upsertDbtConfigTableLocked(decl)
			}
		default:
			s.registerSourceConflictLocked(key, decls)
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

func (s *State) registerSourceConflictLocked(key sourceTableKey, decls map[string]sourceDecl) {
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
	s.Logger.Debugf("Processing config file (adding step): %s", file)

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
		s.Logger.Debugf("Config file %s unchanged (hash match). Skipping reparse.", file)
		return
	}
	s.configFileHashes[file] = hash
	s.configFileHashesMu.Unlock()

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

func (s *State) addWatchDir(path string) error {
	if s.ProjectWatcher == nil || s.ProjectWatcher.Watcher == nil {
		return nil
	}

	s.watchedDirsMu.Lock()
	defer s.watchedDirsMu.Unlock()
	if _, ok := s.watchedDirs[path]; ok {
		return nil
	}
	if err := s.ProjectWatcher.Watcher.Add(path); err != nil {
		return err
	}
	s.watchedDirs[path] = struct{}{}
	return nil
}

func (s *State) resetWatchedDirs() {
	s.watchedDirsMu.Lock()
	defer s.watchedDirsMu.Unlock()
	s.watchedDirs = map[string]struct{}{}
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
		base := filepath.Base(path)
		if s.isSkippableDir(base, d) {
			s.Logger.Debugf("Ignoring %s branch of the tree", base)
			return fs.SkipDir
		}
		if d.IsDir() {
			s.Logger.Debugf("Found dir %s. Adding to ProjectWatcher", path)
			if err := s.addWatchDir(path); err != nil {
				s.Logger.Errorf("Error adding %s to ProjectWatcher: %s", path, err)
			}
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

func (s *State) ScanProjectFiles(root string) error {
	if err := s.FindModelFilesRecursive(root); err != nil {
		return err
	}
	return s.FindConfigFilesRecursive(root)
}

func (s *State) ScanRootPath(rootPath string) error {
	modelDir := filepath.Join(rootPath, s.ModelRoot)
	if err := s.ScanAndWatchDirs([]string{modelDir}, s.FindModelFilesRecursive); err != nil {
		return err
	}

	configDir := filepath.Join(rootPath, s.ConfigRoot)
	return s.ScanAndWatchDirs([]string{configDir}, s.FindConfigFilesRecursive)
}

func (s *State) ScanWorkspaceRoots() error {
	for _, rootPath := range s.RootPaths {
		if err := s.ScanRootPath(rootPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) ScanAndWatchDirs(roots []string, callback ScanCallback) error {
	for _, dir := range roots {
		s.Logger.Debugf("Scanning in: %s", dir)
		if err := callback(dir); err != nil {
			return err
		}
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

func (s *State) applyInsertion(
	doc *Document,
	offset int,
	change lsp.TextDocumentContentChangeEvent,
) {
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

func (s *State) TextDocumentGoToDefinition(
	id int,
	params lsp.DefinitionParams,
) lsp.DefinitionResponse {
	response := lsp.DefinitionResponse{
		Response: lsp.Response{
			RPC: "2.0",
			ID:  &id,
		},
	}
	if !s.ServerActive {
		return response
	}

	doc := s.Documents[params.TextDocument.URI]
	line := getLine(doc.Data, params.Position.Line)
	s.Logger.Debugf("Looking for prefix with model reference in line %s", line)
	modelRef, check := extractModelRefUnderCursor(string(line), params.Position)

	if check {
		s.Logger.Debugf("Found model reference %s in line", modelRef)
		model, ok := s.DbtModels.Get(strings.ToLower(modelRef))
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

// allSourceErrorsLocked flattens SourceFileErrors into a single slice.
// Must be called with DbtConfigMu held.
func (s *State) allSourceErrorsLocked() []string {
	seen := map[string]struct{}{}
	var all []string
	for _, errs := range s.SourceFileErrors {
		for _, err := range errs {
			if _, ok := seen[err.Message]; ok {
				continue
			}
			seen[err.Message] = struct{}{}
			all = append(all, err.Message)
		}
	}
	return all
}

// notifySourceState sends a window/showMessage notification to the client
// reflecting the current source config validity. Call this after releasing
// DbtConfigMu to avoid holding the lock during a write to stdout.
func (s *State) notifySourceState(errs []string) {
	if len(errs) == 0 {
		return
	}
	msg := "dbt-ls: source completion unavailable — config errors detected:\n"
	for _, e := range errs {
		msg += "  • " + e + "\n"
	}
	s.NotifCh <- lsp.ShowMessageNotification{
		Notification: lsp.Notification{Method: "window/showMessage"},
		Params:       lsp.ShowMessageParams{Type: lsp.MessageTypeWarning, Message: msg},
	}
}

// DrainNotifications reads from NotifCh and writes window/showMessage
// notifications to the client. Run this as a long-lived goroutine.
func (s *State) DrainNotifications() {
	for notif := range s.NotifCh {
		s.sendShowMessage(notif.Params.Type, notif.Params.Message)
	}
}

func (s *State) sendShowMessage(messageType int, message string) {
	notif := lsp.ShowMessageNotification{
		Notification: lsp.Notification{Method: "window/showMessage"},
		Params:       lsp.ShowMessageParams{Type: messageType, Message: message},
	}
	msgIn, err := rpc.EncodeMsg(notif)
	if err != nil {
		s.Logger.Errorf("Couldn't encode ShowMessageNotification: %s", err)
		return
	}

	s.Writer.Write([]byte(msgIn))
	s.Logger.Infof("Sent ShowMessageNotification of type %d", notif.Params.Type)
}

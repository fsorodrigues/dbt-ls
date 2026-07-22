package analysis

import (
	"io"
	"net/url"
	"runtime"
	"sync"

	"dbt_ls/logger"
	"dbt_ls/lsp"

	trie "github.com/zyedidia/generic/trie"
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

type State struct {
	Documents           map[string]*Document
	DbtConfigMu         sync.Mutex
	ProjectMu           sync.RWMutex
	ProjectLifecycleMu  sync.Mutex
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
	ProjectRoot         string
	ProjectConfigPath   string
	ShutdownRequested   bool
	DbtModelsMu         sync.Mutex
	DbtModels           *trie.Trie[string]
	DbtModelExtension   string
	DbtConfigExtensions []string
	ModelRoots          []string
	ConfigRoot          string
	Logger              *logger.Logger
	Writer              io.Writer
	ProjectWatcher      *DbtWatcher
	watchedDirs         map[string]struct{}
	watchedDirsMu       sync.Mutex
}

func (s *State) IsServerActive() bool {
	s.ProjectMu.RLock()
	defer s.ProjectMu.RUnlock()
	return s.ServerActive
}

func (s *State) SetServerActive(active bool) {
	s.setServerActive(active)
}

func (s *State) SetProjectRoot(root string) {
	s.ProjectMu.Lock()
	s.ProjectRoot = root
	s.ProjectMu.Unlock()
}

func (s *State) ProjectRootPath() string {
	s.ProjectMu.RLock()
	defer s.ProjectMu.RUnlock()
	return s.ProjectRoot
}

func (s *State) setServerActive(active bool) {
	s.ProjectMu.Lock()
	s.ServerActive = active
	s.ProjectMu.Unlock()
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

func NewState(
	logger *logger.Logger,
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
		ShutdownRequested:   false,
		DbtModels:           models,
		DbtModelExtension:   ".sql",
		DbtConfigExtensions: []string{".yml", ".yaml"},
		ModelRoots:          []string{"models"},
		ConfigRoot:          ".",
		Logger:              logger,
		Writer:              writer,
		ProjectWatcher:      projectWatcher,
		watchedDirs:         map[string]struct{}{},
	}
}

func (s *State) Shutdown() {
	s.setServerActive(false)
	s.ShutdownRequested = true
}

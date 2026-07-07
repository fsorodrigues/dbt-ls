package analysis

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"
	"sync"

	"dbt_ls/lsp"

	"github.com/charmbracelet/log"
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

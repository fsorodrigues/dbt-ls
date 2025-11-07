package analysis

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"dbt_lsp/lsp"

	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
	rope "github.com/zyedidia/generic/rope"
	trie "github.com/zyedidia/generic/trie"
)

type Document struct {
	Data      *Rope
	EditCount *int
}

type State struct {
	Documents         map[string]*Document
	Root              []lsp.WorkspaceFolder
	DbtModels         *trie.Trie[string]
	DbtModelExtension string
	Logger            *log.Logger
	Writer            io.Writer
	Watcher           *fsnotify.Watcher
}

func NewState(logger *log.Logger, writer io.Writer, watcher *fsnotify.Watcher) State {
	models := trie.New[string]()

	return State{
		Documents:         map[string]*Document{},
		DbtModels:         models,
		DbtModelExtension: ".r",
		Logger:            logger,
		Writer:            writer,
		Watcher:           watcher,
	}
}

func newCounter(x int) *int {
	var count *int
	count = new(int)
	*count = x

	return count
}

func (s *State) AddNewModelToIndex(file string) {
	s.Logger.Debugf("Adding file: %s", file)
	s.DbtModels.Put(strings.TrimSuffix(filepath.Base(file), s.DbtModelExtension), file)
}

func (s *State) RemoveModelFromIndex(file string) {
	s.DbtModels.Remove(strings.TrimSuffix(filepath.Base(file), s.DbtModelExtension))
}

func extractRefPrefix(text string) (string, bool) {
	// Matches ref("prefix or ref('prefix
	re := regexp.MustCompile(`.*ref\(['"]([a-zA-Z0-9_\-]*)`)
	match := re.FindStringSubmatch(text)
	if match == nil {
		return "", false
	}

	return match[1], true
}

func (s *State) findFilesRecursive(root string, ext string) ([]string, error) {
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
			s.Watcher.Add(path)
		} else if filepath.Ext(path) == ext {
			s.Logger.Debugf("Found file %s. Selected for LSP indexing", path)
			matchingFiles = append(matchingFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matchingFiles, nil
}

func (s *State) ScanAndWatchDirs(roots []string) error {
	s.Logger.Infof(
		"Starting scan on project. Looking and watching for models with extension %s",
		s.DbtModelExtension,
	)
	for _, dir := range roots {
		s.Logger.Debugf("Scanning for models in: %s", dir)
		dirPattern := fmt.Sprintf("%s", dir)

		files, err := s.findFilesRecursive(dirPattern, s.DbtModelExtension)
		if err != nil {
			s.Logger.Errorf("Error finding model files recursively: %s", err)
			return err
		}

		s.Logger.Debugf("Adding %d models to state", len(files))
		for _, file := range files {
			s.AddNewModelToIndex(file)
		}
	}
	return nil
}

func (s *State) OpenDocument(uri, text string) {
	s.Logger.Infof("Document %s opened", uri)
	s.Logger.Debugf("Text contents on Open: %s", text)

	s.Documents[uri] = &Document{
		Data:      rope.New([]rune(text)),
		EditCount: newCounter(0),
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

func (s *State) applyUpdate(doc *Document, change lsp.TextDocumentContentChangeEvent) {
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

	s.Logger.Debugf("Text contents on Update: %s", string(doc.Data.Value()))
}

func (s State) UpdateDocument(uri string, change lsp.TextDocumentContentChangeEvent) {
	doc := s.Documents[uri]
	changeContents, err := json.Marshal(change.Range)
	if err != nil {
		s.Logger.Errorf("Error creating json from change.Range: %s", err)
	}

	s.Logger.Infof("Document %s updated.", uri)
	s.Logger.Debugf("Text: %s", change.Text)
	s.Logger.Debugf("Text: %v", string(changeContents))
	s.applyUpdate(doc, change)
}

func (s State) TextDocumentCodeCompletion(id int, completion lsp.CompletionParams) lsp.CompletionResponse {
	doc := s.Documents[completion.TextDocument.URI]
	offset := getOffset(doc.Data, completion.Position.Line, completion.Position.Line)
	hunk := doc.Data.Slice(max(0, offset-10), min(offset+10, doc.Data.Len()))
	prefix, check := extractRefPrefix(string(hunk))

	response := lsp.CompletionResponse{
		Response: lsp.Response{
			RPC: "2.0",
			ID:  &id,
		},
		Result: lsp.CompletionList{
			IsIncomplete: true,
			Items:        []lsp.CompletionItem{},
		},
	}

	if check {
		models := s.DbtModels.KeysWithPrefix(prefix)
		s.Logger.Debugf("%s", models)
		for _, modKey := range models {
			modVal, ok := s.DbtModels.Get(modKey)
			if !ok {
				s.Logger.Debug("Error getting value from Trie")
			}
			response.Result.Items = append(response.Result.Items, lsp.CompletionItem{
				Label:         modKey,
				Kind:          18,
				Detail:        "dbt Model",
				Documentation: modVal,
				TextEdit: lsp.CompletionTextEdit{
					Range: lsp.TextDocumentPositionRange{
						Start: completion.Position,
						End:   completion.Position,
					},
					NewText: strings.TrimPrefix(modKey, prefix),
				},
			})
		}

		s.Logger.Debugf("TextDocumentCodeCompletion ready. Contains %d items", len(response.Result.Items))
	}

	return response
}

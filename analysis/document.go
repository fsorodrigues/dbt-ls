package analysis

import (
	"encoding/json"

	"dbt_ls/lsp"

	rope "github.com/zyedidia/generic/rope"
)

type Document struct {
	Data      *Rope
	EditCount *int
	Version   int
}

func newCounter(x int) *int {
	count := new(int)
	*count = x
	return count
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

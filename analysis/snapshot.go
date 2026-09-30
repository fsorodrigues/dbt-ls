package analysis

import "github.com/fsorodrigues/dbt-ls/lsp"

// Snapshot is a detached copy of a document's text. It is safe to read
// without holding any lock; it will not observe edits made after it was taken.
type Snapshot struct {
	Text    []rune
	Version int
}

func (s *State) Snapshot(uri string) (Snapshot, bool) {
	s.DocumentsMu.RLock()
	defer s.DocumentsMu.RUnlock()

	doc, ok := s.Documents[uri]
	if !ok || doc == nil {
		return Snapshot{}, false
	}
	return Snapshot{
		Text:    doc.Data.Slice(0, doc.Data.Len()),
		Version: doc.Version,
	}, true
}

// Offset converts an LSP position to a rune offset into Text.
// Out-of-range positions clamp to the end of the document.
// LSP characters are UTF-16 code units, but this intentionally counts runes;
// they differ for astral-plane characters. Full UTF-16 support is deferred.
func (sn Snapshot) Offset(pos lsp.TextDocumentPosition) int {
	line, char := 0, 0
	for i, r := range sn.Text {
		if line == pos.Line && char == pos.Character {
			return i
		}
		if r == '\n' {
			line, char = line+1, 0
		} else {
			char++
		}
	}
	return len(sn.Text)
}

// Position converts a rune offset back to an LSP position.
func (sn Snapshot) Position(offset int) lsp.TextDocumentPosition {
	line, char := 0, 0
	for i := 0; i < offset && i < len(sn.Text); i++ {
		if sn.Text[i] == '\n' {
			line, char = line+1, 0
		} else {
			char++
		}
	}
	return lsp.TextDocumentPosition{Line: line, Character: char}
}

// Range builds an LSP range from a pair of rune offsets.
func (sn Snapshot) Range(start, end int) lsp.TextDocumentPositionRange {
	return lsp.TextDocumentPositionRange{
		Start: sn.Position(start),
		End:   sn.Position(end),
	}
}

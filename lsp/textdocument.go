package lsp

type TextDocumentItem struct {
	TextDocumentIdentifier
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

func (p *TextDocumentPosition) MoveChar(n int) TextDocumentPosition {
	newPos := &TextDocumentPosition{
		Line:      p.Line,
		Character: p.Character + n,
	}

	return *newPos
}

type TextDocumentPositionRange struct {
	Start TextDocumentPosition `json:"start"`
	End   TextDocumentPosition `json:"end"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     TextDocumentPosition   `json:"position"`
}

type VersionTextDocumentIdentifier struct {
	TextDocumentIdentifier
	Version int `json:"version"`
}

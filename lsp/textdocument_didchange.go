package lsp

type DidChangeTextDocumentNotification struct {
	Notification
	Params DidChangeTextDocumentParams `json:"params"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionTextDocumentIdentifier    `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type TextDocumentContentChangeEvent struct {
	Range TextDocumentContentChangeEventRange `json:"range"`
	Text  string                              `json:"text"`
}

type TextDocumentContentChangeEventRange struct {
	Start TextDocumentPosition `json:"start"`
	End   TextDocumentPosition `json:"end"`
}

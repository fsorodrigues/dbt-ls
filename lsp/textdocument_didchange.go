package lsp

type DidChangeTextDocumentNotification struct {
	Notification
	Params DidChangeTextDocumentParams `json:"params"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionTextDocumentIdentifier    `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"ContentChanges"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

package lsp

type WillSaveTextDocumentNotification struct {
	Notification
	Params WillSaveTextDocumentParams `json:"params"`
}

type WillSaveTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
	Reason       int              `json:"reason"`
}

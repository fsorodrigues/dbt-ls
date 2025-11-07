package lsp

type CompletionParams struct {
	TextDocumentPositionParams
}

type CompletionRequest struct {
	Request
	Params CompletionParams `json:"params"`
}

type CompletionTextEdit struct {
	Range   TextDocumentPositionRange `json:"range"`
	NewText string                    `json:"newText"`
}

type CompletionItem struct {
	Label         string             `json:"label"`
	Detail        string             `json:"detail"`
	Kind          int                `json:"kind"`
	Documentation string             `json:"documentation"`
	TextEdit      CompletionTextEdit `json:"textEdit"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type CompletionResponse struct {
	Response
	Result CompletionList `json:"result"`
}

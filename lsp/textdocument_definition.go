package lsp

type DefinitionParams struct {
	TextDocumentPositionParams
}

type DefinitionRequest struct {
	Request
	Params DefinitionParams `json:"params"`
}

type DefinitionLocation struct {
	TextDocumentIdentifier
	Range TextDocumentPositionRange `json:"range"`
}

type DefinitionResponse struct {
	Response
	Result *DefinitionLocation `json:"result"`
}

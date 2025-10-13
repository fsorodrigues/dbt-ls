package lsp

type (
	ClientInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	InitializeRequestParams struct {
		ClientInfo *ClientInfo `json:"clientInfo"`
	}

	InitializeRequest struct {
		Request
		Params InitializeRequestParams `json:"params"`
	}

	ServerCapabilities struct {
		TextDocumentSync int `json:"textDocumentSync"`
	}

	ServerInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	InitializeResult struct {
		Capabilities ServerCapabilities `json:"capabilities"`
		ServerInfo   ServerInfo         `json:"serverInfo"`
	}

	InitializeResponse struct {
		Response
		Result InitializeResult `json:"result"`
	}
)

func NewInitializeResponse(id int) InitializeResponse {
	return InitializeResponse{
		Response: Response{
			RPC: "2.0",
			ID:  &id,
		},
		Result: InitializeResult{
			Capabilities: ServerCapabilities{
				TextDocumentSync: 1,
			},
			ServerInfo: ServerInfo{
				Name:    "dbt_lsp",
				Version: "0.0",
			},
		},
	}
}

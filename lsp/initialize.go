package lsp

type (
	ClientInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	WorkspaceFolder struct {
		URI  string `json:"uri"`
		Name string `json:"name"`
	}

	InitializeRequestParams struct {
		ClientInfo       *ClientInfo       `json:"clientInfo"`
		WorkspaceFolders []WorkspaceFolder `json:"workspaceFolders"`
	}

	InitializeRequest struct {
		Request
		Params InitializeRequestParams `json:"params"`
	}

	TextDocumentSyncCapability struct {
		OpenClose bool `json:"openClose"`
		Change    int  `json:"change"`
		WillSave  bool `json:"willSave"`
	}

	CompletionProviderCapability struct {
		TriggerCharacters []rune `json:"triggerCharacters"`
	}

	ServerCapabilities struct {
		TextDocumentSync   TextDocumentSyncCapability   `json:"textDocumentSync"`
		CompletionProvider CompletionProviderCapability `json:"completionProvider"`
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
				TextDocumentSync: TextDocumentSyncCapability{
					OpenClose: true,
					WillSave:  true,
					Change:    2,
				},
				CompletionProvider: CompletionProviderCapability{
					TriggerCharacters: []rune("."),
				},
			},
			ServerInfo: ServerInfo{
				Name:    "dbt_lsp",
				Version: "0.0",
			},
		},
	}
}

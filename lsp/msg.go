package lsp

type ClientMessage interface {
	GetMethod() string
}

type Request struct {
	RPC    string `json:"jsonrpc"`
	ID     int    `json:"id"`
	Method string `json:"method"`
}

type Notification struct {
	Method string `json:"method"`
}

func (m Notification) GetMethod() string {
	return m.Method
}

func (m Request) GetMethod() string {
	return m.Method
}

type Response struct {
	RPC string `json:"jsonrpc"`
	ID  *int   `json:"id,omitempty"`
}

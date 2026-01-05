package lsp

type Envelope struct {
	Method   string
	Contents []byte
	Message  ClientMessage
}

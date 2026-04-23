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

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Response
	Error ResponseError `json:"error"`
}

const ErrorCodeInvalidParams = -32602

func NewErrorResponse(id int, code int, message string) ErrorResponse {
	return ErrorResponse{
		Response: Response{
			RPC: "2.0",
			ID:  &id,
		},
		Error: ResponseError{
			Code:    code,
			Message: message,
		},
	}
}

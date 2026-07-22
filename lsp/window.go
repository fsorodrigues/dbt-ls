package lsp

const (
	MessageTypeError   = 1
	MessageTypeWarning = 2
	MessageTypeInfo    = 3
	MessageTypeLog     = 4
	MethodShowMessage  = "window/showMessage"
)

type ShowMessageParams struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}

type ShowMessageNotification struct {
	Notification
	Params ShowMessageParams `json:"params"`
}

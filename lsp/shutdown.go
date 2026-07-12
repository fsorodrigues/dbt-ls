package lsp

type (
	ShutdownRequest struct {
		Request
	}

	ShutdownResponse struct {
		Response
	}
)

func NewShutdownResponse(id int) ShutdownResponse {
	return ShutdownResponse{
		Response: Response{
			RPC: "2.0",
			ID:  &id,
		},
	}
}

package analysis

import (
	"dbt_ls/lsp"
	"dbt_ls/rpc"
)

// allSourceErrorsLocked flattens SourceFileErrors into a single slice.
// Must be called with DbtConfigMu held.
func (s *State) allSourceErrorsLocked() []string {
	seen := map[string]struct{}{}
	var all []string
	for _, errs := range s.SourceFileErrors {
		for _, err := range errs {
			if _, ok := seen[err.Message]; ok {
				continue
			}
			seen[err.Message] = struct{}{}
			all = append(all, err.Message)
		}
	}
	return all
}

// notifySourceState sends a window/showMessage notification to the client
// reflecting the current source config validity. Call this after releasing
// DbtConfigMu to avoid holding the lock during a write to stdout.
func (s *State) notifySourceState(errs []string) {
	if len(errs) == 0 {
		return
	}
	msg := "dbt-ls: source completion unavailable — config errors detected:\n"
	for _, e := range errs {
		msg += "  • " + e + "\n"
	}
	s.NotifCh <- lsp.ShowMessageNotification{
		Notification: lsp.Notification{Method: "window/showMessage"},
		Params:       lsp.ShowMessageParams{Type: lsp.MessageTypeWarning, Message: msg},
	}
}

// DrainNotifications reads from NotifCh and writes window/showMessage
// notifications to the client. Run this as a long-lived goroutine.
func (s *State) DrainNotifications() {
	for notif := range s.NotifCh {
		s.sendShowMessage(notif.Params.Type, notif.Params.Message)
	}
}

func (s *State) sendShowMessage(messageType int, message string) {
	notif := lsp.ShowMessageNotification{
		Notification: lsp.Notification{Method: "window/showMessage"},
		Params:       lsp.ShowMessageParams{Type: messageType, Message: message},
	}
	msgIn, err := rpc.EncodeMsg(notif)
	if err != nil {
		s.Logger.Errorf("Couldn't encode ShowMessageNotification: %s", err)
		return
	}

	s.Writer.Write([]byte(msgIn))
	s.Logger.Infof("Sent ShowMessageNotification of type %d", notif.Params.Type)
}

package analysis

import (
	"context"
	"strings"

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
	var msg strings.Builder
	msg.WriteString("dbt-ls: source completion unavailable — config errors detected:\n")
	for _, e := range errs {
		msg.WriteString("  • " + e + "\n")
	}
	s.ShowMessage(lsp.MessageTypeWarning, msg.String())
}

// ShowMessage queues a window/showMessage notification for the client.
func (s *State) ShowMessage(messageType int, message string) {
	s.NotifCh <- lsp.ShowMessageParams{Type: messageType, Message: message}
}

// DrainNotifications reads from NotifCh and writes window/showMessage
// notifications to the client. Run this as a long-lived goroutine.
func (s *State) DrainNotifications(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.Logger.Debug("Stopped notification drain")
			return

		case notif, ok := <-s.NotifCh:
			if !ok {
				return
			}
			s.sendShowMessage(notif)
		}
	}
}

func (s *State) sendShowMessage(params lsp.ShowMessageParams) {
	notif := lsp.ShowMessageNotification{
		Notification: lsp.Notification{Method: lsp.MethodShowMessage},
		Params:       params,
	}
	msgIn, err := rpc.EncodeMsg(notif)
	if err != nil {
		s.Logger.Errorf("Couldn't encode ShowMessageNotification: %s", err)
		return
	}

	if _, err := s.Writer.Write([]byte(msgIn)); err != nil {
		s.Logger.Errorf("Couldn't write ShowMessageNotification: %s", err)
		return
	}
	s.Logger.Infof("Sent ShowMessageNotification of type %d", notif.Params.Type)
}

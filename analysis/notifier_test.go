package analysis

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"dbt_ls/lsp"
	"dbt_ls/rpc"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type signalWriter struct {
	bytes.Buffer
	written chan struct{}
}

func (w *signalWriter) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	close(w.written)
	return n, err
}

func TestShowMessageQueuesParams(t *testing.T) {
	s := newTestState()
	s.ShowMessage(lsp.MessageTypeWarning, "warning")

	got := <-s.NotifCh
	if got != (lsp.ShowMessageParams{Type: lsp.MessageTypeWarning, Message: "warning"}) {
		t.Fatalf("unexpected queued params: %#v", got)
	}
}

func TestDrainNotificationsWritesShowMessage(t *testing.T) {
	output := &signalWriter{written: make(chan struct{})}
	s := newTestState()
	s.Writer = output

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.DrainNotifications(ctx)
		close(done)
	}()
	s.ShowMessage(lsp.MessageTypeInfo, "hello")

	<-output.written
	cancel()
	<-done

	method, content, err := rpc.DecodeMsg(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if method != lsp.MethodShowMessage {
		t.Fatalf("method = %q, want %q", method, lsp.MethodShowMessage)
	}
	if !bytes.Contains(content, []byte(`"type":3`)) || !bytes.Contains(content, []byte(`"message":"hello"`)) {
		t.Fatalf("unexpected notification content: %s", content)
	}
}

func TestSendShowMessageHandlesWriteError(t *testing.T) {
	s := newTestState()
	s.Writer = failingWriter{}
	s.sendShowMessage(lsp.ShowMessageParams{Type: lsp.MessageTypeError, Message: "failure"})
}

var _ io.Writer = failingWriter{}

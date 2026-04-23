package rpc

import (
	"testing"
)

type TestMsg struct {
	Text string
}

func TestEncodeMsg(t *testing.T) {
	expected := "Content-Length: 36\r\n\r\n{\"Text\":\"Hello from the other side\"}"
	got, _ := EncodeMsg(TestMsg{Text: "Hello from the other side"})

	if got != expected {
		t.Fatalf("Expected %s, got %s", expected, got)
	}
}

func TestDecodeMsg(t *testing.T) {
	msg := "Content-Length: 38\r\n\r\n{\"method\":\"Hello from the other side\"}"
	expectedMsg := "Hello from the other side"
	expectedLength := 38
	gotMsg, gotLength, decodeErr := DecodeMsg([]byte(msg))
	if decodeErr != nil {
		t.Fatalf("Error decoding msg: %s", decodeErr)
	}

	if gotMsg != expectedMsg {
		t.Fatalf("Expected msg %s, got msg %s", expectedMsg, gotMsg)
	}

	if len(gotLength) != expectedLength {
		t.Fatalf("Length expected %d, got %d", expectedLength, len(gotLength))
	}
}

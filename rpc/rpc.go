package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type JsonRpcMsg struct {
	Method string `json:"method"`
}

func EncodeMsg(msg any) (string, error) {
	content, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(content), content), nil
}

func DecodeMsg(msg []byte) (string, int, error) {
	header, content, msgFound := bytes.Cut(msg, []byte{'\r', '\n', '\r', '\n'})
	if !msgFound {
		return "", 0, errors.New("Did not find separator \\r\\n\\r\\n while parsing message")
	}

	_, contentLengthBytes, lengthFound := bytes.Cut(header, []byte{':', ' '})
	if !lengthFound {
		return "", 0, errors.New("Did not find content length while parsing message header")
	}

	contentLength, err := strconv.Atoi(string(contentLengthBytes))
	if err != nil {
		return "", 0, errors.New("Could not parse the length of message value in message header")
	}

	var baseMsg JsonRpcMsg
	if err := json.Unmarshal(content[:contentLength], &baseMsg); err != nil {
		return "", 0, errors.New("Error unmarshalling json data")
	}

	return baseMsg.Method, contentLength, nil
}

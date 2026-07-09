package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type JsonRpcMsg struct {
	JsonRpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Method  string `json:"method"`
}

func EncodeMsg(msg any) (string, error) {
	content, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(content), content), nil
}

func DecodeMsg(msg []byte) (string, []byte, error) {
	header, content, msgFound := bytes.Cut(msg, []byte{'\r', '\n', '\r', '\n'})
	if !msgFound {
		return "", nil, errors.New("did not find separator \\r\\n\\r\\n while parsing message")
	}

	_, contentLengthBytes, lengthFound := bytes.Cut(header, []byte{':', ' '})
	if !lengthFound {
		return "", nil, errors.New("did not find content length while parsing message header")
	}

	contentLength, err := strconv.Atoi(string(contentLengthBytes))
	if err != nil {
		return "", nil, errors.New("could not parse the length of message value in message header")
	}

	var baseMsg JsonRpcMsg
	if err := json.Unmarshal(content[:contentLength], &baseMsg); err != nil {
		return "", nil, errors.New("error unmarshalling json data")
	}

	return baseMsg.Method, content[:contentLength], nil
}

func Split(data []byte, _ bool) (advance int, token []byte, err error) {
	header, content, streamFound := bytes.Cut(data, []byte{'\r', '\n', '\r', '\n'})
	if !streamFound {
		return 0, nil, nil
	}

	_, contentLengthBytes, lengthFound := bytes.Cut(header, []byte{':', ' '})
	if !lengthFound {
		return 0, nil, errors.New("did not find content length header while parsing stdin stream")
	}

	contentLength, err := strconv.Atoi(string(contentLengthBytes))
	if err != nil {
		return 0, nil, errors.New("could not parse the length of stdin stream in stream header")
	}

	if len(content) < contentLength {
		return 0, nil, nil
	}

	totalLength := len(header) + 4 + contentLength

	return totalLength, data[:totalLength], nil
}

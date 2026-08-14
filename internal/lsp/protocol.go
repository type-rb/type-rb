package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcStream struct {
	reader *bufio.Reader
	output io.Writer
}

func newRPCStream(input io.Reader, output io.Writer) *rpcStream {
	return &rpcStream{reader: bufio.NewReader(input), output: output}
}

func (s *rpcStream) read() (message, error) {
	length := -1
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return message{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			return message{}, fmt.Errorf("invalid LSP header %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 0 {
				return message{}, fmt.Errorf("invalid LSP Content-Length %q", value)
			}
			length = parsed
		}
	}
	if length < 0 {
		return message{}, fmt.Errorf("LSP message is missing Content-Length")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(s.reader, payload); err != nil {
		return message{}, err
	}
	var result message
	if err := json.Unmarshal(payload, &result); err != nil {
		return message{}, fmt.Errorf("invalid LSP JSON: %w", err)
	}
	return result, nil
}

func (s *rpcStream) write(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.output, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err = s.output.Write(payload)
	return err
}

func success(id json.RawMessage, result any) response {
	if result == nil {
		result = json.RawMessage("null")
	}
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func failure(id json.RawMessage, code int, err error) response {
	return response{JSONRPC: "2.0", ID: id, Error: &responseError{Code: code, Message: err.Error()}}
}

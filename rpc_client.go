package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func requestViewerRPC(ctx context.Context, client *http.Client, endpoint, method string, params map[string]any) (json.RawMessage, error) {
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid viewer endpoint %q", endpoint)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/rpc"

	rpcRequest := queryRPCRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
	body, err := json.Marshal(rpcRequest)
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", method, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create viewer request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("contact viewer at %s: %w", base.String(), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("viewer returned HTTP %s", response.Status)
	}

	var rpcResponse queryRPCResponse
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&rpcResponse); err != nil {
		return nil, fmt.Errorf("decode viewer response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode viewer response: additional JSON value")
		}
		return nil, fmt.Errorf("decode viewer response: trailing data: %w", err)
	}
	if rpcResponse.JSONRPC != rpcRequest.JSONRPC {
		return nil, fmt.Errorf("decode viewer response: invalid jsonrpc version %q", rpcResponse.JSONRPC)
	}
	var responseID int
	if len(rpcResponse.ID) == 0 || json.Unmarshal(rpcResponse.ID, &responseID) != nil || responseID != rpcRequest.ID {
		return nil, errors.New("decode viewer response: response id does not match request id")
	}
	if rpcResponse.Error != nil {
		return nil, fmt.Errorf("viewer %s error %d: %s", method, rpcResponse.Error.Code, rpcResponse.Error.Message)
	}
	if len(rpcResponse.Result) == 0 {
		return nil, errors.New("decode viewer response: missing result")
	}
	return rpcResponse.Result, nil
}

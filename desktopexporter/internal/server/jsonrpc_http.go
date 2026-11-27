package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

// maxRPCRequestBodyBytes caps how much of an /rpc request body we will read.
// JSON-RPC requests in this app are small (a method name and a few params), so
// 1 MB is generous headroom while preventing a buggy or malicious client from
// exhausting memory with an unbounded body.
const maxRPCRequestBodyBytes = 1 << 20 // 1 MB

func (s *Server) rpcHandler(writer http.ResponseWriter, request *http.Request) {
	if !isAllowedRPCOrigin(request) {
		http.Error(writer, "Forbidden: cross-origin RPC request", http.StatusForbidden)
		return
	}
	requestCtx := (propagation.TraceContext{}).Extract(request.Context(), propagation.HeaderCarrier(request.Header))
	ctx, endRPC := s.tel.RPCRequest(requestCtx)
	var method string
	var responseBytes int
	var rpcError error
	defer func() { endRPC(method, responseBytes, rpcError) }()

	bodyLimit := int64(maxRPCRequestBodyBytes)
	limited := http.MaxBytesReader(writer, request.Body, bodyLimit)
	body, err := io.ReadAll(limited)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			rpcError = fmt.Errorf("%w: request body too large", jsonrpc2.ErrInvalidRequest)
			responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
			return
		}
		rpcError = jsonrpc2.ErrInternal
		responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
		return
	}

	// encoding/json replaces invalid UTF-8 silently. Reject malformed requests
	// before decoding.
	if !utf8.Valid(body) {
		rpcError = jsonrpc2.ErrParse
		responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
		return
	}
	message, err := jsonrpc2.DecodeMessage(body)
	if err != nil {
		rpcError = jsonrpc2.ErrParse
		responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
		return
	}

	rpcRequest, ok := message.(*jsonrpc2.Request)
	if !ok {
		rpcError = jsonrpc2.ErrInvalidRequest
		responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
		return
	}

	// DuckDB builds response JSON inside the query; EncodeMessage is the only
	// separately measured encoding step.
	method = rpcRequest.Method

	result, err := s.jsonrpcHandler.Handle(ctx, rpcRequest)
	if err != nil {
		rpcError = err
		responseBytes = s.sendJSONRPCResponse(ctx, writer, rpcRequest.ID, nil, err)
		return
	}

	responseBytes = s.sendJSONRPCResponse(ctx, writer, rpcRequest.ID, result, nil)
}

// sendJSONRPCResponse writes the response and returns its encoded size in
// bytes, which the caller records on the RPC span. Returns 0 when the response
// could not be produced. ctx carries the RPC span, so the encode child span
// hangs off the call it belongs to.
func (s *Server) sendJSONRPCResponse(ctx context.Context, writer http.ResponseWriter, id jsonrpc2.ID, result any, rpcError error) int {
	response, err := jsonrpc2.NewResponse(id, result, rpcError)
	if err != nil {
		s.logger.Error("creating JSON-RPC response", zap.Error(err))
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return 0
	}

	// DuckDB query time includes JSON construction, so only EncodeMessage is
	// measured separately.
	endEncode := s.tel.RPCEncode(ctx)
	bytes, err := jsonrpc2.EncodeMessage(response)
	endEncode(err)
	if err != nil {
		s.logger.Error("encoding JSON-RPC response", zap.Error(err))
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return 0
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(bytes)
	return len(bytes)
}

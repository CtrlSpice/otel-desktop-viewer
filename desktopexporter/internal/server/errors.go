package server

import "golang.org/x/exp/jsonrpc2"

// Custom JSON-RPC error codes
const (
	ErrCodeTraceNotFound  = -32001
	ErrCodeLogNotFound    = -32002
	ErrCodeMetricNotFound = -32003
	ErrCodeInvalidTraceID = -32004
	ErrCodeInvalidLogID   = -32005
)

// Custom JSON-RPC errors
var (
	ErrTraceNotFound  = jsonrpc2.NewError(ErrCodeTraceNotFound, "Trace not found")
	ErrLogsNotFound   = jsonrpc2.NewError(ErrCodeLogNotFound, "Log not found")
	ErrMetricNotFound = jsonrpc2.NewError(ErrCodeMetricNotFound, "Metric not found")
	ErrInvalidTraceID = jsonrpc2.NewError(ErrCodeInvalidTraceID, "Invalid trace ID")
	ErrInvalidLogID   = jsonrpc2.NewError(ErrCodeInvalidLogID, "Invalid log ID")
)

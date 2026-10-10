package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestImportConfigRPC(t *testing.T) {
	for _, port := range []int{4318, 54318, 0} {
		s, err := NewServer("127.0.0.1:0", port, nil, zap.NewNop(), nil)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":7,"method":"getImportConfig"}`))
		response := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(response, request)
		var reply struct {
			ID     int `json:"id"`
			Result struct {
				Port int `json:"otlpHttpPort"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &reply))
		require.Equal(t, 7, reply.ID)
		if port == 0 {
			require.NotNil(t, reply.Error)
			require.Contains(t, reply.Error.Message, "not configured")
		} else {
			require.Nil(t, reply.Error)
			require.Equal(t, port, reply.Result.Port)
		}
	}
}

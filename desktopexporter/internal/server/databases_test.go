package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/jsonrpc2"
)

type stubDatabaseManager struct {
	list    DatabaseList
	openErr error
	newErr  error
}

func (m *stubDatabaseManager) ListDatabases(context.Context) (DatabaseList, error) {
	return m.list, nil
}

func (m *stubDatabaseManager) OpenDatabase(_ context.Context, name string) (DatabaseList, error) {
	if m.openErr != nil {
		return DatabaseList{}, m.openErr
	}
	m.list.Active = name
	for i := range m.list.Databases {
		m.list.Databases[i].Active = m.list.Databases[i].Name == name
	}
	return m.list, nil
}

func (m *stubDatabaseManager) NewDatabase(_ context.Context, name string) (DatabaseList, error) {
	if m.newErr != nil {
		return DatabaseList{}, m.newErr
	}
	m.list.Active = name
	m.list.Databases = append(m.list.Databases, DatabaseEntry{
		Name:       name,
		SizeBytes:  4096,
		ModifiedAt: "2026-11-26T18:00:00Z",
		Active:     true,
	})
	return m.list, nil
}

func TestDatabaseRPCMethodsAndWireSchema(t *testing.T) {
	h := setupHandler(t)
	mgr := &stubDatabaseManager{
		list: DatabaseList{
			Managed:   true,
			Active:    "current.duckdb",
			Directory: "/tmp/viewer",
			Databases: []DatabaseEntry{
				{Name: "current.duckdb", SizeBytes: 12288, ModifiedAt: "2026-11-26T17:00:00Z", Active: true},
				{Name: "checkout.duckdb", SizeBytes: 24576, ModifiedAt: "2026-11-26T16:00:00Z", Active: false},
			},
		},
	}
	h.databases = mgr

	doc := openRPCDocument()
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	schemaFor := func(name string) map[string]any {
		for _, m := range doc["methods"].([]rpcMethodDescription) {
			if m.Name == name {
				return m.Result.Schema
			}
		}
		t.Fatalf("missing OpenRPC method %s", name)
		return nil
	}

	for _, tc := range []struct {
		method string
		params any
		active string
	}{
		{"listDatabases", []any{}, "current.duckdb"},
		{"openDatabase", map[string]any{"name": "checkout.duckdb"}, "checkout.duckdb"},
		{"newDatabase", []any{"incident.duckdb"}, "incident.duckdb"},
	} {
		t.Run(tc.method, func(t *testing.T) {
			got, err := h.Handle(context.Background(), createRequest(tc.method, tc.params))
			require.NoError(t, err)
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			var decoded any
			require.NoError(t, decodeParams(encoded, &decoded))
			require.NoError(t, checkRPCWireSchema(schemaFor(tc.method), decoded, schemas))
			assert.Equal(t, tc.active, decoded.(map[string]any)["active"])
		})
	}
}

func TestDatabaseRPCErrors(t *testing.T) {
	h := setupHandler(t)

	// Without a manager (in-memory), listDatabases succeeds with managed=false,
	// while open/new return ErrDatabaseSwitchUnsupported.
	got, err := h.Handle(context.Background(), createRequest("listDatabases", []any{}))
	require.NoError(t, err)
	list := got.(DatabaseList)
	assert.False(t, list.Managed)
	assert.Empty(t, list.Databases)

	_, err = h.Handle(context.Background(), createRequest("openDatabase", []any{"a.duckdb"}))
	require.ErrorIs(t, err, ErrDatabaseSwitchUnsupported)

	_, err = h.Handle(context.Background(), createRequest("newDatabase", []any{"a.duckdb"}))
	require.ErrorIs(t, err, ErrDatabaseSwitchUnsupported)

	// Missing or blank name fails parameter validation.
	for _, method := range []string{"openDatabase", "newDatabase"} {
		_, err = h.Handle(context.Background(), createRequest(method, []any{"   "}))
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	}

	// Store errors map to their custom JSON-RPC codes.
	h.databases = &stubDatabaseManager{openErr: store.ErrDatabaseNotFound, newErr: store.ErrDatabaseAlreadyExists}
	_, err = h.Handle(context.Background(), createRequest("openDatabase", []any{"missing.duckdb"}))
	require.ErrorIs(t, err, ErrDatabaseNotFound)
	_, err = h.Handle(context.Background(), createRequest("newDatabase", []any{"dup.duckdb"}))
	require.ErrorIs(t, err, ErrDatabaseAlreadyExists)
}

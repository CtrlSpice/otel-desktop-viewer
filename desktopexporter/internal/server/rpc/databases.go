package rpc

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"golang.org/x/exp/jsonrpc2"
)

// ErrInvalidDatabaseName indicates a caller supplied an invalid session name.
var ErrInvalidDatabaseName = errors.New("invalid database name")

// DatabaseEntry describes one managed capture file in the viewer's app-data directory.
type DatabaseEntry struct {
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes"`
	ModifiedAt string `json:"modifiedAt"`
	Active     bool   `json:"active"`
}

// DatabaseList describes the viewer's storage mode and available managed sessions.
type DatabaseList struct {
	Managed   bool            `json:"managed"`
	Active    string          `json:"active"`
	Databases []DatabaseEntry `json:"databases"`
}

// DatabaseManager lists and switches managed databases for the running viewer.
type DatabaseManager interface {
	ListDatabases(ctx context.Context) (DatabaseList, error)
	OpenDatabase(ctx context.Context, name string) (DatabaseList, error)
	NewDatabase(ctx context.Context, name string) (DatabaseList, error)
}

func (h *Handler) listDatabases(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := validateNoParams(req.Params); err != nil {
		return nil, err
	}
	if h.databases == nil {
		active := ""
		if h.store != nil {
			if p := h.store.DBPath(); p != "" {
				active = filepath.Base(p)
			}
		}
		return DatabaseList{
			Managed:   false,
			Active:    active,
			Databases: []DatabaseEntry{},
		}, nil
	}
	list, err := h.databases.ListDatabases(ctx)
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	if list.Databases == nil {
		list.Databases = []DatabaseEntry{}
	}
	return list, nil
}

func (h *Handler) openDatabase(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	name, err := parseDatabaseNameParam(req.Params)
	if err != nil {
		return nil, err
	}
	if h.databases == nil {
		return nil, ErrDatabaseSwitchUnsupported
	}
	list, err := h.databases.OpenDatabase(ctx, name)
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	if list.Databases == nil {
		list.Databases = []DatabaseEntry{}
	}
	return list, nil
}

func (h *Handler) newDatabase(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	name, err := parseDatabaseNameParam(req.Params)
	if err != nil {
		return nil, err
	}
	if h.databases == nil {
		return nil, ErrDatabaseSwitchUnsupported
	}
	list, err := h.databases.NewDatabase(ctx, name)
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	if list.Databases == nil {
		list.Databases = []DatabaseEntry{}
	}
	return list, nil
}

func parseDatabaseNameParam(raw []byte) (string, error) {
	params, err := decodePositionalParams(raw, 1, 1)
	if err != nil {
		return "", err
	}
	name, ok := params[0].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("name must be a non-empty string: %w", jsonrpc2.ErrInvalidParams)
	}
	return name, nil
}

// mapDatabaseError translates database session errors into JSON-RPC errors.
func mapDatabaseError(err error) (error, bool) {
	switch {
	case errors.Is(err, ErrInvalidDatabaseName):
		return fmt.Errorf("%v: %w", err, jsonrpc2.ErrInvalidParams), true
	case errors.Is(err, store.ErrDatabaseNotFound):
		return ErrDatabaseNotFound, true
	case errors.Is(err, store.ErrDatabaseAlreadyExists):
		return ErrDatabaseAlreadyExists, true
	case errors.Is(err, store.ErrDatabaseSwitchUnsupported):
		return ErrDatabaseSwitchUnsupported, true
	case errors.Is(err, store.ErrSchemaIncompatible):
		return ErrDatabaseIncompatible, true
	default:
		return nil, false
	}
}

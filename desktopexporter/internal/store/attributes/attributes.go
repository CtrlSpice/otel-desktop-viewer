// Package attributes provides scoped keys-first discovery from the shared
// structured search catalogue. Native rows remain the source of exact values.
package attributes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
)

var ErrAttributesStoreInternal = errors.New("attributes store internal error")

type SearchOptions = searchindex.DiscoveryOptions

func Search(ctx context.Context, db *sql.DB, term string) (json.RawMessage, error) {
	return SearchScoped(ctx, db, SearchOptions{Term: term})
}
func SearchScoped(ctx context.Context, db *sql.DB, options SearchOptions) (json.RawMessage, error) {
	result, err := searchindex.Discover(ctx, db, options)
	if err != nil {
		return nil, fmt.Errorf("Search: %w: %w", ErrAttributesStoreInternal, err)
	}
	return result, nil
}

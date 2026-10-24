//go:build !evals

package features

import (
	"context"
	"errors"
)

const EvalsBuilt = false

// Flags has no provider in a public build. Runtime configuration cannot add
// functionality that was excluded at build time.
type Flags struct{}

func New(_ context.Context, file string) (*Flags, error) {
	if file != "" {
		return nil, errors.New("experimental features are not included in this build; build from source with make build EVALS=1")
	}
	return &Flags{}, nil
}

func (*Flags) EvalsEnabled(context.Context) bool { return false }
func (*Flags) Shutdown(context.Context) error    { return nil }

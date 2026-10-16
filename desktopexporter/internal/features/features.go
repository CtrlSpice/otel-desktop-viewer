// Package features owns local, opt-in experiments. Its OpenFeature API is
// isolated from other components in the Collector process.
package features

import (
	"context"
	"fmt"

	flagd "github.com/open-feature/go-sdk-contrib/providers/flagd/pkg"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
)

const Evals = "evals.enabled"
const ExperimentalLabel = "Experimental and unsupported"

type Flags struct {
	api    *openfeature.EvaluationAPI
	client *openfeature.Client
}

// New starts a local file provider only when a file is explicitly configured.
// Without a file, no provider, watcher or network connection is started.
func New(ctx context.Context, file string) (*Flags, error) {
	f := &Flags{}
	if file == "" {
		return f, nil
	}
	provider, err := flagd.NewProvider(
		flagd.WithFileResolver(),
		flagd.WithOfflineFilePath(file),
		flagd.WithOfflinePollMs(5000),
	)
	if err != nil {
		return nil, fmt.Errorf("configure local feature flags: %w", err)
	}
	f.api = isolated.NewAPI()
	if err := f.api.SetProviderAndWait(ctx, provider); err != nil {
		_ = f.api.Shutdown(context.Background())
		return nil, fmt.Errorf("load local feature flags: %w", err)
	}
	f.client = f.api.NewClient()
	return f, nil
}

// EvalsEnabled defaults to false on missing flags, type errors or provider errors.
// Call it at each backend action, not only when constructing navigation.
func (f *Flags) EvalsEnabled(ctx context.Context) bool {
	if f == nil || f.client == nil {
		return false
	}
	enabled, err := f.client.BooleanValue(ctx, Evals, false, openfeature.EvaluationContext{})
	return err == nil && enabled
}

func (f *Flags) Shutdown(ctx context.Context) error {
	if f == nil || f.api == nil {
		return nil
	}
	return f.api.Shutdown(ctx)
}

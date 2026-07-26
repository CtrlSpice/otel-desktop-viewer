package sharedcomponent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/collector/component"
)

type mockComponent struct {
	startCount    atomic.Int32
	shutdownCount atomic.Int32
}

func (m *mockComponent) Start(context.Context, component.Host) error {
	m.startCount.Add(1)
	return nil
}

func (m *mockComponent) Shutdown(context.Context) error {
	m.shutdownCount.Add(1)
	return nil
}

func TestGetOrAddReturnsExistingInstance(t *testing.T) {
	comps := NewSharedComponents[string, *mockComponent]()
	createCount := 0
	create := func() (*mockComponent, error) {
		createCount++
		return &mockComponent{}, nil
	}

	first, err := comps.GetOrAdd("key", create)
	if err != nil {
		t.Fatalf("GetOrAdd: %v", err)
	}
	second, err := comps.GetOrAdd("key", create)
	if err != nil {
		t.Fatalf("GetOrAdd: %v", err)
	}

	if first != second {
		t.Error("expected the same *SharedComponent for the same key")
	}
	if createCount != 1 {
		t.Errorf("create called %d times, want 1", createCount)
	}
}

func TestGetOrAddPropagatesCreateError(t *testing.T) {
	comps := NewSharedComponents[string, *mockComponent]()
	wantErr := errors.New("boom")

	if _, err := comps.GetOrAdd("key", func() (*mockComponent, error) {
		return nil, wantErr
	}); !errors.Is(err, wantErr) {
		t.Errorf("got err %v, want %v", err, wantErr)
	}

	// A failed create must not poison the key.
	sc, err := comps.GetOrAdd("key", func() (*mockComponent, error) {
		return &mockComponent{}, nil
	})
	if err != nil || sc == nil {
		t.Errorf("expected successful create after failure, got (%v, %v)", sc, err)
	}
}

func TestStartAndShutdownRunOnce(t *testing.T) {
	comps := NewSharedComponents[string, *mockComponent]()
	sc, err := comps.GetOrAdd("key", func() (*mockComponent, error) {
		return &mockComponent{}, nil
	})
	if err != nil {
		t.Fatalf("GetOrAdd: %v", err)
	}
	inner := sc.Unwrap()

	ctx := context.Background()
	for range 3 {
		if err := sc.Start(ctx, nil); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	if got := inner.startCount.Load(); got != 1 {
		t.Errorf("underlying Start ran %d times, want 1", got)
	}

	for range 3 {
		if err := sc.Shutdown(ctx); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	}
	if got := inner.shutdownCount.Load(); got != 1 {
		t.Errorf("underlying Shutdown ran %d times, want 1", got)
	}
}

func TestShutdownRemovesKeyForRecreation(t *testing.T) {
	comps := NewSharedComponents[string, *mockComponent]()
	create := func() (*mockComponent, error) { return &mockComponent{}, nil }

	first, _ := comps.GetOrAdd("key", create)
	if err := first.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	second, _ := comps.GetOrAdd("key", create)
	if first == second {
		t.Error("expected a fresh *SharedComponent after Shutdown removed the key")
	}
}

// TestConcurrentGetOrAddAndShutdown exercises the map under concurrent
// GetOrAdd and Shutdown calls; meaningful under -race.
func TestConcurrentGetOrAddAndShutdown(t *testing.T) {
	comps := NewSharedComponents[string, *mockComponent]()
	ctx := context.Background()

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				key := fmt.Sprintf("key-%d", (worker+i)%4)
				sc, err := comps.GetOrAdd(key, func() (*mockComponent, error) {
					return &mockComponent{}, nil
				})
				if err != nil {
					t.Errorf("GetOrAdd: %v", err)
					return
				}
				if err := sc.Start(ctx, nil); err != nil {
					t.Errorf("Start: %v", err)
					return
				}
				if i%3 == 0 {
					if err := sc.Shutdown(ctx); err != nil {
						t.Errorf("Shutdown: %v", err)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
}

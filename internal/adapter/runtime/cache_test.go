/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	"k8s.io/apimachinery/pkg/types"
)

func revision(i int) execution.RevisionKey {
	return execution.RevisionKey{DefinitionUID: types.UID(fmt.Sprint("uid-", i)), Generation: 1, SchemaVersion: "v1alpha1", SemanticsVersion: 1, OperatorBuild: "test-build", AdapterVersion: "1.0.0"}
}
func TestCacheIdleLRUAndActiveBounds(t *testing.T) {
	cache := execution.NewCache()
	builds := map[execution.RevisionKey]int{}
	acquire := func(key execution.RevisionKey) *execution.Snapshot {
		t.Helper()
		snapshot, err := cache.Acquire(context.Background(), key, func(ctx context.Context) (adapter.Adapter, error) { builds[key]++; return successfulCompiler(ctx) })
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	for i := 0; i < 128; i++ {
		acquire(revision(i)).Release()
	}
	// Refresh key 0; insertion must evict key 1, not key 0.
	acquire(revision(0)).Release()
	acquire(revision(128)).Release()
	acquire(revision(0)).Release()
	if builds[revision(0)] != 1 {
		t.Fatal("cache hit did not refresh LRU")
	}
	acquire(revision(1)).Release()
	if builds[revision(1)] != 2 {
		t.Fatal("least-recently-used revision was not evicted")
	}
	var pinned []*execution.Snapshot
	for i := 0; i < 4; i++ {
		pinned = append(pinned, acquire(revision(200+i)))
	}
	if _, err := cache.Acquire(context.Background(), revision(300), successfulCompiler); err == nil {
		t.Fatal("fifth active snapshot accepted")
	}
	pinned[0].Release()
	pinned[0].Release()
	extra := acquire(revision(301))
	if _, err := cache.Acquire(context.Background(), revision(302), successfulCompiler); err == nil {
		t.Fatal("double release freed another reservation")
	}
	extra.Release()
	for _, snapshot := range pinned[1:] {
		snapshot.Release()
	}
}

func TestCacheRevisionIsolationAndPanicCleanup(t *testing.T) {
	cache := execution.NewCache()
	for i := 0; i < 4; i++ {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected compiler panic")
				}
			}()
			_, _ = cache.Acquire(context.Background(), revision(i), func(context.Context) (adapter.Adapter, error) { panic("compile") })
		}()
	}
	builds := 0
	compile := func(ctx context.Context) (adapter.Adapter, error) { builds++; return successfulCompiler(ctx) }
	key := revision(10)
	variants := []execution.RevisionKey{key, key, key, key, key, key, key}
	variants[1].DefinitionUID = "recreated"
	variants[2].Generation++
	variants[3].SchemaVersion = "v1beta1"
	variants[4].SemanticsVersion++
	variants[5].OperatorBuild = "new-build"
	variants[6].AdapterVersion = "2.0.0"
	for _, key := range variants {
		snapshot, err := cache.Acquire(context.Background(), key, compile)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Revision() != key || snapshot.Adapter() == nil {
			t.Fatal("wrong snapshot identity")
		}
		snapshot.Release()
	}
	if builds != len(variants) {
		t.Fatalf("distinct revisions reused: builds=%d", builds)
	}
}

func TestCachePinsSurviveIdleEvictionAndCompileOutsideLock(t *testing.T) {
	cache := execution.NewCache()
	pinned, err := cache.Acquire(context.Background(), revision(0), successfulCompiler)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Release()
	for i := 1; i <= 130; i++ {
		snapshot, err := cache.Acquire(context.Background(), revision(i), successfulCompiler)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Release()
	}
	hit, err := cache.Acquire(context.Background(), revision(0), func(context.Context) (adapter.Adapter, error) {
		t.Error("active snapshot evicted")
		return nil, errors.New("unexpected compile")
	})
	if err != nil {
		t.Fatal(err)
	}
	hit.Release()
	entered := make(chan struct{}, 2)
	resume := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(resume) })
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			snapshot, err := cache.Acquire(context.Background(), revision(1000+i), func(ctx context.Context) (adapter.Adapter, error) {
				entered <- struct{}{}
				select {
				case <-resume:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return successfulCompiler(ctx)
			})
			if snapshot != nil {
				snapshot.Release()
			}
			outcomes <- err
		}(i)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("compiler blocked under cache mutex")
		}
	}
	once.Do(func() { close(resume) })
	for i := 0; i < 2; i++ {
		if err := <-outcomes; err != nil {
			t.Fatal(err)
		}
	}
}

func TestCacheCanceledCompilationIsNotCachedOrPinned(t *testing.T) {
	cache := execution.NewCache()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	snapshot, err := cache.Acquire(ctx, revision(0), func(ctx context.Context) (adapter.Adapter, error) { calls++; cancel(); return successfulCompiler(ctx) })
	if err == nil || snapshot != nil {
		t.Fatal("canceled compilation published a snapshot")
	}
	var active []*execution.Snapshot
	defer func() {
		for _, snapshot := range active {
			snapshot.Release()
		}
	}()
	for i := 0; i < 4; i++ {
		snapshot, err := cache.Acquire(context.Background(), revision(i), func(ctx context.Context) (adapter.Adapter, error) { calls++; return successfulCompiler(ctx) })
		if err != nil {
			t.Fatal(err)
		}
		active = append(active, snapshot)
	}
	if calls != 5 {
		t.Fatalf("canceled result reused: compiler calls=%d", calls)
	}
}

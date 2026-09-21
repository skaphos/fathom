/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"container/list"
	"context"
	"fmt"
	"sync"

	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/types"
)

// RevisionKey includes every identity that can change compiled semantics. Names
// alone cannot reuse a revision after definition recreation or operator upgrade.
type RevisionKey struct {
	DefinitionUID    types.UID
	Generation       int64
	SchemaVersion    string
	SemanticsVersion int32
	OperatorBuild    string
	AdapterVersion   string
}
type cachedRevision struct {
	key     RevisionKey
	adapter adapter.Adapter
	users   int
	idle    *list.Element
}

// Cache belongs to the shared runtime pool. It retains up to 128 idle revisions
// plus four active snapshot reservations. Compilation runs outside its mutex;
// a panic/error/cancellation releases the reservation without caching a failure.
type Cache struct {
	mu        sync.Mutex
	revisions map[RevisionKey]*cachedRevision
	idle      list.List
	active    int
}

func NewCache() *Cache { return &Cache{revisions: map[RevisionKey]*cachedRevision{}} }

// Snapshot pins an immutable compiled adapter. Release is idempotent, including
// under concurrent calls. Callers must obey adapter's immutable run contract.
type Snapshot struct {
	cache    *Cache
	entry    *cachedRevision
	released bool
}

func (s *Snapshot) Adapter() adapter.Adapter { return s.entry.adapter }
func (s *Snapshot) Revision() RevisionKey    { return s.entry.key }

func (c *Cache) Acquire(ctx context.Context, key RevisionKey, compile func(context.Context) (adapter.Adapter, error)) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if compile == nil || key.DefinitionUID == "" || key.Generation < 1 || key.SchemaVersion == "" || key.SemanticsVersion < 1 || key.OperatorBuild == "" || key.AdapterVersion == "" {
		return nil, fmt.Errorf("InvalidDefinition: cache requires complete revision provenance and compiler")
	}
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if c.active >= limits.MaxConcurrentRuns {
		c.mu.Unlock()
		return nil, &Failure{Reason: "WorkLimitExceeded", Detail: "active compiled snapshot limit exceeded"}
	}
	c.active++
	if entry := c.revisions[key]; entry != nil {
		c.pin(entry)
		c.mu.Unlock()
		return &Snapshot{cache: c, entry: entry}, nil
	}
	c.mu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			c.mu.Lock()
			c.active--
			c.mu.Unlock()
		}
	}()
	compileCtx, cancel := context.WithTimeout(ctx, limits.MaxCompileDuration)
	defer cancel()
	compiled, err := compile(compileCtx)
	if compileCtx.Err() != nil {
		return nil, compileCtx.Err()
	}
	if err != nil {
		return nil, err
	}
	if compiled == nil {
		return nil, fmt.Errorf("InvalidDefinition: compiler returned no snapshot")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := compileCtx.Err(); err != nil {
		return nil, err
	}
	entry := c.revisions[key]
	if entry == nil {
		entry = &cachedRevision{key: key, adapter: compiled}
		c.revisions[key] = entry
	}
	// Concurrent misses may construct equivalent snapshots; retain the already
	// published immutable entry and discard the redundant local construction.
	c.pin(entry)
	reserved = false
	return &Snapshot{cache: c, entry: entry}, nil
}
func (c *Cache) pin(entry *cachedRevision) {
	if entry.idle != nil {
		c.idle.Remove(entry.idle)
		entry.idle = nil
	}
	entry.users++
}
func (s *Snapshot) Release() {
	if s == nil || s.cache == nil {
		return
	}
	c := s.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if s.released {
		return
	}
	s.released = true
	c.active--
	s.entry.users--
	if s.entry.users > 0 {
		return
	}
	s.entry.idle = c.idle.PushFront(s.entry)
	for c.idle.Len() > limits.MaxCachedRevisions {
		oldest := c.idle.Back()
		entry := oldest.Value.(*cachedRevision)
		c.idle.Remove(oldest)
		entry.idle = nil
		delete(c.revisions, entry.key)
	}
}

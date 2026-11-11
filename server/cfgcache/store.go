// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cfgcache

import (
	"context"
	"iter"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config"

	cfgcachecommon "go.chromium.org/luci/server/cfgcache/common"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
	"go.chromium.org/luci/server/module"
)

// SameInputs indicates that `a` and `b` had the same inputs.
//
// NOTE: input_revision and transformed_hash are intentionally ignored - only
// the input hash, input_size and transform_version are inputs to the
// transformation algorithm.
func SameInputs(a, b *cfgcachepb.Meta) bool {
	return a.TransformVersion == b.TransformVersion &&
		a.InputHash == b.InputHash &&
		a.InputSize == b.InputSize
}

// Store manages persistence (to durable storage, like a database) of
// transformed config snapshots and their version metadata for a
// [ServicePipeline] or [ProjectPipeline] definition.
type Store[StoreMsg proto.Message] interface {
	// ModuleDependencies should return any module dependencies it needs.
	ModuleDependencies() []module.Dependency

	// LoadMeta returns the stored *cfgcachepb.Metas for config sets currently stored in this
	// store for `pipelineID`.
	LoadMeta(ctx context.Context, pipelineID string) (map[config.Set]*cfgcachepb.Meta, error)

	// Put persists `out` for `(pipelineID, cs)` at the given `m`.
	//
	// Ownership of `out` is given to the Store.
	Put(ctx context.Context, pipelineID string, cs config.Set, m *cfgcachepb.Meta, out StoreMsg) error

	// Delete removes all stored records and metadata associated with `sets` in
	// `pipelineID`.
	Delete(ctx context.Context, pipelineID string, sets []config.Set) error
}

// GettableStore is an optional extension of [Store] for storage backends
// that also support in-memory refreshing.
type GettableStore[StoreMsg proto.Message] interface {
	Store[StoreMsg]

	// Get loads the stored values belonging to `sets` in `pipelineID`.
	//
	// If a given config set is missing, do not yield it.
	//
	// This is an iterator to allow the Store to best determine how to handle
	// batching / memory management in the event there are many config sets
	// and/or large amounts of content. The Store may yield the config sets in
	// any order.
	//
	// Yielding entries which are not in `sets` is an error, as is yielding a
	// given config set multiple times.
	//
	// It is OK if the yielded StoredConfig for a given config.Set does not match
	// the given Meta - yield the meta+value which was *actually* fetched.
	Get(ctx context.Context, pipelineID string, sets map[config.Set]*cfgcachepb.Meta) iter.Seq2[*cfgcachecommon.StoredConfig[StoreMsg], error]
}

// genericStore is a type-erased [Store] (and optional [GettableStore]) stored
// on [ingestConfig].
type genericStore interface {
	GettableStore[proto.Message]
	isGettable() bool
}

// storeAdapter erases the StoreMsg type parameter on a pipeline's [Store].
//
// `gettable` is non-nil iff the underlying [Store] implements [GettableStore].
// Pipelines with in-memory refresh enabled (ic.toMemory != nil) require a
// [GettableStore] at registration time, so `gettable` is always non-nil for
// them. [NoMemoryRefresh] pipelines (ic.toMemory == nil) only require [Store],
// so `gettable` is populated only when the chosen backend also implements
// [GettableStore] (e.g. dsstore.Store or memstore.Store).
type storeAdapter[StoreMsg proto.Message] struct {
	Store[StoreMsg]
	gettable GettableStore[StoreMsg]
}

func (c storeAdapter[StoreMsg]) isGettable() bool {
	return c.gettable != nil
}

func (c storeAdapter[StoreMsg]) Put(ctx context.Context, pipelineID string, cs config.Set, m *cfgcachepb.Meta, out proto.Message) error {
	return c.Store.Put(ctx, pipelineID, cs, m, out.(StoreMsg))
}

func (c storeAdapter[StoreMsg]) Get(ctx context.Context, pipelineID string, sets map[config.Set]*cfgcachepb.Meta) iter.Seq2[*cfgcachecommon.StoredConfig[proto.Message], error] {
	return func(yield func(*cfgcachecommon.StoredConfig[proto.Message], error) bool) {
		// We want to be as transparent as possible, so forward both sc and err,
		// even when err != nil. The underlying iterator will control termination.
		for sc, err := range c.gettable.Get(ctx, pipelineID, sets) {
			var gsc *cfgcachecommon.StoredConfig[proto.Message]
			if sc != nil {
				gsc = &cfgcachecommon.StoredConfig[proto.Message]{
					ConfigSet: sc.ConfigSet,
					Meta:      sc.Meta,
					Config:    sc.Config,
				}
			}
			if !yield(gsc, err) {
				return
			}
		}
	}
}

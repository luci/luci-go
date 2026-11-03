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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config"

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

// genericStore is a type-erased [Store] stored on [ingestConfig].
type genericStore = Store[proto.Message]

type storeAdapter[StoreMsg proto.Message] struct {
	Store[StoreMsg]
}

func (c storeAdapter[StoreMsg]) Put(ctx context.Context, pipelineID string, cs config.Set, m *cfgcachepb.Meta, out proto.Message) error {
	return c.Store.Put(ctx, pipelineID, cs, m, out.(StoreMsg))
}

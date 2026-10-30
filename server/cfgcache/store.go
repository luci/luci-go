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

	"go.chromium.org/luci/server/module"
)

// Meta holds the version metadata for a stored config snapshot.
type Meta struct {
	// InputHash is the combined content hash of the source config files in the
	// snapshot.
	InputHash string

	// TransformVersion is the [ServicePipeline.TransformVersion] or
	// [ProjectPipeline.TransformVersion] that produced the stored config.
	TransformVersion int

	// ContentSize is the size in bytes of the *stored* config message.
	ContentSize int64
}

// SameInput indicates that m and other had the same set of inputs.
//
// ContentSize is a computed output, and so is not considered.
func (m Meta) SameInput(other Meta) bool {
	return m.TransformVersion == other.TransformVersion && m.InputHash == other.InputHash
}

// Store manages persistence (to durable storage, like a database) of
// transformed config snapshots and their version metadata for a
// [ServicePipeline] or [ProjectPipeline] definition.
type Store[StoreMsg proto.Message] interface {
	// ModuleDependencies should return any module dependencies it needs.
	ModuleDependencies() []module.Dependency

	// LoadMeta returns the stored Metas for config sets currently stored in this
	// store for `pipelineID`.
	LoadMeta(ctx context.Context, pipelineID string) (map[config.Set]Meta, error)

	// Put persists `out` for `(pipelineID, cs)` at the given `m`.
	//
	// Ownership of `out` is given to the Store.
	Put(ctx context.Context, pipelineID string, cs config.Set, m Meta, out StoreMsg) error

	// Delete removes all stored records and metadata associated with `sets` in
	// `pipelineID`.
	Delete(ctx context.Context, pipelineID string, sets []config.Set) error
}

// StoredConfig holds a stored config value and its version metadata.
type StoredConfig[StoreMsg proto.Message] struct {
	ConfigSet config.Set
	Meta      Meta
	Config    StoreMsg
}

// genericStore is a type-erased [Store] stored on [ingestConfig].
type genericStore = Store[proto.Message]

type storeAdapter[StoreMsg proto.Message] struct {
	Store[StoreMsg]
}

func (c storeAdapter[StoreMsg]) Put(ctx context.Context, pipelineID string, cs config.Set, m Meta, out proto.Message) error {
	return c.Store.Put(ctx, pipelineID, cs, m, out.(StoreMsg))
}

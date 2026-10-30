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

package dsstore

import (
	"context"
	"encoding/base64"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/server/cfgcache"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
)

// cachedConfigKind is the Datastore entity kind used by [Store].
const cachedConfigKind = "cfgcachev2.CachedConfig"

type cachedConfig[StoreMsg proto.Message] struct {
	_kind string                `gae:"$kind,cfgcachev2.CachedConfig"`
	Extra datastore.PropertyMap `gae:",extra"`

	ID cachedConfigIDProperty `gae:"$id"`

	Config StoreMsg `gae:",zstd"`
	Meta   contentMetaProperty
}

// Store implements [cfgcache.Store] using the `gae/service/datastore` library.
//
// Each stored config is stored in its own datastore entity root, and the
// stored config is compressed with `zstd`.
type Store[StoreMsg proto.Message] struct{}

var _ cfgcache.Store[proto.Message] = (*Store[proto.Message])(nil)

func (Store[StoreMsg]) ModuleDependencies() []module.Dependency {
	return []module.Dependency{
		module.RequiredDependency(gaeemulation.ModuleName),
	}
}

func (Store[StoreMsg]) LoadMeta(ctx context.Context, pipelineID string) (map[config.Set]cfgcache.Meta, error) {
	ctx, err := info.Namespace(datastore.WithoutTransaction(ctx), "")
	if err != nil {
		return nil, err
	}

	ret := make(map[config.Set]cfgcache.Meta)
	encID := base64.RawURLEncoding.EncodeToString([]byte(pipelineID))
	q := datastore.NewQuery(cachedConfigKind).Project("Meta").Gt("Meta", encID+"|").Lt("Meta", encID+"}")
	for row, err := range datastore.RunQuery[*cachedConfig[StoreMsg]](ctx, q).Results {
		if err != nil {
			return nil, errors.Fmt("failed to query projected config hashes: %w", err)
		}
		ret[row.ID.ConfigSet] = row.Meta.Meta
	}
	return ret, nil
}

func (Store[StoreMsg]) Put(ctx context.Context, pipelineID string, cs config.Set, m cfgcache.Meta, out StoreMsg) error {
	ctx, err := info.Namespace(datastore.WithoutTransaction(ctx), "")
	if err != nil {
		return err
	}

	return datastore.Put(ctx, &cachedConfig[StoreMsg]{
		ID: cachedConfigIDProperty{
			PipelineID: pipelineID,
			ConfigSet:  cs,
		},
		Config: out,
		Meta: contentMetaProperty{
			PipelineID: pipelineID,
			Meta:       m,
		},
	})
}

func (Store[StoreMsg]) Delete(ctx context.Context, pipelineID string, sets []config.Set) error {
	if len(sets) == 0 {
		return nil
	}

	ctx, err := info.Namespace(datastore.WithoutTransaction(ctx), "")
	if err != nil {
		return err
	}

	toDelete := make([]*cachedConfig[StoreMsg], len(sets))
	for i, cs := range sets {
		toDelete[i] = &cachedConfig[StoreMsg]{ID: cachedConfigIDProperty{PipelineID: pipelineID, ConfigSet: cs}}
	}
	return datastore.Delete(ctx, toDelete)
}

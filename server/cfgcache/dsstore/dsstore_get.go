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
	"iter"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	cfgcachecommon "go.chromium.org/luci/server/cfgcache/common"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

func (Store[StoreMsg]) Get(ctx context.Context, pipelineID string, sets map[config.Set]*cfgcachepb.Meta) iter.Seq2[*cfgcachecommon.StoredConfig[StoreMsg], error] {
	ctx, err := info.Namespace(datastore.WithoutTransaction(ctx), "")
	if err != nil {
		return func(yield func(*cfgcachecommon.StoredConfig[StoreMsg], error) bool) {
			yield(nil, err)
		}
	}

	toFetch := make([]*cachedConfig[StoreMsg], 0, len(sets))
	for cs, meta := range sets {
		toFetch = append(toFetch, &cachedConfig[StoreMsg]{
			ID:   cachedConfigIDProperty{PipelineID: pipelineID, ConfigSet: cs},
			Meta: contentMetaProperty{pipelineID, meta},
		})
	}
	batches := batch(toFetch, defaultDatastoreStoreBatchSize)

	return func(yield func(*cfgcachecommon.StoredConfig[StoreMsg], error) bool) {
		for _, batch := range batches {
			err := datastore.Get(ctx, batch)
			var merr errors.MultiError
			if err != nil && !errors.As(err, &merr) {
				yield(nil, err) // catastrophic failure
				return
			}

			for i, fetched := range batch {
				if len(merr) > 0 && merr[i] != nil {
					if errors.Is(merr[i], datastore.ErrNoSuchEntity) {
						continue
					}
					yield(fetched.toStoredConfig(), merr[i]) // catastrophic failure
					return
				}
				if !yield(fetched.toStoredConfig(), nil) {
					return
				}
			}
		}
	}
}

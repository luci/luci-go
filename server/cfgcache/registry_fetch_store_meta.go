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
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/logging"
)

// fetchStoreMeta loads stored Meta from each pipeline's Store.
//
// Returns a map of pipelineID->config.Set->Meta.
func (r *Registry) fetchStoreMeta(ctx context.Context) (loadedStoreMeta, error) {
	var mu sync.Mutex
	ret := loadedStoreMeta{}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(8)

	for _, ic := range r.ingestConfigs {
		eg.Go(func() error {
			ctx := logging.SetField(ctx, "pipeline", ic.id)
			logging.Infof(ctx, "Requesting stored metadata.")
			have, err := ic.store.LoadMeta(ctx, ic.id)
			if err != nil {
				return err
			}
			if len(have) == 0 {
				logging.Infof(ctx, "No stored metadata.")
				return nil
			}
			mu.Lock()
			ret[ic.id] = have
			mu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}

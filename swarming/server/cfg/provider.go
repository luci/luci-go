// Copyright 2023 The LUCI Authors.
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

package cfg

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// Provider knows how to load preprocessed configs from the datastore and
// convert them into instances of Config.
//
// Provider is a long-living object that keeps a reference to the most recent
// config, periodically reloading it from the datastore.
type Provider struct {
	cur atomic.Value
}

// NewProvider initializes a Provider by fetching the initial copy of configs.
//
// If there are no configs stored in the datastore (happens when bootstrapping
// a new service), uses some default empty config.
func NewProvider(ctx context.Context) (*Provider, error) {
	cur, err := fetchFromDatastore(ctx, nil)
	if err != nil {
		return nil, err
	}
	p := &Provider{}
	p.cur.Store(cur)
	return p, nil
}

// Config returns an immutable snapshot of the most recent config.
//
// Note that multiple sequential calls to Config() may return different
// snapshots if the config changes between them. For that reason it is better to
// get the snapshot once at the beginning of an RPC handler, and use it
// throughout.
func (p *Provider) Config(ctx context.Context) *Config {
	return p.cur.Load().(*Config)
}

// RefreshPeriodically runs a loop that periodically refetches the config from
// the datastore.
func (p *Provider) RefreshPeriodically(ctx context.Context) {
	for {
		jitter := time.Duration(rand.Int63n(int64(10 * time.Second)))
		if r := <-clock.After(ctx, 30*time.Second+jitter); r.Err != nil {
			return // the context is canceled
		}
		if err := p.refresh(ctx); err != nil {
			// Don't log the error if the server is shutting down.
			if !errors.Is(err, context.Canceled) {
				logging.Warningf(ctx, "Failed to refresh local copy of the config: %s", err)
			}
		}
	}
}

// refresh fetches the new config from the datastore if it is available.
func (p *Provider) refresh(ctx context.Context) error {
	cur := p.cur.Load().(*Config)
	new, err := fetchFromDatastore(ctx, cur)
	if err != nil {
		return err
	}
	p.cur.Store(new)
	return nil
}

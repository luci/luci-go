// Copyright 2015 The LUCI Authors.
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

// Package memcache implements a caching config client backend backed by
// AppEngine's memcache service.
package memcache

import (
	"encoding/hex"
	"time"

	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/caching"

	mc "go.chromium.org/gae/service/memcache"

	"golang.org/x/net/context"
)

const (
	memCacheSchema  = "v1"
	maxMemCacheSize = 1024 * 1024 // 1MB
)

// Backend wraps a backend.B instance with a memcache-backed caching layer whose
// entries expire after exp.
func Backend(b backend.B, exp time.Duration) backend.B {
	return &caching.Backend{
		B: b,
		CacheGet: func(c context.Context, key caching.Key, l caching.Loader) (*caching.Value, error) {
			if key.Authority != backend.AsService {
				return l(c, key, nil)
			}

			// Is the item already cached?
			k := memcacheKey(&key)
			mci, err := mc.GetKey(c, k)
			switch err {
			case nil:
				// Value was cached, successfully retrieved.
				v, err := caching.DecodeValue(mci.Value())
				if err != nil {
					return nil, errors.Annotate(err, "failed to decode cache value from %q", k).Err()
				}
				return v, nil

			case mc.ErrCacheMiss:
				// Value was not cached. Load from Loader and cache.
				v, err := l(c, key, nil)
				if err != nil {
					return nil, err
				}

				// Attempt to cache the value. If this fails, we'll log a warning and
				// move on.
				err = func() error {
					d, err := v.Encode()
					if err != nil {
						return errors.Annotate(err, "failed to encode value").Err()
					}

					if len(d) > maxMemCacheSize {
						return errors.Reason("entry exceeds memcache size (%d > %d)", len(d), maxMemCacheSize).Err()
					}

					item := mc.NewItem(c, k).SetValue(d).SetExpiration(exp)
					if err := mc.Set(c, item); err != nil {
						return errors.Annotate(err, "").Err()
					}
					return nil
				}()
				if err != nil {
					log.Fields{
						log.ErrorKey: err,
						"key":        k,
					}.Warningf(c, "Failed to cache config.")
				}

				// Return the loaded value.
				return v, nil

			default:
				// Unknown memcache error.
				log.Fields{
					log.ErrorKey: err,
					"key":        k,
				}.Warningf(c, "Failed to decode memcached config.")
				return l(c, key, nil)
			}
		},
	}
}

func memcacheKey(key *caching.Key) string { return hex.EncodeToString(key.ParamHash()) }

// Copyright 2016 The LUCI Authors.
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

package caching

import (
	"strings"
	"time"

	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"

	"golang.org/x/net/context"
)

// ProcCache returns a backend.B that caches configuration service results in an
// in-memory proccache instance.
//
// This will only cache results for AsService calls; any other Authority will
// pass through.
func ProcCache(b backend.B, exp time.Duration) backend.B {
	return &Backend{
		B: b,
		CacheGet: func(c context.Context, key Key, l Loader) (*Value, error) {
			if key.Authority != backend.AsService {
				return l(c, key, nil)
			}

			k := mkProcCacheKey(&key)
			ret, err := proccache.GetOrMake(c, k, func() (interface{}, time.Duration, error) {
				v, err := l(c, key, nil)
				if err != nil {
					return nil, 0, err
				}
				return v, exp, nil
			})
			if err != nil {
				return nil, err
			}
			return ret.(*Value), nil
		},
	}
}

type procCacheKey string

func mkProcCacheKey(key *Key) procCacheKey {
	return procCacheKey(strings.Join([]string{
		key.Schema,
		string(key.Op),
		key.ConfigSet,
		key.Path,
		string(key.GetAllTarget),
	}, ":"))
}

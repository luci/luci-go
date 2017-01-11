// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

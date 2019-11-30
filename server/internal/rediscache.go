// Copyright 2019 The LUCI Authors.
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

package internal

import (
	"context"
	"time"

	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/redisconn"
)

// RedisBlobCache implements caching.BlobCache using Redis.
type RedisBlobCache struct {
	Prefix string // prefix to prepend to keys
}

var _ caching.BlobCache = (*RedisBlobCache)(nil)

func (rc *RedisBlobCache) key(k string) string { return rc.Prefix + k }

// Get returns a cached item or ErrCacheMiss if it's not in the cache.
func (rc *RedisBlobCache) Get(ctx context.Context, key string) ([]byte, error) {
	conn, err := redisconn.Get(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	blob, err := redis.Bytes(conn.Do("GET", rc.key(key)))
	if err == redis.ErrNil {
		return nil, caching.ErrCacheMiss
	}
	return blob, err
}

// Set unconditionally overwrites an item in the cache.
//
// If 'exp' is zero, the item will have no expiration time.
func (rc *RedisBlobCache) Set(ctx context.Context, key string, value []byte, exp time.Duration) error {
	conn, err := redisconn.Get(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if exp == 0 {
		_, err = conn.Do("SET", rc.key(key), value)
	} else {
		_, err = conn.Do("PSETEX", rc.key(key), exp.Nanoseconds()/1e6, value)
	}
	return err
}

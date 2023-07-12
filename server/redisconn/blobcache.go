// Copyright 2020 The LUCI Authors.
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

package redisconn

import (
	"context"
	"time"

	"github.com/gomodule/redigo/redis"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"go.chromium.org/luci/server/caching"
)

var tracer = otel.Tracer("go.chromium.org/luci/server/redisconn")

// redisBlobCache implements caching.BlobCache using Redis.
type redisBlobCache struct {
	Prefix string // prefix to prepend to keys
}

var _ caching.BlobCache = (*redisBlobCache)(nil)

func (rc *redisBlobCache) key(k string) string { return rc.Prefix + k }

// Get returns a cached item or ErrCacheMiss if it's not in the cache.
func (rc *redisBlobCache) Get(ctx context.Context, key string) (blob []byte, err error) {
	ctx, span := tracer.Start(ctx, "go.chromium.org/luci/server.RedisBlobCache.Get")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	conn, err := Get(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	blob, err = redis.Bytes(conn.Do("GET", rc.key(key)))
	if err == redis.ErrNil {
		return nil, caching.ErrCacheMiss
	}
	return blob, err
}

// Set unconditionally overwrites an item in the cache.
//
// If 'exp' is zero, the item will have no expiration time.
func (rc *redisBlobCache) Set(ctx context.Context, key string, value []byte, exp time.Duration) (err error) {
	ctx, span := tracer.Start(ctx, "go.chromium.org/luci/server.RedisBlobCache.Set")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	conn, err := Get(ctx)
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

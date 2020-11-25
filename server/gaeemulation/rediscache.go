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

package gaeemulation

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/gae/filter/dscache"
)

const (
	lockPrefix = "L" // items that hold locks start with this byte
	dataPrefix = "D" // items that hold data start with this byte

	maxLockItemLen = len(lockPrefix) + dscache.NonceBytes
)

// The script does "compare and swap".
//
// Arguments:
//  KEYS[1]: the key to operate on.
//  ARGV[1]: the old value to compare to (its first maxLockItemLen bytes).
//  ARGV[2]: the new value to write.
//  ARGV[3]: expiration time (in sec) of the new value.
var casScript = strings.TrimSpace(fmt.Sprintf(`
if redis.call("GETRANGE", KEYS[1], 0, %d) == ARGV[1] then
	return redis.call("SET", KEYS[1], ARGV[2], "EX", ARGV[3])
end
`, maxLockItemLen))

// casScriptSHA1 is SHA1 of `casScript`, to be used with EVALSHA.
var casScriptSHA1 string

func init() {
	h := sha1.New()
	io.WriteString(h, casScript)
	casScriptSHA1 = hex.EncodeToString(h.Sum(nil))
}

// redisCache implements dscache.Cache via Redis.
type redisCache struct {
	pool *redis.Pool
}

func (c redisCache) PutLocks(ctx context.Context, keys []string) (err error) {
	if len(keys) == 0 {
		return nil
	}

	ctx, ts := trace.StartSpan(ctx, "go.chromium.org/luci/server/redisCache.PutLocks")
	defer func() { ts.End(err) }()

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, key := range keys {
		conn.Send("SET", key, lockPrefix, "EX", dscache.LockTimeSeconds)
	}
	_, err = conn.Do("")
	return err
}

func (c redisCache) DropLocks(ctx context.Context, keys []string) (err error) {
	if len(keys) == 0 {
		return nil
	}

	ctx, ts := trace.StartSpan(ctx, "go.chromium.org/luci/server/redisCache.DropLocks")
	defer func() { ts.End(err) }()

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, key := range keys {
		conn.Send("DEL", key)
	}
	_, err = conn.Do("")
	return err
}

func (c redisCache) TryLockAndFetch(ctx context.Context, keys []string, nonce []byte) (items []dscache.CacheItem, err error) {
	if len(keys) == 0 {
		return nil, nil
	}

	ctx, ts := trace.StartSpan(ctx, "go.chromium.org/luci/server/redisCache.TryLockAndFetch")
	defer func() { ts.End(err) }()

	// Prepopulate the response with nil items which mean "cache miss". It is
	// always safe to return them, the dscache will fallback to using datastore
	// (without touching the cache in the end).
	items = make([]dscache.CacheItem, len(keys))

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return items, err
	}
	defer conn.Close()

	prefixedNonce := append([]byte(lockPrefix), nonce...)
	for _, key := range keys {
		if key == "" {
			continue
		}
		conn.Send("SET", key, prefixedNonce, "NX", "EX", dscache.LockTimeSeconds)
		conn.Send("GET", key)
	}
	conn.Flush()

	for i, key := range keys {
		if key == "" {
			continue
		}
		conn.Receive() // skip the result of "SET", we want "GET"
		if body, err := redis.Bytes(conn.Receive()); err == nil {
			items[i] = &cacheItem{key: key, body: body}
		}
		if conn.Err() != nil {
			err = conn.Err() // the connection dropped, can't fetch the rest
			break
		}
	}

	return items, err
}

func (c redisCache) CompareAndSwap(ctx context.Context, items []dscache.CacheItem) (err error) {
	if len(items) == 0 {
		return nil
	}

	ctx, ts := trace.StartSpan(ctx, "go.chromium.org/luci/server/redisCache.CompareAndSwap")
	defer func() { ts.End(err) }()

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Preload the script to make sure Redis knows about it. Redis guarantees that
	// scripts loaded in a connection survive at least until this connection is
	// dropped (in practice they survive until the server is restarted).
	conn.Send("SCRIPT", "LOAD", casScript)

	for _, item := range items {
		item := item.(*cacheItem)
		if item.lock == nil {
			panic("can CAS only promoted items")
		}
		conn.Send("EVALSHA", casScriptSHA1, 1,
			item.key,
			item.lock, // will be compared to what's in the cache right now
			item.body, // the new value if comparison succeeds
			int(item.exp.Seconds()),
		)
	}

	_, err = conn.Do("")
	return err
}

////////////////////////////////////////////////////////////////////////////////

type cacheItem struct {
	key  string
	body []byte

	lock []byte        // set to the previous value of `body` after the promotion
	exp  time.Duration // set after the promotion
}

func (ci *cacheItem) Key() string {
	return ci.key
}

func (ci *cacheItem) Nonce() []byte {
	if bytes.HasPrefix(ci.body, []byte(lockPrefix)) {
		return ci.body[len(lockPrefix):]
	}
	return nil
}

func (ci *cacheItem) Data() []byte {
	if bytes.HasPrefix(ci.body, []byte(dataPrefix)) {
		return ci.body[len(dataPrefix):]
	}
	return nil
}

func (ci *cacheItem) PromoteToData(data []byte, exp time.Duration) {
	// TODO(vadimsh): This allocation can probably be avoided by refactoring
	// dscache.CacheItem interface.
	ci.promote(append([]byte(dataPrefix), data...), exp)
}

func (ci *cacheItem) PromoteToIndefiniteLock() {
	ci.promote([]byte(lockPrefix), time.Hour*24*30)
}

func (ci *cacheItem) promote(body []byte, exp time.Duration) {
	if ci.lock != nil {
		panic("already promoted")
	}
	if !bytes.HasPrefix(ci.body, []byte(lockPrefix)) {
		panic("not a lock item")
	}
	ci.lock = ci.body
	if len(ci.lock) > maxLockItemLen {
		ci.lock = ci.lock[:maxLockItemLen]
	}
	ci.body = body
	ci.exp = exp
}

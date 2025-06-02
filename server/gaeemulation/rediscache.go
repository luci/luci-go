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
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/filter/dscache"
)

var tracer = otel.Tracer("go.chromium.org/luci/server/gaeemulation")

const (
	lockPrefix = 'L' // items that hold locks start with this byte
	dataPrefix = 'D' // items that hold data start with this byte

	// A prefix byte +  nonce.
	maxLockItemLen = 1 + dscache.NonceBytes
)

// To avoid allocations in Prefix() below.
var dataPrefixBuf = []byte{dataPrefix}

// The script does "compare prefix and swap".
//
// Arguments:
//
//	KEYS[1]: the key to operate on.
//	ARGV[1]: the old value to compare to (its first maxLockItemLen bytes).
//	ARGV[2]: the new value to write.
//	ARGV[3]: expiration time (in sec) of the new value.
var casScript = strings.TrimSpace(fmt.Sprintf(`
if redis.call("GETRANGE", KEYS[1], 0, %d) == ARGV[1] then
	return redis.call("SET", KEYS[1], ARGV[2], "EX", ARGV[3])
end
`, maxLockItemLen))

// casScriptSHA1 is SHA1 of `casScript`, to be used with EVALSHA to save on
// a round trip to redis per CAS.
var casScriptSHA1 string

func init() {
	dgst := sha1.Sum([]byte(casScript))
	casScriptSHA1 = hex.EncodeToString(dgst[:])
}

// redisCache implements dscache.Cache via Redis.
type redisCache struct {
	pool *redis.Pool
}

func (c redisCache) do(ctx context.Context, op string, cb func(conn redis.Conn) error) (err error) {
	ctx, span := tracer.Start(ctx, "go.chromium.org/luci/server/redisCache."+op)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		return errors.Fmt("dscache %s: %w", op, err)
	}
	defer conn.Close()

	if err = cb(conn); err != nil {
		return errors.Fmt("dscache %s: %w", op, err)
	}
	return nil
}

func (c redisCache) PutLocks(ctx context.Context, keys []string, timeout time.Duration) error {
	if len(keys) == 0 {
		return nil
	}
	return c.do(ctx, "PutLocks", func(conn redis.Conn) error {
		for _, key := range keys {
			conn.Send("SET", key, []byte{lockPrefix}, "EX", int(timeout.Seconds()))
		}
		_, err := conn.Do("")
		return err
	})
}

func (c redisCache) DropLocks(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.do(ctx, "DropLocks", func(conn redis.Conn) error {
		for _, key := range keys {
			conn.Send("DEL", key)
		}
		_, err := conn.Do("")
		return err
	})
}

func (c redisCache) TryLockAndFetch(ctx context.Context, keys []string, nonce []byte, timeout time.Duration) ([]dscache.CacheItem, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	// Prepopulate the response with nil items which mean "cache miss". It is
	// always safe to return them, the dscache will fallback to using datastore
	// (without touching the cache in the end).
	items := make([]dscache.CacheItem, len(keys))

	err := c.do(ctx, "TryLockAndFetch", func(conn redis.Conn) (err error) {
		// Send a pipeline of SET NX+GET pairs.
		prefixedNonce := append([]byte{lockPrefix}, nonce...)
		for _, key := range keys {
			if key == "" {
				continue
			}
			conn.Send("SET", key, prefixedNonce, "NX", "EX", int(timeout.Seconds()))
			conn.Send("GET", key)
		}
		conn.Flush()

		// Parse replies.
		for i, key := range keys {
			if key == "" {
				continue
			}
			conn.Receive() // skip the result of "SET", we want "GET"
			if body, err := redis.Bytes(conn.Receive()); err == nil {
				items[i] = &cacheItem{key: key, body: body}
			}
			if conn.Err() != nil {
				return conn.Err() // the connection is dropped, can't fetch the rest
			}
		}
		return nil
	})

	return items, err
}

func (c redisCache) CompareAndSwap(ctx context.Context, items []dscache.CacheItem) error {
	if len(items) == 0 {
		return nil
	}

	return c.do(ctx, "CompareAndSwap", func(conn redis.Conn) error {
		toSwap := make([]*cacheItem, len(items))
		for i, item := range items {
			item := item.(*cacheItem)
			if item.lock == nil {
				panic("dscache violated Cache contract: can CAS only promoted items")
			}
			toSwap[i] = item
		}

		for {
			// Pipeline all CAS operations at once.
			for _, item := range toSwap {
				_ = conn.Send("EVALSHA",
					casScriptSHA1, // the script to execute
					1,             // number of key-typed arguments (see casScript)
					item.key,      // the key to operate on
					item.lock,     // will be compared to what's in the cache right now
					item.body,     // the new value if comparison succeeds
					int(item.exp.Seconds()),
				)
			}

			// Flush and read results. Here `err` is a connection-level error and
			// `batchReply` is secretly an array of EVALSHA replies (some of which can
			// be redis.Error).
			batchReply, err := conn.Do("")
			if err != nil {
				return err
			}

			replies := batchReply.([]any)
			if len(replies) != len(toSwap) {
				panic(fmt.Sprintf("Redis protocol violation: %d != %d", len(replies), len(toSwap)))
			}

			// If we get a NOSCRIPT error, need to load the CAS script and redo
			// operations that failed. Any other error is non-recoverable.
			toRetry := toSwap[:0]
			for i, rep := range replies {
				if err, isErr := rep.(redis.Error); isErr {
					if strings.HasPrefix(err.Error(), "NOSCRIPT ") {
						toRetry = append(toRetry, toSwap[i])
					} else {
						return err
					}
				}
			}
			if len(toRetry) == 0 {
				return nil
			}
			toSwap = toRetry

			// Redis doesn't know about the script yet. Load it and retry EVALSHAs.
			// This should happen very rarely (in theory only after Redis server
			// restarts or full flushes).
			logging.Warningf(ctx, "Loading the CAS script into Redis")
			if _, err = conn.Do("SCRIPT", "LOAD", casScript); err != nil {
				return err
			}
		}
	})
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
	if len(ci.body) > 0 && ci.body[0] == lockPrefix {
		return ci.body[1:]
	}
	return nil
}

func (ci *cacheItem) Data() []byte {
	if len(ci.body) > 0 && ci.body[0] == dataPrefix {
		return ci.body[1:]
	}
	return nil
}

func (ci *cacheItem) Prefix() []byte {
	return dataPrefixBuf
}

func (ci *cacheItem) PromoteToData(data []byte, exp time.Duration) {
	if len(data) == 0 || data[0] != dataPrefix {
		panic("dscache violated CacheItem contract: data is not prefixed by Prefix()")
	}
	ci.promote(data, exp)
}

func (ci *cacheItem) PromoteToIndefiniteLock() {
	ci.promote([]byte{lockPrefix}, time.Hour*24*30)
}

func (ci *cacheItem) promote(body []byte, exp time.Duration) {
	if ci.lock != nil {
		panic("already promoted")
	}
	if len(ci.body) == 0 || ci.body[0] != lockPrefix {
		panic("not a lock item")
	}
	ci.lock = ci.body
	// Note: this should not normally happen, but may happen if some items were
	// written with different value of dscache.NonceBytes constant. We need to
	// trim it for the casScript that compares only up to maxLockItemLen bytes.
	if len(ci.lock) > maxLockItemLen {
		ci.lock = ci.lock[:maxLockItemLen]
	}
	ci.body = body
	ci.exp = exp
}

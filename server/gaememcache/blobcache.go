// Copyright 2025 The LUCI Authors.
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

package gaememcache

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/internal/gae"
	"go.chromium.org/luci/server/internal/gae/memcache"
)

// gaeBlobCache implements caching.BlobCache using GAE memcache.
type gaeBlobCache struct {
	namespace string
}

var _ caching.BlobCache = (*gaeBlobCache)(nil)

// Get returns a cached item or ErrCacheMiss if it's not in the cache.
func (rc *gaeBlobCache) Get(ctx context.Context, key string) (blob []byte, err error) {
	req := &memcache.MemcacheGetRequest{
		Key:       [][]byte{[]byte(key)},
		NameSpace: &rc.namespace,
	}

	res := &memcache.MemcacheGetResponse{}
	if err := gae.Call(ctx, "memcache", "Get", req, res); err != nil {
		return nil, err
	}

	switch len(res.Item) {
	case 0:
		return nil, caching.ErrCacheMiss
	case 1:
		item := res.Item[0]
		if string(item.Key) != key {
			return nil, errors.New("the API unexpectedly returned wrong item")
		}
		if item.GetIsDeleteLocked() {
			return nil, caching.ErrCacheMiss
		}
		return item.Value, nil
	default:
		return nil, errors.Fmt("the API unexpectedly returned %d items", len(res.Item))
	}
}

// Set unconditionally overwrites an item in the cache.
//
// If 'exp' is zero, the item will have no expiration time.
func (rc *gaeBlobCache) Set(ctx context.Context, key string, value []byte, exp time.Duration) (err error) {
	var expirationTime *uint32
	if exp != 0 {
		if exp < time.Second {
			// Expiration duration has seconds precision. If the item expires less
			// than a second from now (or we somehow got negative expiration here),
			// use an absolute timestamp in the past to make the item expire ASAP. If
			// we just use 0, the item will live forever.
			//
			// Note that memcache API recognizes "large" values of ExpirationTime as
			// absolute unix timestamps.
			expirationTime = proto.Uint32(uint32(time.Now().Unix()) - 5)
		} else {
			// Cap expiration by at most 1y. That way memcache API will always treat
			// this as a relative duration (not an absolute timestamp).
			expirationTime = proto.Uint32(uint32(min(exp, time.Hour*24*365) / time.Second))
		}
	}

	req := &memcache.MemcacheSetRequest{
		Item: []*memcache.MemcacheSetRequest_Item{
			{
				Key:            []byte(key),
				Value:          value,
				SetPolicy:      memcache.MemcacheSetRequest_SET.Enum(),
				ExpirationTime: expirationTime,
			},
		},
		NameSpace: &rc.namespace,
	}

	res := &memcache.MemcacheSetResponse{}
	if err := gae.Call(ctx, "memcache", "Set", req, res); err != nil {
		return err
	}

	switch {
	case len(res.SetStatus) != 1:
		return errors.Fmt("the API unexpectedly returned wrong number of statuses %d", len(res.SetStatus))
	case res.SetStatus[0] != memcache.MemcacheSetResponse_STORED:
		return errors.Fmt("failed to store the item: %s", res.SetStatus[0])
	default:
		return nil
	}
}

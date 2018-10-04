// Copyright 2017 The LUCI Authors.
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

package config

import (
	"context"
	"time"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/server/caching"

	"github.com/golang/protobuf/proto"
)

// messageCacheKey => proto.Message
var messageCache = caching.RegisterLRUCache(4096)

type messageCacheKey struct {
	cset config.Set
	path string
}

// MessageCache is an in-memory configuration cache. Unlike the "MessageCache" config
// Backend, this stores the unmarshaled configuration object in-memory.
type MessageCache struct {
	// Lifetime is the lifetime applied to an individual cache entry. If Lifetime
	// is <= 0, no caching will occur.
	Lifetime time.Duration
}

// Get returns an unmarshalled configuration service text protobuf message.
//
// If the message is not currently in the process cache, it will be fetched from
// the config service and cached prior to being returned.
func (mc *MessageCache) Get(c context.Context, cset config.Set, path string, msg proto.Message) (
	proto.Message, error) {

	// If no Lifetime is configured, bypass the cache layer.
	if mc.Lifetime <= 0 {
		err := mc.GetUncached(c, cset, path, msg)
		return msg, err
	}

	key := messageCacheKey{cset, path}

	// Load the value from our cache. First, though, take out a lock on this
	// specific config key. This will prevent multiple concurrent accesses from
	// slamming the config service, particularly at startup.
	var v interface{}
	var err error
	v, err = messageCache.LRU(c).GetOrCreate(c, key, func() (interface{}, time.Duration, error) {
		// Not in cache or expired. Reload...
		if err := mc.GetUncached(c, cset, path, msg); err != nil {
			return nil, 0, err
		}
		return proto.Clone(msg), mc.Lifetime, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(proto.Message), nil
}

// GetUncached returns an unmarshalled configuration service text protobuf
// message. This bypasses the cache, and, on success, does not cache the
// resulting value.
func (mc *MessageCache) GetUncached(c context.Context, cset config.Set, path string,
	msg proto.Message) error {

	return cfgclient.Get(c, cfgclient.AsService, cset, path, textproto.Message(msg), nil)
}

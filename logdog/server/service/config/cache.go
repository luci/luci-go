// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package config

import (
	"context"
	"time"

	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/common/sync/mutexpool"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"

	"github.com/golang/protobuf/proto"
)

type messageCacheKey struct {
	cset cfgtypes.ConfigSet
	path string
}

// MessageCache is an in-memory configuration cache. Unlike the "MessageCache" config
// Backend, this stores the unmarshaled configuration object in-memory.
type MessageCache struct {
	// Lifetime is the lifetime applied to an individual cache entry. If Lifetime
	// is <= 0, no caching will occur.
	Lifetime time.Duration

	mutexes mutexpool.P
}

// Get returns an unmarshalled configuration service text protobuf message.
//
// If the message is not currently in the process cache, it will be fetched from
// the config service and cached prior to being returned.
func (pc *MessageCache) Get(c context.Context, cset cfgtypes.ConfigSet, path string, msg proto.Message) (
	proto.Message, error) {

	// If no Lifetime is configured, bypass the cache layer.
	if pc.Lifetime <= 0 {
		err := pc.GetUncached(c, cset, path, msg)
		return msg, err
	}

	key := messageCacheKey{cset, path}

	// Load the value from our cache. First, though, take out a lock on this
	// specific config key. This will prevent multiple concurrent accesses from
	// slamming the config service, particularly at startup.
	var v interface{}
	var err error
	pc.mutexes.WithMutex(key, func() {
		v, err = proccache.GetOrMake(c, key, func() (interface{}, time.Duration, error) {
			// Not in cache or expired. Reload...
			if err := pc.GetUncached(c, cset, path, msg); err != nil {
				return nil, 0, err
			}
			return msg, pc.Lifetime, nil
		})
	})

	if err != nil {
		return nil, err
	}
	return v.(proto.Message), nil
}

// GetUncached returns an unmarshalled configuration service text protobuf
// message. This bypasses the cache, and, on success, does not cache the
// resulting value.
func (pc *MessageCache) GetUncached(c context.Context, cset cfgtypes.ConfigSet, path string,
	msg proto.Message) error {

	return cfgclient.Get(c, cfgclient.AsService, cset, path, textproto.Message(msg), nil)
}

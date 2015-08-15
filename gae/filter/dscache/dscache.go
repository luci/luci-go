// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dscache

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
)

var (
	// InstanceEnabledStatic allows you to statically (e.g. in an init() function)
	// bypass this filter by setting it to false. This takes effect when the
	// application calls IsGloballyEnabled.
	InstanceEnabledStatic = true

	// LockTimeSeconds is the number of seconds that a "lock" memcache entry will
	// have its expiration set to. It's set to just over half of the frontend
	// request handler timeout (currently 60 seconds).
	LockTimeSeconds = 31

	// CacheTimeSeconds is the default number of seconds that a cached entity will
	// be retained (memcache contention notwithstanding). A value of 0 is
	// infinite. This is #seconds instead of time.Duration, because memcache
	// truncates expiration to the second, which means a sub-second amount would
	// actually truncate to an infinite timeout.
	CacheTimeSeconds = int64((time.Hour * 24).Seconds())

	// CompressionThreshold is the number of bytes of entity value after which
	// compression kicks in.
	CompressionThreshold = 860

	// DefaultShards is the default number of key sharding to do.
	DefaultShards int = 1

	// DefaultEnable indicates whether or not caching is globally enabled or
	// disabled by default. Can still be overridden by CacheEnableMeta.
	DefaultEnabled = true
)

const (
	MemcacheVersion = "1"

	// KeyFormat is the format string used to generate memcache keys. It's
	//   gae:<version>:<shard#>:<base64_std_nopad(sha1(datastore.Key))>
	KeyFormat      = "gae:" + MemcacheVersion + ":%x:%s"
	Sha1B64Padding = 1
	Sha1B64Size    = 28 - Sha1B64Padding

	MaxShards          = 256
	MaxShardsLen       = len("ff")
	InternalGAEPadding = 96
	ValueSizeLimit     = (1000 * 1000) - InternalGAEPadding - MaxShardsLen

	CacheEnableMeta     = "dscache.enable"
	CacheExpirationMeta = "dscache.expiration"

	// NonceUint32s is the number of 32 bit uints to use in the 'lock' nonce.
	NonceUint32s = 2

	// GlobalEnabledCheckInterval is how frequently IsGloballyEnabled should check
	// the globalEnabled datastore entry.
	GlobalEnabledCheckInterval = 5 * time.Minute
)

// internalValueSizeLimit is a var for testing purposes.
var internalValueSizeLimit = ValueSizeLimit

type CompressionType byte

const (
	NoCompression CompressionType = iota
	ZlibCompression
)

func (c CompressionType) String() string {
	switch c {
	case NoCompression:
		return "NoCompression"
	case ZlibCompression:
		return "ZlibCompression"
	default:
		return fmt.Sprintf("UNKNOWN_CompressionType(%d)", c)
	}
}

// FlagValue is used to indicate if a memcache entry currently contains an
// item or a lock.
type FlagValue uint32

const (
	ItemUKNONWN FlagValue = iota
	ItemHasData
	ItemHasLock
)

func MakeMemcacheKey(shard int, k datastore.Key) string {
	return fmt.Sprintf(KeyFormat, shard, HashKey(k))
}

func HashKey(k datastore.Key) string {
	dgst := sha1.Sum(serialize.ToBytes(k))
	buf := bytes.Buffer{}
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	_, _ = enc.Write(dgst[:])
	enc.Close()
	return buf.String()[:buf.Len()-Sha1B64Padding]
}

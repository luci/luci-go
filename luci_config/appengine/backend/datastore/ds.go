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

package datastore

import (
	"time"

	"go.chromium.org/luci/appengine/datastoreCache"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/access"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/caching"

	"golang.org/x/net/context"
)

const (
	dsCacheSchema = "v1"

	// RPCDeadline is the deadline applied to config service RPCs.
	RPCDeadline = 10 * time.Minute
)

var dsHandlerKey = "go.chromium.org/luci/appengine/gaeconfig.dsHandlerKey"

func getCacheHandler(c context.Context) datastorecache.Handler {
	v, _ := c.Value(&dsHandlerKey).(datastorecache.Handler)
	return v
}

// Cache is our registered datastore cache, bound to our Config Handler. The
// generator function is used by the cache manager task to get a Handler
// instance during refresh.
var Cache = datastorecache.Cache{
	Name:                 "go.chromium.org/luci/appengine/gaeconfig",
	AccessUpdateInterval: 24 * time.Hour,
	PruneFactor:          4,
	Parallel:             16,
	HandlerFunc:          getCacheHandler,
}

// dsCacheBackend is an interface around datastoreCache.Cache functionality used
// by dsCacheFilter.
//
// We specialize this in testing to swap in other cache backends.
type dsCacheBackend interface {
	Get(c context.Context, key []byte) (datastorecache.Value, error)
}

// Config is a datastore cache configuration.
//
// Cache parameters can be extracted from a config instance.
type Config struct {
	// RefreshInterval is the cache entry refresh interval.
	RefreshInterval time.Duration
	// FailOpen, if true, means that a cache miss or failure should be passed
	// through to the underlying cache.
	FailOpen bool

	// LockerFunc returns the Locker intance to use.
	LockerFunc func(context.Context) datastorecache.Locker

	// userProjAccess is cache of the current user's project access lookups.
	userProjAccess map[string]bool
	// anonProjAccess is cache of the current user's project access lookups.
	anonProjAccess map[string]bool

	// cache, if nil, is the datastore cache. This can be set for testing.
	cache dsCacheBackend
}

// Backend wraps the specified Backend in a datastore-backed cache.
func (dc *Config) Backend(base backend.B) backend.B {
	return &caching.Backend{
		B:           base,
		FailOnError: !dc.FailOpen,
		CacheGet:    dc.cacheGet,
	}
}

// WithHandler installs a datastorecache.Handler into our Context. This is
// used during datastore cache's Refresh calls.
//
// The Handler binds the parameters and Loader to the resolution call. For
// service resolution, this will be the Loader that is provided by the caching
// layer. For cron refresh, this will be the generic Loader provided by
// CronLoader.
func (dc *Config) WithHandler(c context.Context, l caching.Loader, timeout time.Duration) context.Context {
	handler := dsCacheHandler{
		refreshInterval: dc.RefreshInterval,
		failOpen:        dc.FailOpen,
		lockerFunc:      dc.LockerFunc,
		loader:          l,
		loaderTimeout:   timeout,
	}
	return context.WithValue(c, &dsHandlerKey, &handler)
}

func (dc *Config) cacheGet(c context.Context, key caching.Key, l caching.Loader) (
	*caching.Value, error) {

	cache := dc.cache
	if cache == nil {
		cache = &Cache
	}

	// Modify our cache key to always refresh AsService (ACLs will be asserted on
	// load) and request full-content.
	origAuthority := key.Authority
	key.Authority = backend.AsService

	// If we can assert access based on the operation's target ConfigSet, do that
	// before actually performing the action.
	switch key.Op {
	case caching.OpGet:
		// We can deny access to this operation based solely on the requested
		// ConfigSet.
		if err := access.Check(c, origAuthority, cfgtypes.ConfigSet(key.ConfigSet)); err != nil {
			// Access check failed, simulate ErrNoSuchConfig.
			//
			// OpGet: Indicate no content via empty Items.
			return &caching.Value{}, nil
		}
	}

	// For content-fetching operations, always request full content.
	switch key.Op {
	case caching.OpGet, caching.OpGetAll:
		// Always ask for full content.
		key.Content = true
	}

	// Encode our caching key, and use this for our datastore cache key.
	//
	// This gets recoded in dsCacheHandler's "Refresh" to identify the cache
	// operation that is being performed.
	encKey, err := caching.Encode(&key)
	if err != nil {
		return nil, errors.Annotate(err, "failed to encode cache key").Err()
	}

	// Construct a cache handler.
	v, err := cache.Get(dc.WithHandler(c, l, 0), encKey)
	if err != nil {
		return nil, err
	}

	// Decode our response.
	if v.Schema != dsCacheSchema {
		return nil, errors.Reason("response schema (%q) doesn't match current (%q)",
			v.Schema, dsCacheSchema).Err()
	}

	cacheValue, err := caching.DecodeValue(v.Data)
	if err != nil {
		return nil, errors.Annotate(err, "failed to decode cached value").Err()
	}

	// Prune any responses that are not permitted for the supplied Authority.
	switch key.Op {
	case caching.OpGetAll:
		if len(cacheValue.Items) > 0 {
			// Shift over any elements that can't be accessed.
			ptr := 0
			for _, itm := range cacheValue.Items {
				if dc.accessConfigSet(c, origAuthority, itm.ConfigSet) {
					cacheValue.Items[ptr] = itm
					ptr++
				}
			}
			cacheValue.Items = cacheValue.Items[:ptr]
		}
	}

	return cacheValue, nil
}

func (dc *Config) accessConfigSet(c context.Context, a backend.Authority, configSet string) bool {
	var cacheMap *map[string]bool
	switch a {
	case backend.AsService:
		return true
	case backend.AsUser:
		cacheMap = &dc.userProjAccess
	default:
		cacheMap = &dc.anonProjAccess
	}

	// If we've already cached this project access, return the cached value.
	if v, ok := (*cacheMap)[configSet]; ok {
		return v
	}

	// Perform a soft access check.
	canAccess := false
	switch err := access.Check(c, a, cfgtypes.ConfigSet(configSet)); err {
	case nil:
		canAccess = true

	case access.ErrNoAccess:
		// No access.
		break
	case cfgclient.ErrNoConfig:
		log.Fields{
			"configSet": configSet,
		}.Debugf(c, "Checking access to project without a config.")
	default:
		log.Fields{
			log.ErrorKey: err,
			"configSet":  configSet,
		}.Warningf(c, "Error checking for project access.")
	}

	// Cache the result for future lookups.
	if *cacheMap == nil {
		*cacheMap = make(map[string]bool)
	}
	(*cacheMap)[configSet] = canAccess
	return canAccess
}

type dsCacheValue struct {
	// Key is the cache Key.
	Key caching.Key `json:"k"`
	// Value is the cache Value.
	Value *caching.Value `json:"v"`
}

type dsCacheHandler struct {
	failOpen        bool
	refreshInterval time.Duration
	loader          caching.Loader
	lockerFunc      func(context.Context) datastorecache.Locker

	// loaderTimeout, if >0, will be applied prior to performing the loader
	// operation. This is used for cron operations.
	loaderTimeout time.Duration
}

func (dch *dsCacheHandler) FailOpen() bool                       { return dch.failOpen }
func (dch *dsCacheHandler) RefreshInterval([]byte) time.Duration { return dch.refreshInterval }
func (dch *dsCacheHandler) Refresh(c context.Context, key []byte, v datastorecache.Value) (datastorecache.Value, error) {
	// Decode the key into our caching key.
	var ck caching.Key
	if err := caching.Decode(key, &ck); err != nil {
		return v, errors.Annotate(err, "failed to decode cache key").Err()
	}

	var cv *caching.Value
	if v.Schema == dsCacheSchema && len(v.Data) > 0 {
		// We have a currently-cached value, so decode it into "cv".
		var err error
		if cv, err = caching.DecodeValue(v.Data); err != nil {
			return v, errors.Annotate(err, "failed to decode cache value").Err()
		}
	}

	// Apply our timeout, if configured (influences urlfetch).
	if dch.loaderTimeout > 0 {
		var cancelFunc context.CancelFunc
		c, cancelFunc = clock.WithTimeout(c, dch.loaderTimeout)
		defer cancelFunc()
	}

	// Perform a cache load on this value.
	cv, err := dch.loader(c, ck, cv)
	if err != nil {
		return v, errors.Annotate(err, "failed to load cache value").Err()
	}

	keyDesc := ck.String()
	valueDesc := cv.Description()
	log.Infof(c, "Loaded [%s]: %s", keyDesc, valueDesc)

	// Encode the resulting cache value.
	if v.Data, err = cv.Encode(); err != nil {
		return v, errors.Annotate(err, "failed to encode cache value").Err()
	}
	v.Schema = dsCacheSchema
	v.Description = keyDesc + ": " + valueDesc
	return v, nil
}

func (dch *dsCacheHandler) Locker(c context.Context) datastorecache.Locker {
	if dch.lockerFunc != nil {
		return dch.lockerFunc(c)
	}
	return nil
}

// CronLoader returns a caching.Loader implementation to be used
// by the Cron task.
func CronLoader(b backend.B) caching.Loader {
	return func(c context.Context, k caching.Key, v *caching.Value) (*caching.Value, error) {
		return caching.CacheLoad(c, b, k, v)
	}
}

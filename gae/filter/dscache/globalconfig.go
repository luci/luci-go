// Copyright 2015 The LUCI Authors.
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

package dscache

import (
	"sync"
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	mc "go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/clock"

	"golang.org/x/net/context"
)

// GlobalConfig is the entity definition for dscache's global configuration.
//
// It's Enable field can be set to false to cause all dscache operations
// (read and write) to cease in a given application.
//
// This should be manipulated in the GLOBAL (e.g. empty) namespace only. When
// written there, it affects activity in all namespaces.
type GlobalConfig struct {
	_id   int64  `gae:"$id,1"`
	_kind string `gae:"$kind,dscache"`

	Enable bool
}

var (
	globalEnabledLock = sync.RWMutex{}

	// globalEnabled is whether or not memcache has been globally enabled. It is
	// populated by IsGloballyEnabled when SetGlobalEnable has been set to
	// true.
	globalEnabled = true

	// globalEnabledNextCheck is IsGloballyEnabled's last successful check of the
	// global disable key.
	globalEnabledNextCheck = time.Time{}
)

// IsGloballyEnabled checks to see if this filter is enabled globally.
//
// This checks InstanceEnabledStatic, as well as polls the datastore entity
//   /dscache,1 (a GlobalConfig instance)
// Once every GlobalEnabledCheckInterval.
//
// For correctness, any error encountered returns true. If this assumed false,
// then Put operations might incorrectly invalidate the cache.
func IsGloballyEnabled(c context.Context) bool {
	if !InstanceEnabledStatic {
		return false
	}

	now := clock.Now(c)

	globalEnabledLock.RLock()
	nextCheck := globalEnabledNextCheck
	enabledVal := globalEnabled
	globalEnabledLock.RUnlock()

	if now.Before(nextCheck) {
		return enabledVal
	}

	globalEnabledLock.Lock()
	defer globalEnabledLock.Unlock()
	// just in case we raced
	if now.Before(globalEnabledNextCheck) {
		return globalEnabled
	}

	// always go to the default namespace
	c, err := info.Namespace(c, "")
	if err != nil {
		return true
	}
	cfg := &GlobalConfig{Enable: true}
	if err := ds.Get(c, cfg); err != nil && err != ds.ErrNoSuchEntity {
		return true
	}
	globalEnabled = cfg.Enable
	globalEnabledNextCheck = now.Add(GlobalEnabledCheckInterval)
	return globalEnabled
}

// SetGlobalEnable is a convenience function for manipulating the GlobalConfig.
//
// It's meant to be called from admin handlers on your app to turn dscache
// functionality on or off in emergencies.
func SetGlobalEnable(c context.Context, memcacheEnabled bool) error {
	// always go to the default namespace
	c, err := info.Namespace(c, "")
	if err != nil {
		return err
	}
	return ds.RunInTransaction(c, func(c context.Context) error {
		cfg := &GlobalConfig{Enable: true}
		if err := ds.Get(c, cfg); err != nil && err != ds.ErrNoSuchEntity {
			return err
		}
		if cfg.Enable == memcacheEnabled {
			return nil
		}
		cfg.Enable = memcacheEnabled
		if memcacheEnabled {
			// when going false -> true, wipe memcache.
			if err := mc.Flush(c); err != nil {
				return err
			}
		}
		return ds.Put(c, cfg)
	}, nil)
}

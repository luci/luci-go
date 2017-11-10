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

// Package settings implements storage for infrequently changing global
// settings.
//
// Settings are represented as (key, value) pairs, where value is JSON
// serializable struct. Settings are cached internally in the process memory to
// avoid hitting the storage all the time.
package settings

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

var (
	// ErrNoSettings can be returned by Get and Set on fatal errors.
	ErrNoSettings = errors.New("settings: settings are not available")

	// ErrBadType is returned if Get(...) receives unexpected type.
	ErrBadType = errors.New("settings: bad type")
)

// Bundle contains all latest settings.
type Bundle struct {
	Values map[string]*json.RawMessage // immutable

	lock     sync.RWMutex           // protects 'unpacked'
	unpacked map[string]interface{} // deserialized RawMessages
}

// get deserializes value for given key.
func (b *Bundle) get(key string, value interface{}) error {
	raw, ok := b.Values[key]
	if !ok || raw == nil || len(*raw) == 0 {
		return ErrNoSettings
	}

	typ := reflect.TypeOf(value)

	// Fast path for already-in-cache values.
	b.lock.RLock()
	cached, ok := b.unpacked[key]
	b.lock.RUnlock()

	// Slow path.
	if !ok {
		b.lock.Lock()
		defer b.lock.Unlock() // its fine to hold the lock until return

		cached, ok = b.unpacked[key]
		if !ok {
			// 'value' must be a pointer to a struct.
			if typ.Kind() != reflect.Ptr || typ.Elem().Kind() != reflect.Struct {
				return ErrBadType
			}
			// 'cached' is &Struct{}.
			cached = reflect.New(typ.Elem()).Interface()
			if err := json.Unmarshal([]byte(*raw), cached); err != nil {
				return err
			}
			if b.unpacked == nil {
				b.unpacked = make(map[string]interface{}, 1)
			}
			b.unpacked[key] = cached
		}
	}

	// Note: the code below may be called with b.lock in locked or unlocked state.

	// All calls to 'get' must use same type consistently.
	if reflect.TypeOf(cached) != typ {
		return ErrBadType
	}

	// 'value' and 'cached' are &Struct{}, Do *value = *cached.
	reflect.ValueOf(value).Elem().Set(reflect.ValueOf(cached).Elem())
	return nil
}

// Storage knows how to fetch settings from permanent storage and mutate
// them there. Methods of Storage can be called concurrently.
type Storage interface {
	// FetchAllSettings fetches all latest settings at once.
	//
	// Returns the settings and the duration they should be cached for by default.
	FetchAllSettings(c context.Context) (*Bundle, time.Duration, error)

	// UpdateSetting updates a setting at the given key.
	UpdateSetting(c context.Context, key string, value json.RawMessage, who, why string) error
}

// EventualConsistentStorage is Storage where settings changes take effect not
// immediately but by some predefined moment in the future.
type EventualConsistentStorage interface {
	Storage

	// GetConsistencyTime returns "last modification time" + "expiration period".
	//
	// It indicates moment in time when last setting change is fully propagated to
	// all instances.
	//
	// Returns zero time if there are no settings stored.
	GetConsistencyTime(c context.Context) (time.Time, error)
}

// Settings represent process global cache of all settings. Exact same instance
// of Settings should be injected into the context used by request handlers.
type Settings struct {
	storage Storage       // used to load and save settings
	values  lazyslot.Slot // cached settings
}

// New creates new Settings object that uses given Storage to fetch and save
// settings.
func New(storage Storage) *Settings {
	return &Settings{storage: storage}
}

// GetStorage returns underlying Storage instance.
func (s *Settings) GetStorage() Storage {
	return s.storage
}

// Get returns setting value (possibly cached) for the given key.
//
// It will be deserialized into the supplied value. Caller is responsible
// to pass correct type and pass same type to all calls. If the setting is not
// set returns ErrNoSettings.
func (s *Settings) Get(c context.Context, key string, value interface{}) error {
	bundle, err := s.values.Get(c, func(interface{}) (interface{}, time.Duration, error) {
		c, done := clock.WithTimeout(c, 15*time.Second) // retry for 15 sec total
		defer done()
		var bundle *Bundle
		var exp time.Duration
		err := retry.Retry(c, transient.Only(retry.Default), func() (err error) {
			c, done := clock.WithTimeout(c, 2*time.Second) // trigger a retry after 2 sec RPC timeout
			defer done()
			bundle, exp, err = s.storage.FetchAllSettings(c)
			return
		}, nil)
		if err != nil {
			return nil, 0, err
		}
		// Meaning of 0 in FetchAllSettings is different from how lazyslot
		// understands it.
		if exp == 0 {
			exp = lazyslot.ExpiresImmediately
		}
		return bundle, exp, nil
	})
	if err != nil {
		return err
	}
	return bundle.(*Bundle).get(key, value)
}

// GetUncached is like Get, by always fetches settings from the storage.
//
// Do not use GetUncached in performance critical parts, it is much heavier than
// Get.
func (s *Settings) GetUncached(c context.Context, key string, value interface{}) error {
	bundle, _, err := s.storage.FetchAllSettings(c)
	if err != nil {
		return err
	}
	return bundle.get(key, value)
}

// Set changes a setting value for the given key.
//
// New settings will apply only when existing in-memory cache expires.
// In particular, Get() right after Set() may still return old value.
func (s *Settings) Set(c context.Context, key string, value interface{}, who, why string) error {
	blob, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.storage.UpdateSetting(c, key, json.RawMessage(blob), who, why)
}

// SetIfChanged is like Set, but fetches an existing value and compares it to
// a new one before changing it.
//
// Avoids generating new revisions of settings if no changes are actually
// made. Also logs who is making the change.
func (s *Settings) SetIfChanged(c context.Context, key string, value interface{}, who, why string) error {
	// 'value' must be a pointer to a struct. Construct a zero value of this
	// kind of struct.
	typ := reflect.TypeOf(value)
	if typ.Kind() != reflect.Ptr || typ.Elem().Kind() != reflect.Struct {
		return ErrBadType
	}
	existing := reflect.New(typ.Elem()).Interface()

	// Fetch existing settings and compare.
	switch err := s.GetUncached(c, key, existing); {
	case err != nil && err != ErrNoSettings:
		return err
	case reflect.DeepEqual(existing, value):
		return nil
	}

	// Log the change. Ignore JSON marshaling errors, they are not essential
	// (and must not happen anyway).
	existingJSON, _ := json.Marshal(existing)
	modifiedJSON, _ := json.Marshal(value)
	logging.Warningf(c, "Settings %q changed from %s to %s by %q", key, existingJSON, modifiedJSON, who)

	return s.Set(c, key, value, who, why)
}

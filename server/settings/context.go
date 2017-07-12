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

package settings

import (
	"golang.org/x/net/context"
)

type contextKey int

// Use injects Settings into the context to be used by Get and Set.
func Use(c context.Context, s *Settings) context.Context {
	return context.WithValue(c, contextKey(0), s)
}

// GetSettings grabs Settings from the context if it's there.
func GetSettings(c context.Context) *Settings {
	if s, ok := c.Value(contextKey(0)).(*Settings); ok && s != nil {
		return s
	}
	return nil
}

// Get returns setting value (possibly cached) for the given key.
//
// It will be deserialized into the supplied value. Caller is responsible to
// pass correct type here. If the setting is not set or the context doesn't have
// settings implementation in it, returns ErrNoSettings.
func Get(c context.Context, key string, value interface{}) error {
	if s := GetSettings(c); s != nil {
		return s.Get(c, key, value)
	}
	return ErrNoSettings
}

// GetUncached is like Get, by always fetches settings from the storage.
//
// Do not use GetUncached in performance critical parts, it is much heavier than
// Get.
func GetUncached(c context.Context, key string, value interface{}) error {
	if s := GetSettings(c); s != nil {
		return s.GetUncached(c, key, value)
	}
	return ErrNoSettings
}

// Set overwrites a setting value for the given key.
//
// New settings will apply only when existing in-memory cache expires.
// In particular, Get() right after Set() may still return old value.
//
// Returns ErrNoSettings if context doesn't have Settings implementation.
func Set(c context.Context, key string, value interface{}, who, why string) error {
	if s := GetSettings(c); s != nil {
		return s.Set(c, key, value, who, why)
	}
	return ErrNoSettings
}

// SetIfChanged is like Set, but fetches an existing value and compares it to
// a new one before changing it.
//
// Avoids generating new revisions of settings if no changes are actually
// made. Also logs who is making the change.
//
// Returns ErrNoSettings if context doesn't have Settings implementation.
func SetIfChanged(c context.Context, key string, value interface{}, who, why string) error {
	if s := GetSettings(c); s != nil {
		return s.SetIfChanged(c, key, value, who, why)
	}
	return ErrNoSettings
}

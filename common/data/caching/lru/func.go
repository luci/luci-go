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

package lru

import (
	"time"

	"golang.org/x/net/context"
)

// Reference is a reference to a specific Cache instance.
//
// It is invoked with a Context, and must return a valid Cache.
type Reference func(context.Context) *Cache

// Value is a cached value generator. It can be created using Cached.
//
// A Value, when invoked, will return the current cached value. If no value is
// cached, the associated ValueGenerator will be invoked while holding a
// key-specific lock, and the ValueGenerator's response will be returned.
//
// If the ValueGenerator returns a nil error, its value will be cached.
type Value func(context.Context) (interface{}, error)

// ValueGenerator is used by Cached to make a new item, if the previous one has
// expired. It returns a value to put in the cache, along with its expiration
// duration (or <0 if it doesn't expire).
//
// If an error is returned, no value will be cached and the error will be
// returned from its Value.
type ValueGenerator func(context.Context) (interface{}, time.Duration, error)

// Cached returns a Value function that extracts an item from the cache (if it
// is there) or calls a supplied ValueGenerator to initialize and put it there.
//
// Intended to be used like a decorator for top level functions. The Value
// returns a error only if cache initialization callback returns an error.
//
// The ValueGenerator callback is protected by a lock around the specific cache
// key, so multiple requests for a Value will serialize.
func Cached(ref Reference, key interface{}, gen ValueGenerator) Value {
	return func(ctx context.Context) (interface{}, error) {
		return ref(ctx).GetOrCreate(ctx, key, func() (interface{}, time.Duration, error) {
			return gen(ctx)
		})
	}
}

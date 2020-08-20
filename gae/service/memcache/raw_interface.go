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

package memcache

// RawCB is a simple error callback for most of the methods in RawInterface.
type RawCB func(error)

// RawItemCB is the callback for RawInterface.GetMulti. It takes the retrieved
// item and the error for that item (e.g. ErrCacheMiss) if there was one. Item
// is guaranteed to be nil if error is not nil.
type RawItemCB func(Item, error)

// RawInterface is the full interface to the memcache service.
type RawInterface interface {
	NewItem(key string) Item

	AddMulti(items []Item, cb RawCB) error
	SetMulti(items []Item, cb RawCB) error
	GetMulti(keys []string, cb RawItemCB) error
	DeleteMulti(keys []string, cb RawCB) error
	CompareAndSwapMulti(items []Item, cb RawCB) error

	Increment(key string, delta int64, initialValue *uint64) (newValue uint64, err error)

	Flush() error

	Stats() (*Statistics, error)
}

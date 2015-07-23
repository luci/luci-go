// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memcache

// Interface is the full interface to the memcache service.
type Interface interface {
	NewItem(key string) Item

	Add(item Item) error
	Set(item Item) error
	Get(key string) (Item, error)
	Delete(key string) error
	CompareAndSwap(item Item) error

	AddMulti(items []Item) error
	SetMulti(items []Item) error
	GetMulti(keys []string) (map[string]Item, error)
	DeleteMulti(keys []string) error
	CompareAndSwapMulti(items []Item) error

	Increment(key string, delta int64, initialValue uint64) (newValue uint64, err error)
	IncrementExisting(key string, delta int64) (newValue uint64, err error)

	Flush() error

	Stats() (*Statistics, error)
}

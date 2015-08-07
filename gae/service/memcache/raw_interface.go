// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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

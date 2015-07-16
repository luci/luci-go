// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

type key int

var (
	globalInfoKey         key
	globalInfoFilterKey   key = 1
	rawDatastoreKey       key = 2
	rawDatastoreFilterKey key = 3
	memcacheKey           key = 4
	memcacheFilterKey     key = 5
	taskQueueKey          key = 6
	taskQueueFilterKey    key = 7

	mathRandKey key = 8
)

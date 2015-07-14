// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

type key int

var (
	globalInfoKey key

	datastoreKey key = 1
	memcacheKey  key = 2
	taskQueueKey key = 3

	timeNowKey  key = 4
	mathRandKey key = 5
)

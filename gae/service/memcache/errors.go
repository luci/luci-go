// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file contains errors which are mirrors/duplicates of the
// upstream SDK errors. This exists so that users can depend solely on this
// wrapper library without also needing to import the SDK.

package memcache

import (
	orig "google.golang.org/appengine/memcache"
)

// These are pass-through versions from the managed-VM SDK. All implementations
// must return these (exact) errors (not just an error with the same text).
var (
	ErrCacheMiss   = orig.ErrCacheMiss
	ErrCASConflict = orig.ErrCASConflict
	ErrNoStats     = orig.ErrNoStats
	ErrNotStored   = orig.ErrNotStored
	ErrServerError = orig.ErrServerError
)

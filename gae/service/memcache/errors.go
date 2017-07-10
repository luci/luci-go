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

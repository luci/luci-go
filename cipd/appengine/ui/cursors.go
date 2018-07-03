// Copyright 2018 The LUCI Authors.
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

package ui

import (
	"crypto/sha256"
	"encoding/base64"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/logging"
)

type cursorKind string

const instancesListing cursorKind = "v1:instances"

// cursorKey is a memcache key for items that link cursor to a previous page.
func (k cursorKind) cursorKey(pkg, cursor string) string {
	blob := sha256.Sum224([]byte(pkg + ":" + cursor))
	return string(k) + ":" + base64.RawStdEncoding.EncodeToString(blob[:])
}

// storePrevCursor stores mapping cursor => prev, so that prev can be fetched
// later by fetchPrevCursor.
//
// Logs and ignores errors. Cursor mapping is non-essential functionality.
func (k cursorKind) storePrevCursor(c context.Context, pkg, cursor, prev string) {
	itm := memcache.NewItem(c, k.cursorKey(pkg, cursor))
	itm.SetValue([]byte(prev))
	itm.SetExpiration(24 * time.Hour)
	if err := memcache.Set(c, itm); err != nil {
		logging.WithError(err).Errorf(c, "Failed to store prev cursor %q in memcache", k)
	}
}

// fetchPrevCursor returns a cursor stored by storePrevCursor.
//
// Logs and ignores errors. Cursor mapping is non-essential functionality.
func (k cursorKind) fetchPrevCursor(c context.Context, pkg, cursor string) string {
	itm, err := memcache.GetKey(c, k.cursorKey(pkg, cursor))
	if err != nil {
		if err != memcache.ErrCacheMiss {
			logging.WithError(err).Errorf(c, "Failed to fetch prev cursor %q from memcache", k)
		}
		return ""
	}
	return string(itm.Value())
}

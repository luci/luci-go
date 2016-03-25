// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"time"

	"github.com/luci/gae/service/datastore"
)

// AttemptResult holds the raw, compressed json blob returned from the
// execution.
type AttemptResult struct {
	_id     int64          `gae:"$id,1"`
	Attempt *datastore.Key `gae:"$parent"`

	Data string `gae:",noindex"`

	// These are denormalized across Attempt and AttemptResult
	Expiration time.Time
	Size       uint32
}

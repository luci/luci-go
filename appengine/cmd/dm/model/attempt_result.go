// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
)

// AttemptResult holds the raw, compressed json blob returned from the
// execution.
type AttemptResult struct {
	_id     int64          `gae:"$id,1"`
	Attempt *datastore.Key `gae:"$parent"`

	Data []byte `gae:",noindex"`
}

// ToDisplay returns a display.AttemptResult for this AttemptResult.
func (ar *AttemptResult) ToDisplay() *display.AttemptResult {
	return &display.AttemptResult{
		ID:   *types.NewAttemptID(ar.Attempt.StringID()),
		Data: string(ar.Data),
	}
}

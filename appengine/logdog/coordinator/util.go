// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"time"
)

// normalizeTime rounds a time.Time to the precision that datastore is capable
// of storing.
//
// We round the time to the nearest Microsecond b/c that's all GAE is able
// to store. This enforces consistency regardless of whether the time is
// user-supplied (registration) or loaded from datastore (subsequent updates).
func normalizeTime(t time.Time) time.Time {
	return t.Round(time.Microsecond)
}

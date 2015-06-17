// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package replication

import (
	"testing"
)

func TestTimeConvertShouldWork(t *testing.T) {
	expected := int64(123)
	tm := timestampToTime(expected)
	actual := timeToTimestamp(tm)
	if actual != expected {
		t.Errorf("timeToTimestamp(timestampToTime(%q) = %q; want %q", expected, actual, expected)
	}
}

// TODO(yyanagisawa): use test double to test protoToAuthDBSnapshot.

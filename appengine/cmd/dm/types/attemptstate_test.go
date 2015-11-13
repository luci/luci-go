// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAttemptState(t *testing.T) {
	t.Parallel()

	Convey("AttemptState string", t, func() {
		So(UNKNOWN.String(), ShouldEqual, "UNKNOWN")
		So(AttemptState(127).String(), ShouldEqual, "AttemptState(127)")
	})
}

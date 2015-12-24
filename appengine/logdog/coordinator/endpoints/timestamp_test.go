// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package endpoints

import (
	"testing"

	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetConfig(t *testing.T) {
	t.Parallel()

	Convey(`Can convert a time to RFC3339.`, t, func() {
		s := ToRFC3339(testclock.TestTimeUTC)
		So(s, ShouldEqual, "0001-02-03T04:05:06.000000007Z")

		Convey(`And can convert back.`, func() {
			t, err := ParseRFC3339(s)
			So(err, ShouldBeNil)
			So(t, ShouldResembleV, testclock.TestTimeUTC)
		})
	})
}

// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestError(t *testing.T) {
	t.Parallel()

	Convey("withStatus", t, func() {
		Convey("withStatus(nil, *) returns nil", func() {
			So(withStatus(nil, http.StatusBadRequest), ShouldBeNil)
		})
	})
}

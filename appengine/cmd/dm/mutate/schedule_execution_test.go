// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestScheduleExecution(t *testing.T) {
	t.Parallel()

	Convey("ScheduleExecution", t, func() {
		c := memory.Use(context.Background())
		se := &ScheduleExecution{types.NewAttemptID("quest|fffffffe")}

		Convey("Root", func() {
			So(se.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {
			muts, err := se.RollForward(c)
			So(err, ShouldBeNil)
			So(muts, ShouldBeNil)
		})
	})
}

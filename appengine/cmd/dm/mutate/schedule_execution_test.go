// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mutate

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestScheduleExecution(t *testing.T) {
	t.Parallel()

	Convey("ScheduleExecution", t, func() {
		c := memory.Use(context.Background())
		se := &ScheduleExecution{dm.NewAttemptID("quest", 1)}

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

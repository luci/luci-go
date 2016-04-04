// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStandardMetrics(t *testing.T) {
	Convey("Default version", t, func() {
		c, _ := buildGAETestContext()
		tsmon.GetState(c).S = store.NewInMemory(&target.Task{ServiceName: proto.String("default target")})
		standardMetricsCallback(c)
		tsmon.Flush(c)

		monitor := tsmon.GetState(c).M.(*monitor.Fake)
		So(len(monitor.Cells), ShouldEqual, 1)
		So(monitor.Cells[0][0].Name, ShouldEqual, "appengine/default_version")
		So(monitor.Cells[0][0].Value, ShouldEqual, "testVersion1")
	})
}

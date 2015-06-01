// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memlogger

import (
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/luci/luci-go/common/logging"
)

func TestLogger(t *testing.T) {
	Convey("logger", t, func() {
		c := Use(context.Background())
		l := logging.Get(c)
		So(l, ShouldNotBeNil)
		l.Debugf("test %s", LogDebug)
		l.Infof("test %s", LogInfo)
		l.Warningf("test %s", LogWarn)
		l.Errorf("test %s", LogError)
		l.Errorf("test WAT: %s", LogLevel(9001))
		ml := l.(*MemLogger)
		mld := ml.data

		So(len(*mld), ShouldEqual, 5)
		So((*mld)[0], ShouldResemble, LogEntry{LogDebug, "test DBG", nil})
		So((*mld)[1], ShouldResemble, LogEntry{LogInfo, "test IFO", nil})
		So((*mld)[2], ShouldResemble, LogEntry{LogWarn, "test WRN", nil})
		So((*mld)[3], ShouldResemble, LogEntry{LogError, "test ERR", nil})
		So((*mld)[4], ShouldResemble, LogEntry{LogError, "test WAT: ???", nil})
	})

	Convey("logger context", t, func() {
		c := Use(context.Background())
		l := logging.Get(c)
		So(l, ShouldNotBeNil)
		ml := l.(*MemLogger)

		l.Infof("totally works: %s", "yes")

		So(len(*ml.data), ShouldEqual, 1)
		So((*ml.data)[0], ShouldResemble, LogEntry{LogInfo, "totally works: yes", nil})
	})

	Convey("field data", t, func() {
		c := Use(context.Background())
		data := map[string]interface{}{
			"trombone": 50,
			"cat":      "amazing",
		}
		c = logging.SetFields(c, logging.NewFields(data))
		l := logging.Get(c)
		ml := l.(*MemLogger)

		l.Infof("Some unsuspecting log")
		So((*ml.data)[0].Data["trombone"], ShouldEqual, 50)
		So((*ml.data)[0].Data["cat"], ShouldEqual, "amazing")
	})
}

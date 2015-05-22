// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memlogger

import (
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"infra/libs/logging"
)

func TestLogger(t *testing.T) {
	Convey("logger", t, func() {
		c := Use(context.Background())
		l := logging.Get(c)
		So(l, ShouldNotBeNil)
		l.Infof("test %s", LogInfo)
		l.Warningf("test %s", LogWarn)
		l.Errorf("test %s", LogError)
		l.Errorf("test WAT: %s", LogLevel(9001))
		ml := l.(*MemLogger)

		So(len(*ml), ShouldEqual, 4)
		So((*ml)[0], ShouldResemble, LogEntry{LogInfo, "test IFO"})
		So((*ml)[1], ShouldResemble, LogEntry{LogWarn, "test WRN"})
		So((*ml)[2], ShouldResemble, LogEntry{LogError, "test ERR"})
		So((*ml)[3], ShouldResemble, LogEntry{LogError, "test WAT: ???"})
	})

	Convey("logger context", t, func() {
		c := Use(context.Background())
		l := logging.Get(c)
		So(l, ShouldNotBeNil)
		ml := l.(*MemLogger)

		l.Infof("totally works: %s", "yes")

		So(len(*ml), ShouldEqual, 1)
		So((*ml)[0], ShouldResemble, LogEntry{LogInfo, "totally works: yes"})
	})
}

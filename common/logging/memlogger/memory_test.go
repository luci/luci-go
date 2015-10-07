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
		c := Use(logging.SetLevel(context.Background(), logging.Debug))
		l := logging.Get(c)
		So(l, ShouldNotBeNil)
		l.Debugf("test %s", logging.Debug)
		l.Infof("test %s", logging.Info)
		l.Warningf("test %s", logging.Warning)
		l.Errorf("test %s", logging.Error)
		l.Errorf("test WAT: %s", logging.Level(9001))
		ml := l.(*MemLogger)

		msgs := ml.Messages()
		So(len(msgs), ShouldEqual, 5)
		So(msgs[0], ShouldResemble, LogEntry{logging.Debug, "test debug", nil})
		So(msgs[1], ShouldResemble, LogEntry{logging.Info, "test info", nil})
		So(msgs[2], ShouldResemble, LogEntry{logging.Warning, "test warning", nil})
		So(msgs[3], ShouldResemble, LogEntry{logging.Error, "test error", nil})
		So(msgs[4], ShouldResemble, LogEntry{logging.Error, "test WAT: unknown", nil})
	})

	Convey("logger context", t, func() {
		c := Use(context.Background())
		l := logging.Get(c)
		So(l, ShouldNotBeNil)
		ml := l.(*MemLogger)

		l.Infof("totally works: %s", "yes")

		msgs := ml.Messages()
		So(len(msgs), ShouldEqual, 1)
		So(msgs[0], ShouldResemble, LogEntry{logging.Info, "totally works: yes", nil})
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
		msgs := ml.Messages()
		So(msgs[0].Data["trombone"], ShouldEqual, 50)
		So(msgs[0].Data["cat"], ShouldEqual, "amazing")
	})

	Convey("reset", t, func() {
		c := Use(context.Background())
		l := logging.Get(c).(*MemLogger)

		l.Infof("hello")
		So(len(l.Messages()), ShouldEqual, 1)

		l.Reset()
		So(len(l.Messages()), ShouldEqual, 0)

		l.Infof("shweeet")
		So(len(l.Messages()), ShouldEqual, 1)
	})
}

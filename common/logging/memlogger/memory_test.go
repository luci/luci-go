// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memlogger

import (
	"bytes"
	"sync"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/luci/luci-go/common/logging"
)

func TestLogger(t *testing.T) {
	Convey("Zero", t, func() {
		var l MemLogger
		l.Debugf("test %s", logging.Debug)
		l.Infof("test %s", logging.Info)

		So(&l, ShouldHaveLog, logging.Debug, "test debug")
		So(&l, ShouldHaveLog, logging.Info, "test info")
	})
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

		So(ml, ShouldHaveLog, logging.Debug, "test debug")
		So(ml, ShouldHaveLog, logging.Info, "test info")
		So(ml, ShouldHaveLog, logging.Warning, "test warning")
		So(ml, ShouldHaveLog, logging.Error, "test error")
		So(ml, ShouldHaveLog, logging.Error, "test WAT: unknown")
	})

	Convey("logger context", t, func() {
		c := Use(context.Background())
		l := logging.Get(c)
		So(l, ShouldNotBeNil)
		ml := l.(*MemLogger)

		l.Infof("totally works: %s", "yes")

		So(ml, ShouldHaveLog, logging.Info, "totally works: yes")
		So(ml, ShouldNotHaveLog, logging.Warning, "totally works: yes")
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

	Convey("dump", t, func() {
		var l MemLogger
		l.fields = map[string]interface{}{"key": 100}
		l.Debugf("test %s", logging.Debug)
		l.Infof("test %s", logging.Info)

		buf := bytes.Buffer{}
		l.Dump(&buf)

		So(buf.String(), ShouldEqual, `
DUMP LOG:
  debug: test debug: {"key":100}
  info: test info: {"key":100}
`)
	})
}

func TestLoggerAssertion(t *testing.T) {
	t.Parallel()

	Convey("ShouldHaveLog", t, func() {
		Convey("basic", func() {
			m := &MemLogger{
				lock: &sync.Mutex{},
				data: &[]LogEntry{
					{
						Level:     logging.Error,
						Msg:       "HI THAR",
						Data:      map[string]interface{}{"hi": 3},
						CallDepth: 47,
					},
				},
			}

			So(ShouldHaveLog(m, logging.Error, "HI THAR", map[string]interface{}{"hi": 3}), ShouldEqual, "")
			So(ShouldHaveLog(m, logging.Error, "HI THAR", map[string]interface{}{"hi": 4}), ShouldNotEqual, "")
			So(ShouldHaveLog(m, logging.Error, "Hi THAR", map[string]interface{}{"hi": 4}), ShouldNotEqual, "")
			So(ShouldHaveLog(m, logging.Error, "THAR", map[string]interface{}{"hi": 4}), ShouldNotEqual, "")
		})

		Convey("level and message", func() {
			m := &MemLogger{
				lock: &sync.Mutex{},
				data: &[]LogEntry{
					{
						Level: logging.Error,
						Msg:   "HI THAR",
						Data:  map[string]interface{}{"hi": 3},
					},
				},
			}

			So(ShouldHaveLog(m, logging.Error, "HI THAR"), ShouldEqual, "")
		})

		Convey("level only", func() {
			m := &MemLogger{
				lock: &sync.Mutex{},
				data: &[]LogEntry{
					{
						Level:     logging.Error,
						Msg:       "HI THAR",
						Data:      nil,
						CallDepth: 47,
					},
				},
			}

			So(ShouldHaveLog(m, logging.Error, "BYE"), ShouldNotEqual, "")
			So(ShouldHaveLog(m, logging.Error), ShouldEqual, "")
		})

		Convey("bad logger", func() {
			So(ShouldHaveLog(nil), ShouldNotEqual, "")
		})

		Convey("bad level", func() {
			m := &MemLogger{}

			So(ShouldHaveLog(m, "BOO"), ShouldNotEqual, "")
		})

		Convey("bad message", func() {
			m := &MemLogger{}

			So(ShouldHaveLog(m, logging.Error, 48), ShouldNotEqual, "")
		})

		Convey("bad depth", func() {
			m := &MemLogger{}

			So(ShouldHaveLog(m, logging.Error, "HI THAR", "NO BAD"), ShouldNotEqual, "")
		})

		Convey("not found", func() {
			m := &MemLogger{
				lock: &sync.Mutex{},
				data: &[]LogEntry{},
			}

			So(ShouldHaveLog(m, logging.Error, "BYE THAR", 47), ShouldNotEqual, "")
		})

		Convey("need at least one argument", func() {
			m := &MemLogger{}

			So(ShouldHaveLog(m), ShouldNotEqual, "")
		})
	})
}

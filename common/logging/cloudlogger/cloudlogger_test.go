// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudlogger

import (
	"errors"
	"testing"
	"time"

	cloudLogging "cloud.google.com/go/logging"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeCloudLogger struct {
	entries []cloudLogging.Entry
}

func (f *fakeCloudLogger) Log(entry cloudLogging.Entry) {
	f.entries = append(f.entries, entry)
}

// testUse is a testing version of Use(), which does not assert that the main and lines
// loggers are cloudLogging.Logger.
func testUse(c context.Context, main, lines loggerIface) context.Context {
	return logging.SetFactory(c, GetFactory(main, lines))
}

func TestLogger(t *testing.T) {
	t.Parallel()

	Convey("Testing environment", t, func() {
		c := context.Background()
		c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
		parent := &fakeCloudLogger{}
		child := &fakeCloudLogger{}

		Convey("Single main logger", func() {
			c = testUse(c, parent, nil)
			Convey("Basic debug", func() {
				logging.Debugf(c, "this is a log")
				So(len(parent.entries), ShouldEqual, 1)
				So(parent.entries[0].Payload.(logging.Fields)["message"], ShouldEqual, "this is a log")
				So(parent.entries[0].Severity, ShouldEqual, cloudLogging.Debug)
			})
			Convey("Basic error", func() {
				logging.Errorf(c, "this is a log")
				So(len(parent.entries), ShouldEqual, 1)
				So(parent.entries[0].Payload.(logging.Fields)["message"], ShouldEqual, "this is a log")
				So(parent.entries[0].Severity, ShouldEqual, cloudLogging.Error)
			})
		})

		Convey("Both loggers", func() {
			c = testUse(c, parent, child)
			Convey("Basic debug lines", func() {
				logging.Debugf(c, "this is a log")
				So(len(parent.entries), ShouldEqual, 0)
				So(len(child.entries), ShouldEqual, 1)
				So(child.entries[0].Payload.(logging.Fields)["message"], ShouldEqual, "this is a log")
			})
			Convey("Basic with nil request", func() {
				c = StartTrace(c, nil)
				logging.Infof(c, "log line")
				c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Minute))
				EndTraceWithError(c, 0, nil)

				So(len(child.entries), ShouldEqual, 1)
				So(len(parent.entries), ShouldEqual, 1)
				So(parent.entries[0].Severity, ShouldEqual, cloudLogging.Info)
			})
			Convey("Basic with local IP", func() {
				c = WithLocalIP(c, "127.0.0.1")
				c = StartTrace(c, nil)
				logging.Infof(c, "log line")
				c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Minute))
				EndTraceWithError(c, 0, nil)

				So(parent.entries[0].HTTPRequest.LocalIP, ShouldEqual, "127.0.0.1")
			})
			Convey("Basic with err request", func() {
				c = StartTrace(c, nil)
				logging.Errorf(c, "error log line")
				c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC.Add(time.Minute))
				EndTraceWithError(c, 0, errors.New("some error"))

				So(len(child.entries), ShouldEqual, 1)
				So(len(parent.entries), ShouldEqual, 1)
				So(parent.entries[0].Severity, ShouldEqual, cloudLogging.Error)
				So(parent.entries[0].Payload.(logging.Fields)["error"], ShouldEqual, "some error")
			})
		})
	})
}

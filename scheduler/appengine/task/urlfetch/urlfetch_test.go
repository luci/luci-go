// Copyright 2015 The LUCI Authors.
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

package urlfetch

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/urlfetch"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"

	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/task/utils/tasktest"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateProtoMessage(t *testing.T) {
	tm := TaskManager{}

	Convey("ValidateProtoMessage passes good msg", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url: "https://blah.com",
		}), ShouldBeNil)
	})

	Convey("ValidateProtoMessage wrong type", t, func() {
		So(tm.ValidateProtoMessage(&messages.NoopTask{}), ShouldErrLike, "wrong type")
	})

	Convey("ValidateProtoMessage empty", t, func() {
		So(tm.ValidateProtoMessage(tm.ProtoMessageType()), ShouldErrLike, "expecting a non-empty UrlFetchTask")
	})

	Convey("ValidateProtoMessage bad method", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Method: "BLAH",
		}), ShouldErrLike, "unsupported HTTP method")
	})

	Convey("ValidateProtoMessage no URL", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{}), ShouldErrLike, "field 'url' is required")
	})

	Convey("ValidateProtoMessage bad URL", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url: "%%%%",
		}), ShouldErrLike, "invalid URL")
	})

	Convey("ValidateProtoMessage non-absolute URL", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url: "/abc",
		}), ShouldErrLike, "not an absolute url")
	})

	Convey("ValidateProtoMessage bad timeout", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url:        "https://blah.com",
			TimeoutSec: -1,
		}), ShouldErrLike, "minimum allowed 'timeout_sec' is 1 sec")
	})

	Convey("ValidateProtoMessage large timeout", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url:        "https://blah.com",
			TimeoutSec: 10000,
		}), ShouldErrLike, "maximum allowed 'timeout_sec' is 480 sec")
	})
}

func TestLaunchTask(t *testing.T) {
	tm := TaskManager{}

	Convey("LaunchTask works", t, func(c C) {
		ts, ctx := newTestContext(time.Unix(0, 1))
		defer ts.Close()
		ctl := &tasktest.TestController{
			TaskMessage: &messages.UrlFetchTask{
				Url: ts.URL,
			},
			SaveCallback: func() error { return nil },
		}
		So(tm.LaunchTask(ctx, ctl), ShouldBeNil)
		So(ctl.Log[0], ShouldEqual, "GET "+ts.URL)
		So(ctl.Log[1], ShouldStartWith, "Finished with overall status SUCCEEDED in 0")
	})
}

func newTestContext(now time.Time) (*httptest.Server, context.Context) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, Client!"))
	}))
	c := context.Background()
	c = clock.Set(c, testclock.New(now))
	c = urlfetch.Set(c, http.DefaultTransport)
	return ts, c
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package urlfetch

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/urlfetch"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"

	"github.com/luci/luci-go/appengine/cmd/cron/messages"
	"github.com/luci/luci-go/appengine/cmd/cron/task"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateProtoMessage(t *testing.T) {
	tm := TaskManager{}

	Convey("ValidateProtoMessage passes good msg", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url: strPtr("https://blah.com"),
		}), ShouldBeNil)
	})

	Convey("ValidateProtoMessage wrong type", t, func() {
		So(tm.ValidateProtoMessage(&messages.NoopTask{}), ShouldErrLike, "wrong type")
	})

	Convey("ValidateProtoMessage empty", t, func() {
		So(tm.ValidateProtoMessage(tm.ProtoMessageType()), ShouldErrLike, "field 'url' is required")
	})

	Convey("ValidateProtoMessage bad method", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Method: strPtr("BLAH"),
		}), ShouldErrLike, "unsupported HTTP method")
	})

	Convey("ValidateProtoMessage no URL", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{}), ShouldErrLike, "field 'url' is required")
	})

	Convey("ValidateProtoMessage bad URL", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url: strPtr("%%%%"),
		}), ShouldErrLike, "invalid URL")
	})

	Convey("ValidateProtoMessage non-absolute URL", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url: strPtr("/abc"),
		}), ShouldErrLike, "not an absolute url")
	})

	Convey("ValidateProtoMessage small timeout", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url:        strPtr("https://blah.com"),
			TimeoutSec: intPtr(0),
		}), ShouldErrLike, "minimum allowed 'timeout_sec' is 1 sec")
	})

	Convey("ValidateProtoMessage large timeout", t, func() {
		So(tm.ValidateProtoMessage(&messages.UrlFetchTask{
			Url:        strPtr("https://blah.com"),
			TimeoutSec: intPtr(10000),
		}), ShouldErrLike, "maximum allowed 'timeout_sec' is 480 sec")
	})
}

func TestLaunchTask(t *testing.T) {
	tm := TaskManager{}

	Convey("LaunchTask works", t, func(c C) {
		ts, ctx := newTestContext(time.Unix(0, 1))
		defer ts.Close()
		msg := &messages.UrlFetchTask{
			Url: strPtr(ts.URL),
		}
		ctl := &dumbController{}
		So(tm.LaunchTask(ctx, msg, ctl), ShouldBeNil)
		So(ctl.log[:2], ShouldResemble, []string{
			"GET " + ts.URL,
			"Finished with overall status SUCCEEDED in 0",
		})
	})
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int32 {
	j := int32(i)
	return &j
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

type dumbController struct {
	log    []string
	status task.Status
}

func (c *dumbController) DebugLog(format string, args ...interface{}) {
	c.log = append(c.log, fmt.Sprintf(format, args...))
}

func (c *dumbController) Save(status task.Status) error {
	c.status = status
	return nil
}

func (c *dumbController) PrepareTopic(publisher string) (topic string, token string, err error) {
	return "", "", errors.New("not implemented")
}

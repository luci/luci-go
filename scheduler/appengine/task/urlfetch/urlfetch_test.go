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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/urlfetch"

	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"
)

var _ task.Manager = (*TaskManager)(nil)

func TestValidateProtoMessage(t *testing.T) {
	t.Parallel()

	tm := TaskManager{}
	ftt.Run("ValidateProtoMessage works", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}
		validate := func(msg proto.Message) error {
			tm.ValidateProtoMessage(c, msg, "some-project:some-realm")
			return c.Finalize()
		}
		t.Run("ValidateProtoMessage passes good msg", func(t *ftt.Test) {
			assert.Loosely(t, validate(&messages.UrlFetchTask{
				Url: "https://blah.com",
			}), should.BeNil)
		})

		t.Run("ValidateProtoMessage wrong type", func(t *ftt.Test) {
			assert.Loosely(t, validate(&messages.NoopTask{}), should.ErrLike("wrong type"))
		})

		t.Run("ValidateProtoMessage empty", func(t *ftt.Test) {
			assert.Loosely(t, validate(tm.ProtoMessageType()), should.ErrLike("expecting a non-empty UrlFetchTask"))
		})

		t.Run("ValidateProtoMessage bad method", func(t *ftt.Test) {
			assert.Loosely(t, validate(&messages.UrlFetchTask{
				Method: "BLAH",
			}), should.ErrLike("unsupported HTTP method"))
		})

		t.Run("ValidateProtoMessage no URL", func(t *ftt.Test) {
			assert.Loosely(t, validate(&messages.UrlFetchTask{}), should.ErrLike("field 'url' is required"))
		})

		t.Run("ValidateProtoMessage bad URL", func(t *ftt.Test) {
			assert.Loosely(t, validate(&messages.UrlFetchTask{
				Url: "%%%%",
			}), should.ErrLike("invalid URL"))
		})

		t.Run("ValidateProtoMessage non-absolute URL", func(t *ftt.Test) {
			assert.Loosely(t, validate(&messages.UrlFetchTask{
				Url: "/abc",
			}), should.ErrLike("not an absolute url"))
		})

		t.Run("ValidateProtoMessage bad timeout", func(t *ftt.Test) {
			assert.Loosely(t, validate(&messages.UrlFetchTask{
				Url:        "https://blah.com",
				TimeoutSec: -1,
			}), should.ErrLike("minimum allowed 'timeout_sec' is 1 sec"))
		})

		t.Run("ValidateProtoMessage large timeout", func(t *ftt.Test) {
			assert.Loosely(t, validate(&messages.UrlFetchTask{
				Url:        "https://blah.com",
				TimeoutSec: 10000,
			}), should.ErrLike("maximum allowed 'timeout_sec' is 480 sec"))
		})
	})
}

func TestLaunchTask(t *testing.T) {
	t.Parallel()

	tm := TaskManager{}

	ftt.Run("LaunchTask works", t, func(c *ftt.Test) {
		ts, ctx := newTestContext(time.Unix(0, 1))
		defer ts.Close()
		ctl := &tasktest.TestController{
			TaskMessage: &messages.UrlFetchTask{
				Url: ts.URL,
			},
			SaveCallback: func() error { return nil },
		}
		assert.Loosely(c, tm.LaunchTask(ctx, ctl), should.BeNil)
		assert.Loosely(c, ctl.Log[0], should.Equal("GET "+ts.URL))
		assert.Loosely(c, ctl.Log[1], should.HavePrefix("Finished with overall status SUCCEEDED in 0"))
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

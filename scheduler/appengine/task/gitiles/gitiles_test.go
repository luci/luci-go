// Copyright 2016 The LUCI Authors.
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

package gitiles

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/urlfetch"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTriggerBuild(t *testing.T) {
	Convey("LaunchTask triggers builds for new commits", t, func(ctx C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			resp := ""
			switch r.URL.Path {
			case "/+refs":
				resp = `)]}'
{
  "refs/heads/master": {
    "value": "baddcafe"
  }
}`
			case "/+log/refs/heads/master~..refs/heads/master":
				resp = `)]}'
{
  "log": [
    {
      "commit": "baddcafe",
      "tree": "deadfeed",
      "parents": [
        "b0000000"
      ],
      "author": {
        "name": "First Last",
        "email": "firstlast@example.com",
        "time": "Mon Jan 01 12:00:00 1970 -0000"
      },
      "committer": {
        "name": "First Last",
        "email": "firstlast@example.com",
        "time": "Mon Jan 01 12:00:00 1970 -0000"
      },
      "message": "test\n"
    }
  ]
}`
			default:
				ctx.Printf("Unknown URL: %s\n", r.URL.Path)
				w.WriteHeader(400)
				return
			}
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, resp)
		}))
		defer ts.Close()

		c := memory.Use(context.Background())
		c = urlfetch.Set(c, http.DefaultTransport)

		m := TaskManager{}
		ctl := &tasktest.TestController{
			TaskMessage: &messages.GitilesTask{
				Repo: ts.URL,
				Refs: []string{"refs/heads/master"},
			},
			Client:        http.DefaultClient,
			SaveCallback:  func() error { return nil },
			OverrideJobID: "proj/job",
		}

		// Launch.
		So(m.LaunchTask(c, ctl), ShouldBeNil)
		So(ctl.Log[0], ShouldEqual, "Trigger build for commit baddcafe")
		r := Repository{ID: "proj/job:" + ts.URL}
		So(ds.Get(c, &r), ShouldBeNil)
		So(r, ShouldResemble, Repository{
			ID: "proj/job:" + ts.URL,
			References: []Reference{
				{Name: "refs/heads/master", Revision: "baddcafe"},
			},
		})
	})

	Convey("LaunchTask doesn't do anything if there are no new commits", t, func(ctx C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			resp := ""
			switch r.URL.Path {
			case "/+refs":
				resp = `)]}'
{
  "refs/heads/master": {
    "value": "baddcafe"
  }
}`
			default:
				ctx.Printf("Unknown URL: %s\n", r.URL.Path)
				w.WriteHeader(400)
				return
			}
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, resp)
		}))
		defer ts.Close()

		c := memory.Use(context.Background())
		c = urlfetch.Set(c, http.DefaultTransport)

		m := TaskManager{}
		ctl := &tasktest.TestController{
			TaskMessage: &messages.GitilesTask{
				Repo: ts.URL,
				Refs: []string{"refs/heads/master"},
			},
			Client:        http.DefaultClient,
			SaveCallback:  func() error { return nil },
			OverrideJobID: "proj/job",
		}

		// Launch.
		So(ds.Put(c,
			&Repository{
				ID: "proj/job:" + ts.URL,
				References: []Reference{
					{Name: "refs/heads/master", Revision: "baddcafe"},
				},
			},
		), ShouldBeNil)

		So(m.LaunchTask(c, ctl), ShouldBeNil)
		So(len(ctl.Log), ShouldEqual, 0)
	})
}

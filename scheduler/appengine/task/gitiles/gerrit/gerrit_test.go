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

package gerrit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/urlfetch"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetRefs(t *testing.T) {
	Convey("GetRefs returns refs", t, func(ctx C) {
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

		u, err := url.Parse(ts.URL)
		So(err, ShouldBeNil)
		g := NewGerrit(u)
		r, err := g.GetRefs(c, []string{"refs/heads/master"})
		So(err, ShouldBeNil)
		So(r, ShouldResemble, map[string]string{
			"refs/heads/master": "baddcafe",
		})
	})
}

func TestGetLog(t *testing.T) {
	Convey("GetLog returns commits", t, func(ctx C) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			resp := ""
			switch r.URL.Path {
			case "/+log/baddcafe~..refs/heads/master":
				resp = `)]}'
{
  "log": [
    {
      "commit": "deadcafe",
      "tree": "deadfeed",
      "parents": [
        "baddcafe"
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
  ],
  "next": "baddcafe"
}`
			case "/+log/baddcafe~..baddcafe":
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
        "time": "Mon Jan 01 12:00:00 1970"
      },
      "committer": {
        "name": "First Last",
        "email": "firstlast@example.com",
        "time": "Mon Jan 01 12:00:00 1970"
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

		u, err := url.Parse(ts.URL)
		So(err, ShouldBeNil)
		g := NewGerrit(u)
		l, err := g.GetLog(c, "refs/heads/master", "baddcafe~")
		So(err, ShouldBeNil)
		So(len(l), ShouldEqual, 2)
		So(l[0].Commit, ShouldEqual, "deadcafe")
		So(l[1].Commit, ShouldEqual, "baddcafe")
	})
}

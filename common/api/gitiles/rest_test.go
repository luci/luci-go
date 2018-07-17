// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/proto/gitiles"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("Log with bad project", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {})
		defer srv.Close()
		_, err := c.Log(ctx, &gitiles.LogRequest{})
		So(err, ShouldErrLike, "project is required")
	})

	Convey("Log w/o pages", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n{\"log\": [%s, %s]}\n", fakeCommit1Str, fakeCommit2Str)
		})
		defer srv.Close()

		Convey("Return All", func() {
			res, err := c.Log(ctx, &gitiles.LogRequest{
				Project:            "repo",
				Committish:         "8de6836858c99e48f3c58164ab717bda728e95dd",
				ExcludeAncestorsOf: "master",
				PageSize:           10,
			})
			So(err, ShouldBeNil)
			So(len(res.Log), ShouldEqual, 2)
			So(res.Log[0].Author.Name, ShouldEqual, "Author 1")
			So(res.Log[1].Id, ShouldEqual, "dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40")
		})
	})
}

func TestRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("bad project", t, func() {
		c := &client{BaseURL: "https://a.googlesource.com/a"}
		_, err := c.Refs(ctx, &gitiles.RefsRequest{
			RefsPath: gitiles.AllRefs,
		})
		So(err, ShouldErrLike, `project is required`)
	})

	Convey("bad RefsPath", t, func() {
		c := &client{BaseURL: "https://a.googlesource.com/a"}
		_, err := c.Refs(ctx, &gitiles.RefsRequest{
			Project:  "repo",
			RefsPath: "bad",
		})
		So(err, ShouldErrLike, `refsPath must be "refs" or start with "refs/"`)
	})

	Convey("Refs All", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `)]}'
				{
					"refs/heads/master": { "value": "deadbeef" },
					"refs/heads/infra/config": { "value": "0000beef" },
					"refs/changes/01/123001/1": { "value": "123dead001beef1" },
					"refs/other/ref": { "value": "ba6" },
					"123deadbeef123": { "target": "f00" },
					"HEAD": {
						"value": "deadbeef",
						"target": "refs/heads/master"
					}
				}
			`)
		})
		defer srv.Close()

		res, err := c.Refs(ctx, &gitiles.RefsRequest{
			Project:  "repo",
			RefsPath: gitiles.AllRefs,
		})
		So(err, ShouldBeNil)
		So(res.Revisions, ShouldResemble, map[string]string{
			"HEAD":                     "refs/heads/master",
			"refs/heads/master":        "deadbeef",
			"refs/heads/infra/config":  "0000beef",
			"refs/other/ref":           "ba6",
			"refs/changes/01/123001/1": "123dead001beef1",
			// Skipping "123dead001beef1" which has no value.
		})
	})
	Convey("Refs heads", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `)]}'
				{
					"master": { "value": "deadbeef" },
					"infra/config": { "value": "0000beef" }
				}
			`)
		})
		defer srv.Close()

		res, err := c.Refs(ctx, &gitiles.RefsRequest{
			Project:  "repo",
			RefsPath: gitiles.Branches,
		})
		So(err, ShouldBeNil)
		So(res.Revisions, ShouldResemble, map[string]string{
			"refs/heads/master":       "deadbeef",
			"refs/heads/infra/config": "0000beef",
		})
	})
}
func newMockClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, gitiles.GitilesClient) {
	srv := httptest.NewServer(http.HandlerFunc(handler))
	return srv, &client{BaseURL: srv.URL}
}

var (
	fakeCommit1Str = `{
		"commit": "0b2c5409e58a71c691b05454b55cc5580cc822d1",
		"tree": "3c6f95bc757698cd6aca3c49f88f640fd145ea69",
		"parents": [ "dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40" ],
		"author": {
			"name": "Author 1",
			"email": "author1@example.com",
			"time": "Mon Jul 17 15:02:43 2017 -0800"
		},
		"committer": {
			"name": "Commit Bot",
			"email": "commit-bot@chromium.org",
			"time": "Mon Jul 17 15:02:43 2017 +0000"
		},
		"message": "Import wpt@d96d68ed964f9bfc2bb248c2d2fab7a8870dc685\\n\\nCr-Commit-Position: refs/heads/master@{#487078}"
	}`
	fakeCommit2Str = `{
		"commit": "dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40",
		"tree": "1ba2335c07915c31597b97a8d824aecc85a996f6",
		"parents": ["8de6836858c99e48f3c58164ab717bda728e95dd"],
		"author": {
			"name": "Author 2",
			"email": "author-2@example.com",
			"time": "Mon Jul 17 15:01:13 2017"
		},
		"committer": {
			"name": "Commit Bot",
			"email": "commit-bot@chromium.org",
			"time": "Mon Jul 17 15:01:13 2017"
		},
		"message": "[Web Payments] User weak ptr in Payment Request\u0027s error callback\\n\\nBug: 742329\\nReviewed-on: https://chromium-review.googlesource.com/570982\\nCr-Commit-Position: refs/heads/master@{#487077}"
  }`
)

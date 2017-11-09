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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTime(t *testing.T) {
	t.Parallel()

	Convey("Marshal and Unmarshal Identity", t, func() {
		// Nanoseconds must be zero because the string format in between
		// does not contain nanoseconds.
		tBefore := Time{time.Date(12, 2, 5, 6, 1, 3, 0, time.UTC)}
		bytes, err := json.Marshal(tBefore)
		So(err, ShouldBeNil)

		var tAfter Time
		err = json.Unmarshal(bytes, &tAfter)
		So(err, ShouldBeNil)

		So(tBefore, ShouldResemble, tAfter)
	})
}

func TestRepoURL(t *testing.T) {
	t.Parallel()
	Convey("Malformed", t, func() {
		f := func(arg string) {
			So(ValidateRepoURL(arg), ShouldNotBeNil)
			_, err := NormalizeRepoURL(arg, true)
			So(err, ShouldNotBeNil)
		}

		f("wtf/\\is\this")
		f("https://example.com/repo.git")
		f("http://bad-protocol.googlesource.com/repo.git")
		f("https://a.googlesource.com")
		f("https://a.googlesource.com/")
		f("a.googlesource.com/no-protocol.git")
		f("https://a.googlesource.com/no-protocol#fragment")
	})

	Convey("OK", t, func() {
		f := func(arg, exp string) {
			So(ValidateRepoURL(arg), ShouldBeNil)
			act, err := NormalizeRepoURL(arg, true)
			So(err, ShouldBeNil)
			So(act, ShouldEqual, exp)
		}

		f("https://chromium.googlesource.com/repo.git",
			"https://chromium.googlesource.com/a/repo")
		f("https://chromium.googlesource.com/repo/",
			"https://chromium.googlesource.com/a/repo")
		f("https://chromium.googlesource.com/a/repo",
			"https://chromium.googlesource.com/a/repo")
		f("https://chromium.googlesource.com/parent/repo.git/",
			"https://chromium.googlesource.com/a/parent/repo")
	})
}

func TestLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("Log with bad URL", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {})
		defer srv.Close()
		_, err := c.Log(ctx, "bad://repo.git/", "master", Limit(10))
		So(err, ShouldNotBeNil)
	})

	Convey("Log w/o pages", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n{\"log\": [%s, %s]}\n", fakeCommit1Str, fakeCommit2Str)
		})
		defer srv.Close()

		Convey("Return All", func() {
			commits, err := c.Log(ctx, "https://c.googlesource.com/repo",
				"master..8de6836858c99e48f3c58164ab717bda728e95dd", Limit(10))
			So(err, ShouldBeNil)
			So(len(commits), ShouldEqual, 2)
			So(commits[0].Author.Name, ShouldEqual, "Author 1")
			So(commits[1].Commit, ShouldEqual, "dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40")
		})

		Convey("Do not exceed limit", func() {
			commits, err := c.Log(ctx, "https://c.googlesource.com/repo",
				"master..8de6836858c99e48f3c58164ab717bda728e95dd", Limit(1))
			So(err, ShouldBeNil)
			So(len(commits), ShouldEqual, 1)
			So(commits[0].Author.Name, ShouldEqual, "Author 1")
		})
	})

	Convey("Log Paging", t, func() {
		cursorSent := ""
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			if next := r.URL.Query().Get("s"); next == "" {
				fmt.Fprintf(w, ")]}'\n{\"log\": [%s], \"next\": \"next_cursor_value\"}\n", fakeCommit1Str)
			} else {
				cursorSent = next
				fmt.Fprintf(w, ")]}'\n{\"log\": [%s]}\n", fakeCommit2Str)
			}
		})
		defer srv.Close()

		Convey("Page till no cursor", func() {
			commits, err := c.Log(ctx, "https://c.googlesource.com/repo",
				"master..8de6836858c99e48f3c58164ab717bda728e95dd", Limit(10))
			So(err, ShouldBeNil)
			So(cursorSent, ShouldEqual, "next_cursor_value")
			So(len(commits), ShouldEqual, 2)
			So(commits[0].Author.Name, ShouldEqual, "Author 1")
			So(commits[1].Commit, ShouldEqual, "dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40")
		})

		Convey("Page till limit", func() {
			commits, err := c.Log(ctx, "https://c.googlesource.com/repo",
				"master", Limit(1))
			So(err, ShouldBeNil)
			So(cursorSent, ShouldEqual, "")
			So(len(commits), ShouldEqual, 1)
			So(commits[0].Author.Name, ShouldEqual, "Author 1")
		})
	})
}

func TestLogForward(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("LogForward with bad URL", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {})
		defer srv.Close()
		_, err := c.LogForward(ctx, "bad://repo.git/", "8de6836858c99e48f3c58164ab717bda728e95dd", "master")
		So(err, ShouldNotBeNil)
	})

	Convey("LogForward w/o pages", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n{\"log\": [%s, %s]}\n", fakeCommit1Str, fakeCommit2Str)
		})
		defer srv.Close()

		Convey("Return All", func() {
			commits, err := c.LogForward(ctx, "https://c.googlesource.com/repo",
				"8de6836858c99e48f3c58164ab717bda728e95dd", "master")
			So(err, ShouldBeNil)
			So(len(commits), ShouldEqual, 2)
			So(commits[0].Author.Name, ShouldEqual, "Author 2")
			So(commits[1].Commit, ShouldEqual, "0b2c5409e58a71c691b05454b55cc5580cc822d1")
		})
	})

	Convey("LogForward Paging", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			next := r.URL.Query().Get("s")
			logEntries := make([]string, 100)
			for i := 0; i < 100; i++ {
				logEntries[i] = fakeCommit1Str
			}
			logStr := strings.Join(logEntries, ", ")
			switch next {
			case "":
				fmt.Fprintf(w, ")]}'\n{\"log\": [%s], \"next\": \"next_cursor_value\"}\n", logStr)
			case "next_cursor_value":
				fmt.Fprintf(w, ")]}'\n{\"log\": [%s], \"next\": \"last_cursor_value\"}\n", logStr)
			case "last_cursor_value":
				fmt.Fprintf(w, ")]}'\n{\"log\": [%s]}\n", fakeCommit2Str)
			}
		})
		defer srv.Close()

		Convey("Page past limit", func() {
			commits, err := c.LogForward(ctx, "https://c.googlesource.com/repo",
				"8de6836858c99e48f3c58164ab717bda728e95dd", "master")
			So(err, ShouldBeNil)
			So(len(commits), ShouldEqual, 201)
			So(commits[0].Author.Name, ShouldEqual, "Author 2")
			So(commits[0].Commit, ShouldEqual, "dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40")
		})

	})
}

func TestRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("Refs Bad RefsPath", t, func() {
		c := Client{mockRepoURL: "https://a.googlesource.com/a/repo"}
		_, err := c.Refs(context.Background(), "https://c.googlesource.com/repo", "bad")
		So(err, ShouldNotBeNil)
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
					"HEAD": { "value": "deadbeef",
										"target": "refs/heads/master" }
				}
			`)
		})
		defer srv.Close()
		refs, err := c.Refs(ctx, "https://c.googlesource.com/repo", "refs")
		So(err, ShouldBeNil)
		So(refs, ShouldResemble, map[string]string{
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
		refs, err := c.Refs(ctx, "https://c.googlesource.com/repo", "refs/heads")
		So(err, ShouldBeNil)
		So(refs, ShouldResemble, map[string]string{
			"refs/heads/master":       "deadbeef",
			"refs/heads/infra/config": "0000beef",
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

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

////////////////////////////////////////////////////////////////////////////////

func newMockClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *Client) {
	srv := httptest.NewServer(http.HandlerFunc(handler))
	return srv, &Client{Client: http.DefaultClient, mockRepoURL: srv.URL}
}

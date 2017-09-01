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

package gerrit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGerritURL(t *testing.T) {
	t.Parallel()
	Convey("Malformed", t, func() {
		f := func(arg string) {
			So(ValidateGerritURL(arg), ShouldNotBeNil)
			_, err := NormalizeGerritURL(arg)
			So(err, ShouldNotBeNil)
		}

		f("wtf/\\is\this")
		f("https://example.com/")
		f("http://bad-protocol-review.googlesource.com/")
		f("no-protocol-review.googlesource.com/")
		f("https://a-review.googlesource.com/path-and#fragment")
		f("https://a-review.googlesource.com/any-path-actually")
	})

	Convey("OK", t, func() {
		f := func(arg, exp string) {
			So(ValidateGerritURL(arg), ShouldBeNil)
			act, err := NormalizeGerritURL(arg)
			So(err, ShouldBeNil)
			So(act, ShouldEqual, exp)
		}
		f("https://a-review.googlesource.com", "https://a-review.googlesource.com/")
		f("https://a-review.googlesource.com/", "https://a-review.googlesource.com/")
		f("https://chromium-review.googlesource.com/", "https://chromium-review.googlesource.com/")
		f("https://chromium-review.googlesource.com", "https://chromium-review.googlesource.com/")
	})
}

func TestNewClient(t *testing.T) {
	t.Parallel()
	Convey("Malformed", t, func() {
		f := func(arg string) {
			_, err := NewClient(http.DefaultClient, arg)
			So(err, ShouldNotBeNil)
		}
		f("badurl")
		f("http://a.googlesource.com")
		f("https://a/")
	})
	Convey("OK", t, func() {
		f := func(arg string) {
			_, err := NewClient(http.DefaultClient, arg)
			So(err, ShouldBeNil)
		}
		f("https://a-review.googlesource.com/")
		f("https://a-review.googlesource.com")
	})
}

func TestQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("ChangeQuery", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n[%s]\n", fakeCL1Str)
		})
		defer srv.Close()

		Convey("Basic", func() {
			cls, more, err := c.ChangeQuery(ctx,
				ChangeQueryRequest{
					Query: "some_query",
				})
			So(err, ShouldBeNil)
			So(len(cls), ShouldEqual, 1)
			So(cls[0].Owner.AccountID, ShouldEqual, 1118104)
			So(more, ShouldBeFalse)
		})

	})

	Convey("ChangeQuery with more changes", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n[%s]\n", fakeCL2Str)
		})
		defer srv.Close()

		Convey("Basic", func() {
			cls, more, err := c.ChangeQuery(ctx,
				ChangeQueryRequest{
					Query: "4efbec9a685b238fced35b81b7f3444dc60150b1",
				})
			So(err, ShouldBeNil)
			So(len(cls), ShouldEqual, 1)
			So(cls[0].Owner.AccountID, ShouldEqual, 1178184)
			So(more, ShouldBeFalse)
		})
	})

}
func TestGetChangeDetails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("Details", t, func() {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeCL3Str)
		})
		defer srv.Close()

		Convey("WithOptions", func() {
			cl, err := c.GetChangeDetails(ctx, "629279", []string{"CURRENT_REVISION"})
			So(err, ShouldBeNil)
			So(cl.RevertOf, ShouldEqual, 629277)
			So(cl.CurrentRevision, ShouldEqual, "1ee75012c0de")
		})

	})

}

////////////////////////////////////////////////////////////////////////////////

var (
	fakeCL1Str = `{
	    "id": "infra%2Fluci%2Fluci-go~master~I4c01b6686740f15844dc86aab73ee4ce00b90fe3",
	    "project": "infra/luci/luci-go",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "I4c01b6686740f15844dc86aab73ee4ce00b90fe3",
	    "subject": "gitiles: Implement forward log.",
	    "status": "NEW",
	    "created": "2017-08-22 18:46:58.000000000",
	    "updated": "2017-08-23 22:33:34.000000000",
	    "submit_type": "REBASE_ALWAYS",
	    "mergeable": true,
	    "insertions": 154,
	    "deletions": 23,
	    "unresolved_comment_count": 3,
	    "has_review_started": true,
	    "_number": 627036,
	    "owner": {
		"_account_id": 1118104
	    }
	}`
	fakeCL2Str = `{
	    "id": "infra%2Finfra~master~Ia292f77ae6bd94afbd746da0b08500f738904d15",
	    "project": "infra/infra",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "Ia292f77ae6bd94afbd746da0b08500f738904d15",
	    "subject": "[Findit] Add flake analyzer forced rerun instructions to makefile.",
	    "status": "MERGED",
	    "created": "2017-08-23 17:25:40.000000000",
	    "updated": "2017-08-23 22:51:03.000000000",
	    "submitted": "2017-08-23 22:51:03.000000000",
	    "insertions": 4,
	    "deletions": 1,
	    "unresolved_comment_count": 0,
	    "has_review_started": true,
	    "_number": 629277,
	    "owner": {
		"_account_id": 1178184
	    },
	    "_has_more_changes": true
	}`
	fakeCL3Str = `{
	    "id": "infra%2Finfra~master~Ia292f77ae6bd94a000046da0b08500f738904d15",
	    "project": "infra/infra",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "Ia292f77ae6bd94a000046da0b08500f738904d15",
	    "subject": "Revert of [Findit] Add flake analyzer forced rerun instructions to makefile.",
	    "status": "MERGED",
	    "current_revision" : "1ee75012c0de",
	    "revert_of": 629277,
	    "created": "2017-08-23 18:25:40.000000000",
	    "updated": "2017-08-23 23:51:03.000000000",
	    "submitted": "2017-08-23 23:51:03.000000000",
	    "insertions": 1,
	    "deletions": 4,
	    "unresolved_comment_count": 0,
	    "has_review_started": true,
	    "_number": 629279,
	    "owner": {
		"_account_id": 1178184
	    }
	}`
)

////////////////////////////////////////////////////////////////////////////////

func newMockClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *Client) {
	srv := httptest.NewServer(http.HandlerFunc(handler))
	pu, _ := url.Parse(srv.URL)
	c := &Client{http.DefaultClient, *pu}
	return srv, c
}

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

package gerrit

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/smartystreets/goconvey/convey"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("GetChange", t, func() {
		Convey("Validate args", func() {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {})
			defer srv.Close()

			_, err := c.GetChange(ctx, &gerritpb.GetChangeRequest{})
			So(err, ShouldErrLike, "number must be positive")
		})

		req := &gerritpb.GetChangeRequest{Number: 1}

		Convey("HTTP 404", func() {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
			})
			defer srv.Close()

			_, err := c.GetChange(ctx, req)
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.NotFound)
		})

		Convey("HTTP 200", func() {
			expectedChange := &gerritpb.ChangeInfo{
				Number: 1,
				Owner: &gerritpb.AccountInfo{
					Name:            "John Doe",
					Email:           "jdoe@example.com",
					SecondaryEmails: []string{"johndoe@chromium.org"},
					Username:        "jdoe",
				},
				Project:         "example/repo",
				Ref:             "refs/heads/master",
				CurrentRevision: "deadbeef",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Number: 1,
						Ref:    "refs/changes/123",
						Files: map[string]*gerritpb.FileInfo{
							"go/to/file.go": {
								LinesInserted: 32,
								LinesDeleted:  44,
								SizeDelta:     -567,
								Size:          11984,
							},
						},
					},
				},
			}
			var actualRequest *http.Request
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				actualRequest = r
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `)]}'{
					"_number": 1,
					"owner": {
						"name":             "John Doe",
						"email":            "jdoe@example.com",
						"secondary_emails": ["johndoe@chromium.org"],
						"username":         "jdoe"
					},
					"project": "example/repo",
					"branch":  "master",
					"current_revision": "deadbeef",
					"revisions": {
						"deadbeef": {
							"_number": 1,
							"ref": "refs/changes/123",
							"files": {
								"go/to/file.go": {
									"lines_inserted": 32,
									"lines_deleted": 44,
									"size_delta": -567,
									"size": 11984
								}
							}
						}
					}
				}`)
			})
			defer srv.Close()

			Convey("Basic", func() {
				res, err := c.GetChange(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, expectedChange)
			})
			Convey("Options", func() {
				req.Options = append(req.Options, gerritpb.QueryOption_DETAILED_ACCOUNTS, gerritpb.QueryOption_ALL_COMMITS)
				_, err := c.GetChange(ctx, req)
				So(err, ShouldBeNil)
				So(
					actualRequest.URL.Query()["o"],
					ShouldResemble,
					[]string{"DETAILED_ACCOUNTS", "ALL_COMMITS"},
				)
			})
			Convey("Request", func() {
				_, err := c.GetChange(ctx, req)
				So(err, ShouldBeNil)

				body, err := ioutil.ReadAll(actualRequest.Body)
				So(err, ShouldBeNil)
				So(body, ShouldHaveLength, 0)
			})
		})
	})
}

func TestRestCreateChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("CreateChange basic", t, func() {
		var actualBody []byte
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			// ignore errors here, but verify body later.
			actualBody, _ = ioutil.ReadAll(r.Body)
			w.WriteHeader(201)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'`)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"_number":   1,
				"project":   "example/repo",
				"branch":    "master",
				"change_id": "c1",
				"status":    "NEW",
			})
		})
		defer srv.Close()

		req := gerritpb.CreateChangeRequest{
			Project:    "example/repo",
			Ref:        "refs/heads/master",
			Subject:    "example subject",
			BaseCommit: "someOpaqueHash",
		}
		res, err := c.CreateChange(ctx, &req)
		So(err, ShouldBeNil)
		So(res, ShouldResemble, &gerritpb.ChangeInfo{
			Number:  1,
			Project: "example/repo",
			Ref:     "refs/heads/master",
			Status:  gerritpb.ChangeInfo_NEW,
		})

		var ci changeInput
		err = json.Unmarshal(actualBody, &ci)
		So(err, ShouldBeNil)
		So(ci, ShouldResemble, changeInput{
			Project:    "example/repo",
			Branch:     "refs/heads/master",
			Subject:    "example subject",
			BaseCommit: "someOpaqueHash",
		})
	})
}

func TestRestChangeEditFileContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("ChangeEditFileContent basic", t, func() {
		// large enough?
		var actualBody []byte
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			// ignore errors here, but verify body later.
			actualBody, _ = ioutil.ReadAll(r.Body)
			// API returns 204 on success.
			w.WriteHeader(204)
		})
		defer srv.Close()

		_, err := c.ChangeEditFileContent(ctx, &gerritpb.ChangeEditFileContentRequest{
			Number:   42,
			Project:  "someproject",
			FilePath: "some/path",
			Content:  []byte("changed file"),
		})
		So(err, ShouldBeNil)
		So(actualURL.Path, ShouldEqual, "/changes/someproject~42/edit/some/path")
		So(actualBody, ShouldResemble, []byte("changed file"))
	})
}

func TestGetMergeable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("GetMergeable basic", t, func() {
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
        {
          "submit_type": "CHERRY_PICK",
          "strategy": "simple-two-way-in-core",
          "mergeable": true,
          "commit_merged": false,
          "content_merged": false,
          "conflicts": [
            "conflict1",
            "conflict2"
          ],
          "mergeable_into": [
            "my_branch_1"
          ]
        }`)
		})
		defer srv.Close()

		mi, err := c.GetMergeable(ctx, &gerritpb.GetMergeableRequest{
			Number:     42,
			Project:    "someproject",
			RevisionId: "somerevision",
		})
		So(err, ShouldBeNil)
		So(actualURL.Path, ShouldEqual, "/changes/someproject~42/revisions/somerevision/mergeable")
		So(mi, ShouldResemble, &gerritpb.MergeableInfo{
			SubmitType:    gerritpb.MergeableInfo_CHERRY_PICK,
			Strategy:      gerritpb.MergeableStrategy_SIMPLE_TWO_WAY_IN_CORE,
			Mergeable:     true,
			CommitMerged:  false,
			ContentMerged: false,
			Conflicts:     []string{"conflict1", "conflict2"},
			MergeableInto: []string{"my_branch_1"},
		})
	})
}

func newMockPbClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, gerritpb.GerritClient) {
	// TODO(tandrii): rename this func once newMockClient name is no longer used in the same package.
	srv := httptest.NewServer(http.HandlerFunc(handler))
	return srv, &client{BaseURL: srv.URL}
}

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
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	. "github.com/smartystreets/goconvey/convey"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("ListChanges", t, func() {
		Convey("Validates Limit number", func() {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {})
			defer srv.Close()

			_, err := c.ListChanges(ctx, &gerritpb.ListChangesRequest{
				Query: "label:Commit-Queue",
				Limit: -1,
			})
			So(err, ShouldErrLike, "must be nonnegative")

			_, err = c.ListChanges(ctx, &gerritpb.ListChangesRequest{
				Query: "label:Commit-Queue",
				Limit: 1001,
			})
			So(err, ShouldErrLike, "should be at most")
		})

		req := &gerritpb.ListChangesRequest{
			Query: "label:Code-Review",
			Limit: 1,
		}

		Convey("With a HTTP 404 response", func() {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
			})
			defer srv.Close()
			_, err := c.ListChanges(ctx, req)
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.NotFound)
		})

		Convey("OK case with one change, _more_changes set in response", func() {
			expectedResponse := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{
					{
						Number: 1,
						Owner: &gerritpb.AccountInfo{
							Name:     "John Doe",
							Email:    "jdoe@example.com",
							Username: "jdoe",
						},
						Project: "example/repo",
						Ref:     "refs/heads/master",
						Created: timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
						Updated: timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
					},
				},
				MoreChanges: true,
			}
			var actualRequest *http.Request
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				actualRequest = r
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `)]}'[
					{
						"_number": 1,
						"owner": {
							"name":             "John Doe",
							"email":            "jdoe@example.com",
							"username":         "jdoe"
						},
						"project": "example/repo",
						"branch":  "master",
						"created":   "2014-05-05 07:15:44.639000000",
						"updated":   "2014-05-05 07:15:44.639000000",
						"_more_changes": true
					}
				]`)
			})
			defer srv.Close()

			Convey("Response and request are as expected", func() {
				res, err := c.ListChanges(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, expectedResponse)
				So(actualRequest.URL.Query()["q"], ShouldResemble, []string{"label:Code-Review"})
				So(actualRequest.URL.Query()["S"], ShouldResemble, []string{"0"})
				So(actualRequest.URL.Query()["n"], ShouldResemble, []string{"1"})
			})

			Convey("Options are included in the request", func() {
				req.Options = append(req.Options, gerritpb.QueryOption_DETAILED_ACCOUNTS, gerritpb.QueryOption_ALL_COMMITS)
				_, err := c.ListChanges(ctx, req)
				So(err, ShouldBeNil)
				So(
					actualRequest.URL.Query()["o"],
					ShouldResemble,
					[]string{"DETAILED_ACCOUNTS", "ALL_COMMITS"},
				)
			})
		})
	})
}

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
				Labels: map[string]*gerritpb.LabelInfo{
					"Code-Review": {
						Approved: &gerritpb.AccountInfo{
							Name:  "Rubber Stamper",
							Email: "rubberstamper@example.com",
						},
					},
				},
				Created: timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
				Updated: timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
				Messages: []*gerritpb.ChangeMessageInfo{
					{
						Id: "YH-egE",
						Author: &gerritpb.AccountInfo{
							Name:     "John Doe",
							Email:    "john.doe@example.com",
							Username: "jdoe",
						},
						Date:    timestamppb.New(parseTime("2013-03-23T21:34:02.419000000Z")),
						Message: "Patch Set 1:\n\nThis is the first message.",
					},
					{
						Id: "WEEdhU",
						Author: &gerritpb.AccountInfo{
							Name:     "Jane Roe",
							Email:    "jane.roe@example.com",
							Username: "jroe",
						},
						Date:    timestamppb.New(parseTime("2013-03-23T21:36:52.332000000Z")),
						Message: "Patch Set 1:\n\nThis is the second message.\n\nWith a line break.",
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
					"created":   "2014-05-05 07:15:44.639000000",
					"updated":   "2014-05-05 07:15:44.639000000",
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
					},
					"labels": {
						"Code-Review": {
							"approved": {
								"name": "Rubber Stamper",
								"email": "rubberstamper@example.com"
							}
						}
					},
					"messages": [
						{
							"id": "YH-egE",
							"author": {
								"_account_id": 1000096,
								"name": "John Doe",
								"email": "john.doe@example.com",
								"username": "jdoe"
							},
							"date": "2013-03-23 21:34:02.419000000",
							"message": "Patch Set 1:\n\nThis is the first message.",
							"_revision_number": 1
						},
						{
							"id": "WEEdhU",
							"author": {
								"_account_id": 1000097,
								"name": "Jane Roe",
								"email": "jane.roe@example.com",
								"username": "jroe"
							},
							"date": "2013-03-23 21:36:52.332000000",
							"message": "Patch Set 1:\n\nThis is the second message.\n\nWith a line break.",
							"_revision_number": 1
						}
					]
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
				"created":   "2014-05-05 07:15:44.639000000",
				"updated":   "2014-05-05 07:15:44.639000000",
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
			Number:      1,
			Project:     "example/repo",
			Ref:         "refs/heads/master",
			Status:      gerritpb.ChangeInfo_NEW,
			Submittable: false,
			Created:     timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
			Updated:     timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
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

// TODO (yulanlin): Assert body verbatim without decoding
func TestAddReviewer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("Add reviewer to cc basic", t, func() {
		var actualURL *url.URL
		var actualBody []byte
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			// ignore the error because body contents will be checked
			actualBody, _ = ioutil.ReadAll(r.Body)
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
			{
				"input": "ccer@test.com",
				"ccs": [
					{
						"_account_id": 10001,
						"name": "Reviewer Review",
						"approvals": {
							"Code-Review": "-1"
						}
					}
				]
			}`)
		})
		defer srv.Close()

		req := &gerritpb.AddReviewerRequest{
			Number:    42,
			Project:   "someproject",
			Reviewer:  "ccer@test.com",
			State:     gerritpb.AddReviewerRequest_ADD_REVIEWER_STATE_CC,
			Confirmed: true,
			Notify:    gerritpb.Notify_NOTIFY_OWNER,
		}
		res, err := c.AddReviewer(ctx, req)
		So(err, ShouldBeNil)

		// assert the request was as expected
		So(actualURL.Path, ShouldEqual, "/changes/someproject~42/reviewers")
		var body addReviewerRequest
		err = json.Unmarshal(actualBody, &body)
		if err != nil {
			t.Logf("failed to decode req body: %v\n", err)
		}
		So(body, ShouldResemble, addReviewerRequest{
			Reviewer:  "ccer@test.com",
			State:     "CC",
			Confirmed: true,
			Notify:    "OWNER",
		})

		// assert the result was as expected
		So(res, ShouldResemble, &gerritpb.AddReviewerResult{
			Input:     "ccer@test.com",
			Reviewers: []*gerritpb.ReviewerInfo{},
			Ccs: []*gerritpb.ReviewerInfo{
				&gerritpb.ReviewerInfo{
					Account: &gerritpb.AccountInfo{
						Name:      "Reviewer Review",
						AccountId: 10001,
					},
					Approvals: map[string]int32{
						"Code-Review": -1,
					},
				},
			},
		})
	})
}

func TestAddToAttentionSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("Add to attention set", t, func() {
		var actualURL *url.URL
		var actualBody []byte
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			// ignore the error because body contents will be checked
			actualBody, _ = ioutil.ReadAll(r.Body)
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
				{
					"_account_id": 10001,
					"name": "FYI reviewer",
					"email": "fyi@test.com",
					"username": "fyi"
				}`)
		})
		defer srv.Close()

		req := &gerritpb.AttentionSetRequest{
			Project: "someproject",
			Number:  42,
			User:    "fyi@test.com",
			Reason:  "For awareness",
			Notify:  gerritpb.Notify_NOTIFY_ALL,
		}
		res, err := c.AddToAttentionSet(ctx, req)
		So(err, ShouldBeNil)

		// assert the request was as expected
		So(actualURL.Path, ShouldEqual, "/changes/someproject~42/attention")
		expectedBody, err := json.Marshal(attentionSetRequest{
			User:   "fyi@test.com",
			Reason: "For awareness",
			Notify: "ALL",
		})
		if err != nil {
			t.Logf("failed to encode expected body: %v\n", err)
		}
		So(actualBody, ShouldResemble, expectedBody)

		// assert the result was as expected
		So(res, ShouldResemble, &gerritpb.AccountInfo{
			AccountId: 10001,
			Name:      "FYI reviewer",
			Email:     "fyi@test.com",
			Username:  "fyi",
		})
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

func TestListFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("ListFiles basic", t, func() {
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
				{
					"gerrit-server/src/main/java/com/google/gerrit/server/project/RefControl.java": {
						"lines_inserted": 123456
					},
					"file2": {
						"size": 7
					}
				}`)
		})
		defer srv.Close()

		mi, err := c.ListFiles(ctx, &gerritpb.ListFilesRequest{
			Number:     42,
			Project:    "someproject",
			RevisionId: "somerevision",
			Parent:     999,
		})
		So(err, ShouldBeNil)
		So(actualURL.Path, ShouldEqual, "/changes/someproject~42/revisions/somerevision/files/")
		So(actualURL.Query().Get("parent"), ShouldEqual, "999")
		So(mi, ShouldResemble, &gerritpb.ListFilesResponse{
			Files: map[string]*gerritpb.FileInfo{
				"gerrit-server/src/main/java/com/google/gerrit/server/project/RefControl.java": {
					LinesInserted: 123456,
				},
				"file2": {
					Size: 7,
				},
			},
		})
	})
}

func TestListProjects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("ListProjects basic", t, func() {
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
			{
				"android_apks": {
				  "id": "android_apks",
				  "state": "ACTIVE",
				  "branches": {
					"main": "82264ea131fcc2a386b83e38b962b370315c7c93"
				  },
				  "web_links": [
					{
					  "name": "gitiles",
					  "url": "https://chromium.googlesource.com/android_apks/",
					  "target": "_blank"
					}
				  ]
				}
			  }`)
		})
		defer srv.Close()

		projects, err := c.ListProjects(ctx, &gerritpb.ListProjectsRequest{
			Ref: "refs/heads/main",
		})
		So(err, ShouldBeNil)
		So(actualURL.Path, ShouldEqual, "/projects/")
		So(actualURL.Query().Get("b"), ShouldEqual, "refs/heads/main")
		So(projects, ShouldResemble, &gerritpb.ListProjectsResponse{
			Projects: map[string]*gerritpb.ProjectInfo{
				"android_apks": &gerritpb.ProjectInfo{
					Name:  "android_apks",
					State: gerritpb.ProjectInfo_PROJECT_STATE_ACTIVE,
					Refs: map[string]string{
						"refs/heads/main": "82264ea131fcc2a386b83e38b962b370315c7c93",
					},
					WebLinks: []*gerritpb.WebLinkInfo{
						&gerritpb.WebLinkInfo{
							Name: "gitiles",
							Url:  "https://chromium.googlesource.com/android_apks/",
						},
					},
				},
			},
		})
	})
}

func TestGetBranchInfo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("Get Branch Info", t, func() {
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
			{
				"web_links": [
				  {
					"name": "gitiles",
					"url": "https://chromium.googlesource.com/infra/experimental/+/refs/heads/main",
					"target": "_blank"
				  }
				],
				"ref": "refs/heads/main",
				"revision": "10e5c33f63a843440cbe6c9c6cbc1bf513c598eb",
				"can_delete": true
			  }`)
		})
		defer srv.Close()

		bi, err := c.GetRefInfo(ctx, &gerritpb.RefInfoRequest{
			Project: "infra/experimental",
			Ref:     "refs/heads/main",
		})
		So(err, ShouldBeNil)

		So(actualURL.Path, ShouldEqual, "/projects/infra/experimental/branches/refs/heads/main")
		So(bi, ShouldResemble, &gerritpb.RefInfo{
			Ref:      "refs/heads/main",
			Revision: "10e5c33f63a843440cbe6c9c6cbc1bf513c598eb",
		})
	})
}

func newMockPbClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, gerritpb.GerritClient) {
	// TODO(tandrii): rename this func once newMockClient name is no longer used in the same package.
	srv := httptest.NewServer(http.HandlerFunc(handler))
	return srv, &client{BaseURL: srv.URL}
}

// parseTime parses a RFC3339Nano formatted timestamp string.
// Panics when error occurs during parse.
func parseTime(t string) time.Time {
	ret, err := time.Parse(time.RFC3339Nano, t)
	if err != nil {
		panic(err)
	}
	return ret
}

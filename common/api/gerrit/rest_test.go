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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil"
)

func TestBuildURL(t *testing.T) {
	t.Parallel()

	ftt.Run("buildURL works correctly", t, func(t *ftt.Test) {
		cPB, err := NewRESTClient(nil, "x-review.googlesource.com", true)
		assert.Loosely(t, err, should.BeNil)
		c, ok := cPB.(*client)
		assert.Loosely(t, ok, should.BeTrue)

		assert.Loosely(t, c.buildURL("/changes/project~123", nil, nil), should.Match(
			"https://x-review.googlesource.com/a/changes/project~123"))
		assert.Loosely(t, c.buildURL("/changes/project~123", url.Values{"o": []string{"ONE", "TWO"}}, nil), should.Match(
			"https://x-review.googlesource.com/a/changes/project~123?o=ONE&o=TWO"))

		opt := UseGerritMirror(func(host string) string { return "mirror-" + host })
		assert.Loosely(t, c.buildURL("/changes/project~123", nil, []grpc.CallOption{opt}), should.Match(
			"https://mirror-x-review.googlesource.com/a/changes/project~123"))

		c.auth = false
		assert.Loosely(t, c.buildURL("/path", nil, nil), should.Match(
			"https://x-review.googlesource.com/path"))
	})
}

func TestListAccountEmails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("ListAccountEmails", t, func(t *ftt.Test) {
		t.Run("Validates empty email", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {})
			defer srv.Close()

			_, err := c.ListAccountEmails(ctx, &gerritpb.ListAccountEmailsRequest{
				Email: "",
			})

			assert.Loosely(t, err, should.ErrLike("The email field must be present"))
		})

		t.Run("Returns emails API response", func(t *ftt.Test) {
			expectedResponse := &gerritpb.ListAccountEmailsResponse{
				Emails: []*gerritpb.EmailInfo{
					{Email: "foo@google.com", Preferred: true},
					{Email: "foo@chromium.org"},
				},
			}

			var actualRequest *http.Request
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				actualRequest = r
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `)]}'[
					{
						"email": "foo@google.com",
						"preferred": true
					},
					{
						"email": "foo@chromium.org"
					}
				]`)
			})
			defer srv.Close()

			res, err := c.ListAccountEmails(ctx, &gerritpb.ListAccountEmailsRequest{
				Email: "foo@google.com",
			})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(expectedResponse))
			assert.Loosely(t, actualRequest.RequestURI, should.ContainSubstring("/accounts/foo@google.com/emails"))
		})

		t.Run("escape + in email", func(t *ftt.Test) {
			var actualRequest *http.Request
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				actualRequest = r
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `)]}'[
					{
						"email": "foo+review@google.com",
						"preferred": true
					}
				]`)
			})
			defer srv.Close()
			_, err := c.ListAccountEmails(ctx, &gerritpb.ListAccountEmailsRequest{
				Email: "foo+review@google.com",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRequest.RequestURI, should.ContainSubstring("/accounts/foo%2Breview@google.com/emails"))
		})
	})
}

func TestListChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("ListChanges", t, func(t *ftt.Test) {
		t.Run("Validates Limit number", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {})
			defer srv.Close()

			_, err := c.ListChanges(ctx, &gerritpb.ListChangesRequest{
				Query: "label:Commit-Queue",
				Limit: -1,
			})
			assert.Loosely(t, err, should.ErrLike("must be nonnegative"))

			_, err = c.ListChanges(ctx, &gerritpb.ListChangesRequest{
				Query: "label:Commit-Queue",
				Limit: 1001,
			})
			assert.Loosely(t, err, should.ErrLike("should be at most"))
		})

		req := &gerritpb.ListChangesRequest{
			Query: "label:Code-Review",
			Limit: 1,
		}

		t.Run("OK case with one change, _more_changes set in response", func(t *ftt.Test) {
			expectedResponse := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{
					{
						Number: 1,
						Owner: &gerritpb.AccountInfo{
							AccountId: 1000096,
							Name:      "John Doe",
							Email:     "jdoe@example.com",
							Username:  "jdoe",
						},
						Status:    gerritpb.ChangeStatus_MERGED,
						Project:   "example/repo",
						Ref:       "refs/heads/master",
						Created:   timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
						Updated:   timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
						Submitted: timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
						Branch:    "master",
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
							"_account_id":      1000096,
							"name":             "John Doe",
							"email":            "jdoe@example.com",
							"username":         "jdoe"
						},
						"status": "MERGED",
						"project": "example/repo",
						"branch":  "master",
						"created":   "2014-05-05 07:15:44.639000000",
						"updated":   "2014-05-05 07:15:44.639000000",
						"submitted": "2014-05-05 07:15:44.639000000",
						"_more_changes": true
					}
				]`)
			})
			defer srv.Close()

			t.Run("Response and request are as expected", func(t *ftt.Test) {
				res, err := c.ListChanges(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(expectedResponse))
				assert.Loosely(t, actualRequest.URL.Query()["q"], should.Resemble([]string{"label:Code-Review"}))
				assert.Loosely(t, actualRequest.URL.Query()["S"], should.Resemble([]string{"0"}))
				assert.Loosely(t, actualRequest.URL.Query()["n"], should.Resemble([]string{"1"}))
			})

			t.Run("Options are included in the request", func(t *ftt.Test) {
				req.Options = append(req.Options, gerritpb.QueryOption_DETAILED_ACCOUNTS, gerritpb.QueryOption_ALL_COMMITS)
				_, err := c.ListChanges(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t,
					actualRequest.URL.Query()["o"],
					should.Resemble(
						[]string{"DETAILED_ACCOUNTS", "ALL_COMMITS"},
					))
			})
		})
	})
}

func TestGetChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("GetChange", t, func(t *ftt.Test) {
		t.Run("Validate args", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {})
			defer srv.Close()

			_, err := c.GetChange(ctx, &gerritpb.GetChangeRequest{})
			assert.Loosely(t, err, should.ErrLike("number must be positive"))
		})

		req := &gerritpb.GetChangeRequest{Number: 1}

		t.Run("OK", func(t *ftt.Test) {
			expectedChange := &gerritpb.ChangeInfo{
				Number: 1,
				Owner: &gerritpb.AccountInfo{
					AccountId:       1000096,
					Name:            "John Doe",
					Email:           "jdoe@example.com",
					SecondaryEmails: []string{"johndoe@chromium.org"},
					Username:        "jdoe",
					Tags:            []string{"SERVICE_USER"},
				},
				Project:         "example/repo",
				Ref:             "refs/heads/master",
				Subject:         "Added new feature",
				Status:          gerritpb.ChangeStatus_NEW,
				CurrentRevision: "deadbeef",
				Submittable:     true,
				IsPrivate:       true,
				MetaRevId:       "cafecafe",
				Hashtags:        []string{"example_tag"},
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Number: 1,
						Kind:   gerritpb.RevisionInfo_REWORK,
						Uploader: &gerritpb.AccountInfo{
							AccountId:       1000096,
							Name:            "John Doe",
							Email:           "jdoe@example.com",
							SecondaryEmails: []string{"johndoe@chromium.org"},
							Username:        "jdoe",
							Tags:            []string{"SERVICE_USER"},
						},
						Ref:         "refs/changes/123",
						Created:     timestamppb.New(parseTime("2016-03-29T17:47:23.751000000Z")),
						Description: "first upload",
						Files: map[string]*gerritpb.FileInfo{
							"go/to/file.go": {
								LinesInserted: 32,
								LinesDeleted:  44,
								SizeDelta:     -567,
								Size:          11984,
							},
						},
						Commit: &gerritpb.CommitInfo{
							Id:      "", // Gerrit doesn't set it, as it duplicates key in revisions map.
							Message: "Title.\n\nBody is here.\n\nChange-Id: I100deadbeef",
							Parents: []*gerritpb.CommitInfo_Parent{
								{Id: "deadbeef00"},
							},
							Author: &gerritpb.GitPersonInfo{
								Name:  "John Doe",
								Email: "jdoe@example.com",
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
					"Commit-Queue": {
						Optional:     true,
						DefaultValue: 0,
						Values:       map[int32]string{0: "Not ready", 1: "Dry run", 2: "Commit"},
						All: []*gerritpb.ApprovalInfo{
							{
								User: &gerritpb.AccountInfo{
									AccountId: 1010101,
									Name:      "Dry Runner",
									Email:     "dry-runner@example.com",
								},
								Value:                1,
								PermittedVotingRange: &gerritpb.VotingRangeInfo{Min: 0, Max: 2},
								Date:                 timestamppb.New(parseTime("2020-12-13T18:32:35.000000000Z")),
							},
						},
					},
				},
				Created:   timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
				Updated:   timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
				Submitted: timestamppb.New(parseTime("0001-01-01T00:00:00.00000000Z")),
				Messages: []*gerritpb.ChangeMessageInfo{
					{
						Id: "YH-egE",
						Author: &gerritpb.AccountInfo{
							AccountId: 1000096,
							Name:      "John Doe",
							Email:     "john.doe@example.com",
							Username:  "jdoe",
						},
						Date:    timestamppb.New(parseTime("2013-03-23T21:34:02.419000000Z")),
						Message: "Patch Set 1:\n\nThis is the first message.",
						Tag:     "autogenerated:gerrit:test",
					},
					{
						Id: "WEEdhU",
						Author: &gerritpb.AccountInfo{
							AccountId: 1000097,
							Name:      "Jane Roe",
							Email:     "jane.roe@example.com",
							Username:  "jroe",
						},
						Date:    timestamppb.New(parseTime("2013-03-23T21:36:52.332000000Z")),
						Message: "Patch Set 1:\n\nThis is the second message.\n\nWith a line break.",
					},
				},
				Requirements: []*gerritpb.Requirement{
					{
						Status:       gerritpb.Requirement_REQUIREMENT_STATUS_OK,
						FallbackText: "nothing more required",
						Type:         "alpha-numer1c-type",
					},
				},
				SubmitRequirements: []*gerritpb.SubmitRequirementResultInfo{
					{
						Name:        "Verified",
						Description: "Submit requirement for the 'Verified' label",
						Status:      gerritpb.SubmitRequirementResultInfo_NOT_APPLICABLE,
						IsLegacy:    false,
						ApplicabilityExpressionResult: &gerritpb.SubmitRequirementExpressionInfo{
							Fulfilled: false,
						},
					},
					{
						Name:        "Code-Owners",
						Description: "Code Owners overrides approval",
						Status:      gerritpb.SubmitRequirementResultInfo_SATISFIED,
						IsLegacy:    false,
						ApplicabilityExpressionResult: &gerritpb.SubmitRequirementExpressionInfo{
							Fulfilled: true,
						},
						SubmittabilityExpressionResult: &gerritpb.SubmitRequirementExpressionInfo{
							Expression:   "has:approval_code-owners",
							Fulfilled:    true,
							PassingAtoms: []string{"has:approval_code-owners"},
							FailingAtoms: []string{},
						},
						OverrideExpressionResult: &gerritpb.SubmitRequirementExpressionInfo{
							Expression:   "label:Owners-Override=+1",
							Fulfilled:    false,
							PassingAtoms: []string{},
							FailingAtoms: []string{"label:Owners-Override=+1"},
						},
					},
					{
						Name:        "Code-Review",
						Description: "Submit requirement for the 'Code-Review' label",
						Status:      gerritpb.SubmitRequirementResultInfo_UNSATISFIED,
						IsLegacy:    false,
						SubmittabilityExpressionResult: &gerritpb.SubmitRequirementExpressionInfo{
							Expression:   "label:Code-Review=MAX,user=non_uploader -label:Code-Review=MIN",
							Fulfilled:    false,
							PassingAtoms: []string{},
							FailingAtoms: []string{
								"label:Code-Review=MAX,user=non_uploader",
								"label:Code-Review=MIN",
							},
						},
						OverrideExpressionResult: &gerritpb.SubmitRequirementExpressionInfo{
							Expression:   "label:Bot-Commit=+1",
							Fulfilled:    false,
							PassingAtoms: []string{},
							FailingAtoms: []string{"label:Bot-Commit=+1"},
						},
					},
				},
				Reviewers: &gerritpb.ReviewerStatusMap{
					Reviewers: []*gerritpb.AccountInfo{
						{
							AccountId: 1000096,
							Name:      "John Doe",
							Email:     "john.doe@example.com",
							Username:  "jdoe",
						},
						{
							AccountId: 1000097,
							Name:      "Jane Roe",
							Email:     "jane.roe@example.com",
							Username:  "jroe",
						},
					},
				},
				Branch: "master",
			}
			var actualRequest *http.Request
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				actualRequest = r
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `)]}'{
					"_number": 1,
					"status": "NEW",
					"owner": {
						"_account_id":      1000096,
						"name":             "John Doe",
						"email":            "jdoe@example.com",
						"secondary_emails": ["johndoe@chromium.org"],
						"username":         "jdoe",
						"tags":             ["SERVICE_USER"]
					},
					"created":   "2014-05-05 07:15:44.639000000",
					"updated":   "2014-05-05 07:15:44.639000000",
					"project": "example/repo",
					"branch":  "master",
					"current_revision": "deadbeef",
					"submittable": true,
					"is_private": true,
					"meta_rev_id": "cafecafe",
					"hashtags": ["example_tag"],
					"subject": "Added new feature",
					"revisions": {
						"deadbeef": {
							"_number": 1,
							"kind": "REWORK",
							"ref": "refs/changes/123",
							"uploader": {
								"_account_id":      1000096,
								"name":             "John Doe",
								"email":            "jdoe@example.com",
								"secondary_emails": ["johndoe@chromium.org"],
								"username":         "jdoe",
								"tags":             ["SERVICE_USER"]
							},
							"created": "2016-03-29 17:47:23.751000000",
							"description": "first upload",
							"files": {
								"go/to/file.go": {
									"lines_inserted": 32,
									"lines_deleted": 44,
									"size_delta": -567,
									"size": 11984
								}
							},
							"commit": {
								"parents": [{"commit": "deadbeef00"}],
								"author": {
									"name": "John Doe",
									"email": "jdoe@example.com",
									"date": "2014-05-05 07:15:44.639000000",
									"tz": 60
								},
								"committer": {
									"name": "John Doe",
									"email": "jdoe@example.com",
									"date": "2014-05-05 07:15:44.639000000",
									"tz": 60
								},
								"subject": "Title.",
								"message": "Title.\n\nBody is here.\n\nChange-Id: I100deadbeef"
							}
						}
					},
					"labels": {
						"Code-Review": {
							"approved": {
								"name": "Rubber Stamper",
								"email": "rubberstamper@example.com"
							}
						},
						"Commit-Queue": {
							"all": [
								{
									"value": 1,
									"date": "2020-12-13 18:32:35.000000000",
									"permitted_voting_range": {
										"min": 0,
										"max": 2
									},
									"_account_id": 1010101,
									"name": "Dry Runner",
									"email": "dry-runner@example.com",
									"avatars": [
										{
											"url": "https://example.com/photo.jpg",
											"height": 32
										}
									]
								}
							],
							"values": {
								" 0": "Not ready",
								"+1": "Dry run",
								"+2": "Commit"
							},
							"default_value": 0,
							"optional": true
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
							"_revision_number": 1,
							"tag": "autogenerated:gerrit:test"
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
					],
					"requirements": [
						{
							"status": "OK",
							"fallback_text": "nothing more required",
							"type": "alpha-numer1c-type"
						}
					],
					"submit_requirements": [
						{
							"name": "Verified",
							"description": "Submit requirement for the 'Verified' label",
							"status": "NOT_APPLICABLE",
							"is_legacy": false,
							"applicability_expression_result": {
								"fulfilled": false
							}
						},
						{
							"name": "Code-Owners",
							"description": "Code Owners overrides approval",
							"status": "SATISFIED",
							"is_legacy": false,
							"applicability_expression_result": {
								"fulfilled": true
							},
							"submittability_expression_result": {
								"expression": "has:approval_code-owners",
								"fulfilled": true,
								"passing_atoms": ["has:approval_code-owners"],
								"failing_atoms": []
							},
							"override_expression_result": {
								"expression": "label:Owners-Override=+1",
								"fulfilled": false,
								"passing_atoms": [],
								"failing_atoms": ["label:Owners-Override=+1"]
							}
						},
						{
							"name": "Code-Review",
							"description": "Submit requirement for the 'Code-Review' label",
							"status": "UNSATISFIED",
							"is_legacy": false,
							"submittability_expression_result": {
								"expression": "label:Code-Review=MAX,user=non_uploader -label:Code-Review=MIN",
								"fulfilled": false,
								"passing_atoms": [],
								"failing_atoms": [
									"label:Code-Review=MAX,user=non_uploader",
									"label:Code-Review=MIN"
								]
							},
							"override_expression_result": {
								"expression": "label:Bot-Commit=+1",
								"fulfilled": false,
								"passing_atoms": [],
								"failing_atoms": ["label:Bot-Commit=+1"]
							}
						}
					],
					"reviewers": {
						"REVIEWER": [
							{
								"_account_id": 1000096,
								"name": "John Doe",
								"email": "john.doe@example.com",
								"username": "jdoe"
							},
							{
								"_account_id": 1000097,
								"name": "Jane Roe",
								"email": "jane.roe@example.com",
								"username": "jroe"
							}
						]
					}
				}`)
			})
			defer srv.Close()

			t.Run("Basic", func(t *ftt.Test) {
				res, err := c.GetChange(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(expectedChange))
			})

			t.Run("With project", func(t *ftt.Test) {
				req.Project = "infra/luci"
				res, err := c.GetChange(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(expectedChange))
				assert.Loosely(t, actualRequest.URL.EscapedPath(), should.Equal("/changes/infra%2Fluci~1"))
			})

			t.Run("Options", func(t *ftt.Test) {
				req.Options = append(req.Options, gerritpb.QueryOption_DETAILED_ACCOUNTS, gerritpb.QueryOption_ALL_COMMITS)
				_, err := c.GetChange(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t,
					actualRequest.URL.Query()["o"],
					should.Resemble(
						[]string{"DETAILED_ACCOUNTS", "ALL_COMMITS"},
					))
			})

			t.Run("Meta", func(t *ftt.Test) {
				req.Meta = "deadbeef"
				_, err := c.GetChange(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t,
					actualRequest.URL.Query()["meta"],
					should.Resemble(
						[]string{"deadbeef"},
					))
			})
		})
	})
}

func TestRestCreateChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("CreateChange basic", t, func(t *ftt.Test) {
		var actualBody []byte
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			// ignore errors here, but verify body later.
			actualBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(201)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'`)
			json.NewEncoder(w).Encode(map[string]any{
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.Resemble(&gerritpb.ChangeInfo{
			Number:      1,
			Project:     "example/repo",
			Ref:         "refs/heads/master",
			Status:      gerritpb.ChangeStatus_NEW,
			Submittable: false,
			Created:     timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
			Updated:     timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
			Submitted:   timestamppb.New(parseTime("0001-01-01T00:00:00.00000000Z")),
			Branch:      "master",
		}))

		var ci changeInput
		err = json.Unmarshal(actualBody, &ci)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ci, should.Resemble(changeInput{
			Project:    "example/repo",
			Branch:     "refs/heads/master",
			Subject:    "example subject",
			BaseCommit: "someOpaqueHash",
		}))
	})
}

func TestSubmitRevision(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("SubmitRevision", t, func(t *ftt.Test) {
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'`)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "MERGED",
			})
		})
		defer srv.Close()

		req := &gerritpb.SubmitRevisionRequest{
			Number:     42,
			RevisionId: "someRevision",
			Project:    "someProject",
		}
		res, err := c.SubmitRevision(ctx, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.Resemble(&gerritpb.SubmitInfo{
			Status: gerritpb.ChangeStatus_MERGED,
		}))
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someProject~42/revisions/someRevision/submit"))
	})
}

func TestRestChangeEditFileContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("ChangeEditFileContent basic", t, func(t *ftt.Test) {
		// large enough?
		var actualBody []byte
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			// ignore errors here, but verify body later.
			actualBody, _ = io.ReadAll(r.Body)
			// API returns 204 on success.
			w.WriteHeader(204)
		})
		defer srv.Close()

		_, err := c.ChangeEditFileContent(ctx, &gerritpb.ChangeEditFileContentRequest{
			Number:   42,
			Project:  "some/project",
			FilePath: "some/path+foo",
			Content:  []byte("changed file"),
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualURL.RawPath, should.Equal("/changes/some%2Fproject~42/edit/some%2Fpath%2Bfoo"))
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/some/project~42/edit/some/path+foo"))
		assert.Loosely(t, actualBody, should.Resemble([]byte("changed file")))
	})
}

// TODO (yulanlin): Assert body verbatim without decoding
func TestAddReviewer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Add reviewer to cc basic", t, func(t *ftt.Test) {
		var actualURL *url.URL
		var actualBody []byte
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			// ignore the error because body contents will be checked
			actualBody, _ = io.ReadAll(r.Body)
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
							"Code-Review": " 0"
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
		assert.Loosely(t, err, should.BeNil)

		// assert the request was as expected
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someproject~42/reviewers"))
		var body addReviewerRequest
		err = json.Unmarshal(actualBody, &body)
		if err != nil {
			t.Logf("failed to decode req body: %v\n", err)
		}
		assert.Loosely(t, body, should.Resemble(addReviewerRequest{
			Reviewer:  "ccer@test.com",
			State:     "CC",
			Confirmed: true,
			Notify:    "OWNER",
		}))

		// assert the result was as expected
		assert.Loosely(t, res, should.Resemble(&gerritpb.AddReviewerResult{
			Input:     "ccer@test.com",
			Reviewers: []*gerritpb.ReviewerInfo{},
			Ccs: []*gerritpb.ReviewerInfo{
				{
					Account: &gerritpb.AccountInfo{
						Name:      "Reviewer Review",
						AccountId: 10001,
					},
					Approvals: map[string]int32{
						"Code-Review": 0,
					},
				},
			},
		}))
	})
}

func TestDeleteReviewer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Delete reviewer", t, func(t *ftt.Test) {
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			// API returns 204 on success.
			w.WriteHeader(204)
		})
		defer srv.Close()

		_, err := c.DeleteReviewer(ctx, &gerritpb.DeleteReviewerRequest{
			Number:    42,
			Project:   "someproject",
			AccountId: "jdoe@example.com",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someproject~42/reviewers/jdoe@example.com/delete"))
	})
}

func TestSetReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Set Review", t, func(t *ftt.Test) {
		var actualURL *url.URL
		var actualRawBody []byte
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			actualRawBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
			{
				"labels": {
					"Code-Review": -1
				}
			}`)
		})
		defer srv.Close()

		res, err := c.SetReview(ctx, &gerritpb.SetReviewRequest{
			Number:     42,
			Project:    "someproject",
			RevisionId: "somerevision",
			Message:    "This is a message",
			Labels: map[string]int32{
				"Code-Review": -1,
			},
			Tag:    "autogenerated:cq",
			Notify: gerritpb.Notify_NOTIFY_OWNER,
			NotifyDetails: &gerritpb.NotifyDetails{
				Recipients: []*gerritpb.NotifyDetails_Recipient{
					{
						RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
						Info: &gerritpb.NotifyDetails_Info{
							Accounts: []int64{4, 5, 3},
						},
					},
					{
						RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
						Info: &gerritpb.NotifyDetails_Info{
							Accounts: []int64{2, 3, 1},
							// 3 is overlapping with the first recipient,
						},
					},
					{
						RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_BCC,
						Info: &gerritpb.NotifyDetails_Info{
							Accounts: []int64{6, 1},
						},
					},
				},
			},
			OnBehalfOf: 10001,
			Ready:      true,
			AddToAttentionSet: []*gerritpb.AttentionSetInput{
				{User: "10002", Reason: "passed presubmit"},
			},
			RemoveFromAttentionSet: []*gerritpb.AttentionSetInput{
				{User: "10001", Reason: "passed presubmit"},
			},
			IgnoreAutomaticAttentionSetRules: true,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someproject~42/revisions/somerevision/review"))

		var actualBody, expectedBody map[string]any
		assert.Loosely(t, json.Unmarshal(actualRawBody, &actualBody), should.BeNil)
		assert.Loosely(t, json.Unmarshal([]byte(`{
			"message": "This is a message",
			"labels": {
				"Code-Review": -1
			},
			"tag": "autogenerated:cq",
			"notify": "OWNER",
			"notify_details": {
				"TO":  {"accounts": [1, 2, 3, 4, 5]},
				"BCC": {"accounts": [1, 6]}
			},
			"on_behalf_of": 10001,
			"ready": true,
			"add_to_attention_set": [
				{"user": "10002", "reason": "passed presubmit"}
			],
			"remove_from_attention_set": [
				{"user": "10001", "reason": "passed presubmit"}
			],
			"ignore_automatic_attention_set_rules": true
		}`), &expectedBody), should.BeNil)
		assert.Loosely(t, actualBody, should.Resemble(expectedBody))

		assert.Loosely(t, res, should.Resemble(&gerritpb.ReviewResult{
			Labels: map[string]int32{
				"Code-Review": -1,
			},
		}))
	})

	ftt.Run("Set Review can add reviewers", t, func(t *ftt.Test) {
		var actualURL *url.URL
		var actualRawBody []byte
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			actualRawBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
			{
				"reviewers": {
					"jdoe@example.com": {
						"input": "jdoe@example.com",
						"reviewers": [
							{
								"_account_id": 10001,
								"name": "John Doe",
								"email": "jdoe@example.com",
								"approvals": {
									"Verified": "0",
									"Code-Review": "0"
								}
							}
						]
					},
					"10003": {
						"input": "10003",
						"ccs": [
							{
								"_account_id": 10003,
								"name": "Eve Smith",
								"email": "esmith@example.com",
								"approvals": {
									"Verified": "0",
									"Code-Review": "0"
								}
							}
						]
					}
				}
			}`)
		})
		defer srv.Close()

		res, err := c.SetReview(ctx, &gerritpb.SetReviewRequest{
			Number:     42,
			Project:    "someproject",
			RevisionId: "somerevision",
			Message:    "This is a message",
			Reviewers: []*gerritpb.ReviewerInput{
				{
					Reviewer: "jdoe@example.com",
				},
				{
					Reviewer: "10003",
					State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
				},
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someproject~42/revisions/somerevision/review"))

		var actualBody, expectedBody map[string]any
		assert.Loosely(t, json.Unmarshal(actualRawBody, &actualBody), should.BeNil)
		assert.Loosely(t, json.Unmarshal([]byte(`{
			"message": "This is a message",
			"reviewers": [
				{"reviewer": "jdoe@example.com"},
				{"reviewer": "10003", "state": "CC"}
			]
		}`), &expectedBody), should.BeNil)
		assert.Loosely(t, actualBody, should.Resemble(expectedBody))

		assert.Loosely(t, res, should.Resemble(&gerritpb.ReviewResult{
			Reviewers: map[string]*gerritpb.AddReviewerResult{
				"jdoe@example.com": {
					Input: "jdoe@example.com",
					Reviewers: []*gerritpb.ReviewerInfo{
						{
							Account: &gerritpb.AccountInfo{
								Name:      "John Doe",
								Email:     "jdoe@example.com",
								AccountId: 10001,
							},
							Approvals: map[string]int32{
								"Verified":    0,
								"Code-Review": 0,
							},
						},
					},
				},
				"10003": {
					Input: "10003",
					Ccs: []*gerritpb.ReviewerInfo{
						{
							Account: &gerritpb.AccountInfo{
								Name:      "Eve Smith",
								Email:     "esmith@example.com",
								AccountId: 10003,
							},
							Approvals: map[string]int32{
								"Verified":    0,
								"Code-Review": 0,
							},
						},
					},
				},
			},
		}))
	})
}

func TestAddToAttentionSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Add to attention set", t, func(t *ftt.Test) {
		var actualURL *url.URL
		var actualBody []byte
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			// ignore the error because body contents will be checked
			actualBody, _ = io.ReadAll(r.Body)
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
			Input: &gerritpb.AttentionSetInput{
				User:   "fyi@test.com",
				Reason: "For awareness",
				Notify: gerritpb.Notify_NOTIFY_ALL,
			},
		}
		res, err := c.AddToAttentionSet(ctx, req)
		assert.Loosely(t, err, should.BeNil)

		// assert the request was as expected
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someproject~42/attention"))
		expectedBody, err := json.Marshal(attentionSetInput{
			User:   "fyi@test.com",
			Reason: "For awareness",
			Notify: "ALL",
		})
		if err != nil {
			t.Logf("failed to encode expected body: %v\n", err)
		}
		assert.Loosely(t, actualBody, should.Resemble(expectedBody))

		// assert the result was as expected
		assert.Loosely(t, res, should.Resemble(&gerritpb.AccountInfo{
			AccountId: 10001,
			Name:      "FYI reviewer",
			Email:     "fyi@test.com",
			Username:  "fyi",
		}))
	})
}

func TestRevertChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("RevertChange", t, func(t *ftt.Test) {
		t.Run("Validate args", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {})
			defer srv.Close()

			_, err := c.RevertChange(ctx, &gerritpb.RevertChangeRequest{})
			assert.Loosely(t, err, should.ErrLike("number must be positive"))
		})

		t.Run("OK", func(t *ftt.Test) {
			req := &gerritpb.RevertChangeRequest{
				Number:  3964,
				Message: "This is the message added to the revert CL.",
			}

			expectedChange := &gerritpb.ChangeInfo{
				Number: 3965,
				Owner: &gerritpb.AccountInfo{
					AccountId:       1000096,
					Name:            "John Doe",
					Email:           "jdoe@example.com",
					SecondaryEmails: []string{"johndoe@chromium.org"},
					Username:        "jdoe",
					Tags:            []string{"SERVICE_USER"},
				},
				Project:   "example/repo",
				Ref:       "refs/heads/master",
				Status:    gerritpb.ChangeStatus_NEW,
				Created:   timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
				Updated:   timestamppb.New(parseTime("2014-05-05T07:15:44.639000000Z")),
				Submitted: timestamppb.New(parseTime("0001-01-01T00:00:00.00000000Z")),
				Messages: []*gerritpb.ChangeMessageInfo{
					{
						Id: "YH-egE",
						Author: &gerritpb.AccountInfo{
							AccountId: 1000096,
							Name:      "John Doe",
							Email:     "john.doe@example.com",
							Username:  "jdoe",
						},
						Date:    timestamppb.New(parseTime("2013-03-23T21:34:02.419000000Z")),
						Message: "Patch Set 1:\n\nThis is the message added to the revert CL.",
					},
				},
				Branch: "master",
			}

			var actualRequest *http.Request
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				actualRequest = r
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `)]}'{
					"_number": 3965,
					"status": "NEW",
					"owner": {
						"_account_id":      1000096,
						"name":             "John Doe",
						"email":            "jdoe@example.com",
						"secondary_emails": ["johndoe@chromium.org"],
						"username":         "jdoe",
						"tags":             ["SERVICE_USER"]
					},
					"created":   "2014-05-05 07:15:44.639000000",
					"updated":   "2014-05-05 07:15:44.639000000",
					"project": "example/repo",
					"branch":  "master",
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
							"message": "Patch Set 1:\n\nThis is the message added to the revert CL.",
							"_revision_number": 1
						}
					]
				}`)
			})
			defer srv.Close()

			t.Run("Basic", func(t *ftt.Test) {
				res, err := c.RevertChange(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(expectedChange))
				assert.Loosely(t, actualRequest.URL.EscapedPath(), should.Equal("/changes/3964/revert"))
			})

			t.Run("With project", func(t *ftt.Test) {
				req.Project = "infra/luci"
				res, err := c.RevertChange(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(expectedChange))
				assert.Loosely(t, actualRequest.URL.EscapedPath(), should.Equal("/changes/infra%2Fluci~3964/revert"))
			})
		})
	})
}

func TestGetMergeable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("GetMergeable basic", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someproject~42/revisions/somerevision/mergeable"))
		assert.Loosely(t, mi, should.Resemble(&gerritpb.MergeableInfo{
			SubmitType:    gerritpb.MergeableInfo_CHERRY_PICK,
			Strategy:      gerritpb.MergeableStrategy_SIMPLE_TWO_WAY_IN_CORE,
			Mergeable:     true,
			CommitMerged:  false,
			ContentMerged: false,
			Conflicts:     []string{"conflict1", "conflict2"},
			MergeableInto: []string{"my_branch_1"},
		}))
	})
}

func TestListFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("ListFiles basic", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someproject~42/revisions/somerevision/files/"))
		assert.Loosely(t, actualURL.Query().Get("parent"), should.Equal("999"))
		assert.Loosely(t, mi, should.Resemble(&gerritpb.ListFilesResponse{
			Files: map[string]*gerritpb.FileInfo{
				"gerrit-server/src/main/java/com/google/gerrit/server/project/RefControl.java": {
					LinesInserted: 123456,
				},
				"file2": {
					Size: 7,
				},
			},
		}))
	})
}

func TestGetRelatedChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("GetRelatedChanges works", t, func(t *ftt.Test) {
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			// Taken from
			// https://chromium-review.googlesource.com/changes/playground%2Fgerrit-cq~1563638/revisions/2/related
			fmt.Fprint(w, `)]}'
				{
				  "changes": [
				    {
				      "project": "playground/gerrit-cq",
				      "change_id": "If00fa4f207440d7f12fbfff8c05afa9077ab0c21",
				      "commit": {
				        "commit": "4d048b016cb4df4d5d2805f0d3d1042cb1d80671",
				        "parents": [
				          {
				            "commit": "cd7db096c014399c369ddddd319708c3f46752f5"
				          }
				        ],
				        "author": {
				          "name": "Andrii Shyshkalov",
				          "email": "tandrii@chromium.org",
				          "date": "2019-04-11 06:41:01.000000000",
				          "tz": -420
				        },
				        "subject": "p3 change"
				      },
				      "_change_number": 1563639,
				      "_revision_number": 1,
				      "_current_revision_number": 1,
				      "status": "NEW"
				    },
				    {
				      "project": "playground/gerrit-cq",
				      "change_id": "I80bf05eb9124dc126490820ec192c77a24938622",
				      "commit": {
				        "commit": "bce1f3beea01b8b282001b01bd9ea442730d578e",
				        "parents": [
				          {
				            "commit": "fdd1f6d3875e68c99303ebfb25dd5d097e91c83f"
				          }
				        ],
				        "author": {
				          "name": "Andrii Shyshkalov",
				          "email": "tandrii@chromium.org",
				          "date": "2019-04-11 06:40:28.000000000",
				          "tz": -420
				        },
				        "subject": "p2 change"
				      },
				      "_change_number": 1563638,
				      "_revision_number": 2,
				      "_current_revision_number": 2,
				      "status": "NEW"
				    },
				    {
				      "project": "playground/gerrit-cq",
				      "change_id": "Icf12c110abc0cbc0c7d01a40dc047683634a62d7",
				      "commit": {
				        "commit": "fdd1f6d3875e68c99303ebfb25dd5d097e91c83f",
				        "parents": [
				          {
				            "commit": "f8e5384ee591cd5105113098d24c60e750b6c4f6"
				          }
				        ],
				        "author": {
				          "name": "Andrii Shyshkalov",
				          "email": "tandrii@chromium.org",
				          "date": "2019-04-11 06:40:18.000000000",
				          "tz": -420
				        },
				        "subject": "p1 change"
				      },
				      "_change_number": 1563637,
				      "_revision_number": 1,
				      "_current_revision_number": 1,
				      "status": "NEW"
				    }
				  ]
				}
			`)
		})
		defer srv.Close()

		rcs, err := c.GetRelatedChanges(ctx, &gerritpb.GetRelatedChangesRequest{
			Number:     1563638,
			Project:    "playground/gerrit-cq",
			RevisionId: "2",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualURL.EscapedPath(), should.Equal("/changes/playground%2Fgerrit-cq~1563638/revisions/2/related"))
		assert.Loosely(t, rcs, should.Resemble(&gerritpb.GetRelatedChangesResponse{
			Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				{
					Project: "playground/gerrit-cq",
					Commit: &gerritpb.CommitInfo{
						Id:      "4d048b016cb4df4d5d2805f0d3d1042cb1d80671",
						Parents: []*gerritpb.CommitInfo_Parent{{Id: "cd7db096c014399c369ddddd319708c3f46752f5"}},
						Author: &gerritpb.GitPersonInfo{
							Name:  "Andrii Shyshkalov",
							Email: "tandrii@chromium.org",
						},
					},
					Number:          1563639,
					Patchset:        1,
					CurrentPatchset: 1,
					Status:          gerritpb.ChangeStatus_NEW,
				},
				{
					Project: "playground/gerrit-cq",
					Commit: &gerritpb.CommitInfo{
						Id:      "bce1f3beea01b8b282001b01bd9ea442730d578e",
						Parents: []*gerritpb.CommitInfo_Parent{{Id: "fdd1f6d3875e68c99303ebfb25dd5d097e91c83f"}},
						Author: &gerritpb.GitPersonInfo{
							Name:  "Andrii Shyshkalov",
							Email: "tandrii@chromium.org",
						},
					},
					Number:          1563638,
					Patchset:        2,
					CurrentPatchset: 2,
					Status:          gerritpb.ChangeStatus_NEW,
				},
				{
					Project: "playground/gerrit-cq",
					Commit: &gerritpb.CommitInfo{
						Id:      "fdd1f6d3875e68c99303ebfb25dd5d097e91c83f",
						Parents: []*gerritpb.CommitInfo_Parent{{Id: "f8e5384ee591cd5105113098d24c60e750b6c4f6"}},
						Author: &gerritpb.GitPersonInfo{
							Name:  "Andrii Shyshkalov",
							Email: "tandrii@chromium.org",
						},
					},
					Number:          1563637,
					Patchset:        1,
					CurrentPatchset: 1,
					Status:          gerritpb.ChangeStatus_NEW,
				},
			},
		}))
	})
}

func TestGetFileOwners(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ftt.Run("Get Owners: ", t, func(t *ftt.Test) {
		t.Run("Details", func(t *ftt.Test) {
			var actualURL *url.URL
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				actualURL = r.URL
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `)]}'
				{"code_owners":[{
					"account":{
						"_account_id":1000096,
						"name":"User Name",
						"email":"user@test.com",
						"avatars":[{"url":"https://test.com/photo.jpg","height":32}]
					}}]}`)
			})
			defer srv.Close()

			resp, err := c.ListFileOwners(ctx, &gerritpb.ListFileOwnersRequest{
				Project: "projectName",
				Ref:     "main",
				Path:    "path/to/file",
				Options: &gerritpb.AccountOptions{
					Details: true,
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualURL.Path, should.Equal("/projects/projectName/branches/main/code_owners/path/to/file"))
			assert.Loosely(t, actualURL.Query().Get("o"), should.Equal("DETAILS"))
			assert.Loosely(t, resp, should.Resemble(&gerritpb.ListOwnersResponse{
				Owners: []*gerritpb.OwnerInfo{
					{
						Account: &gerritpb.AccountInfo{
							AccountId: 1000096,
							Name:      "User Name",
							Email:     "user@test.com",
						},
					},
				},
			}))
		})
		t.Run("All Emails", func(t *ftt.Test) {
			var actualURL *url.URL
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				actualURL = r.URL
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `)]}'
				{"code_owners": [{
					"account": {
						"_account_id": 1000096,
						"email": "test@test.com",
						"secondary_emails": ["alt@test.com"]
					}}]}`)
			})
			defer srv.Close()

			resp, err := c.ListFileOwners(ctx, &gerritpb.ListFileOwnersRequest{
				Project: "projectName",
				Ref:     "main",
				Path:    "path/to/file",
				Options: &gerritpb.AccountOptions{
					AllEmails: true,
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualURL.Path, should.Equal("/projects/projectName/branches/main/code_owners/path/to/file"))
			assert.Loosely(t, actualURL.Query().Get("o"), should.Equal("ALL_EMAILS"))
			assert.Loosely(t, resp, should.Resemble(&gerritpb.ListOwnersResponse{
				Owners: []*gerritpb.OwnerInfo{
					{
						Account: &gerritpb.AccountInfo{
							AccountId:       1000096,
							Email:           "test@test.com",
							SecondaryEmails: []string{"alt@test.com"},
						},
					},
				},
			}))
		})
	})
}

func TestListProjects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("List Projects", t, func(t *ftt.Test) {
		t.Run("...works for a single ref", func(t *ftt.Test) {
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
				Refs: []string{"refs/heads/main"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualURL.Path, should.Equal("/projects/"))
			assert.Loosely(t, actualURL.Query().Get("b"), should.Equal("refs/heads/main"))
			assert.Loosely(t, projects, should.Resemble(&gerritpb.ListProjectsResponse{
				Projects: map[string]*gerritpb.ProjectInfo{
					"android_apks": {
						Name:  "android_apks",
						State: gerritpb.ProjectInfo_PROJECT_STATE_ACTIVE,
						Refs: map[string]string{
							"refs/heads/main": "82264ea131fcc2a386b83e38b962b370315c7c93",
						},
						WebLinks: []*gerritpb.WebLinkInfo{
							{
								Name: "gitiles",
								Url:  "https://chromium.googlesource.com/android_apks/",
							},
						},
					},
				},
			}))
		})

		t.Run("...works for multiple refs", func(t *ftt.Test) {
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
						"main": "82264ea131fcc2a386b83e38b962b370315c7c93",
						"master": "82264ea131fcc2a386b83e38b962b370315c7c93"
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
				Refs: []string{"refs/heads/main", "refs/heads/master"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualURL.Path, should.Equal("/projects/"))
			assert.Loosely(t, actualURL.Query()["b"], should.Resemble([]string{"refs/heads/main", "refs/heads/master"}))
			assert.Loosely(t, projects, should.Resemble(&gerritpb.ListProjectsResponse{
				Projects: map[string]*gerritpb.ProjectInfo{
					"android_apks": {
						Name:  "android_apks",
						State: gerritpb.ProjectInfo_PROJECT_STATE_ACTIVE,
						Refs: map[string]string{
							"refs/heads/main":   "82264ea131fcc2a386b83e38b962b370315c7c93",
							"refs/heads/master": "82264ea131fcc2a386b83e38b962b370315c7c93",
						},
						WebLinks: []*gerritpb.WebLinkInfo{
							{
								Name: "gitiles",
								Url:  "https://chromium.googlesource.com/android_apks/",
							},
						},
					},
				},
			}))
		})
	})
}

func TestGetBranchInfo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Get Branch Info", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, actualURL.Path, should.Equal("/projects/infra/experimental/branches/refs/heads/main"))
		assert.Loosely(t, bi, should.Resemble(&gerritpb.RefInfo{
			Ref:      "refs/heads/main",
			Revision: "10e5c33f63a843440cbe6c9c6cbc1bf513c598eb",
		}))
	})
}

func TestGetPureRevert(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Get Pure Revert", t, func(t *ftt.Test) {
		var actualURL *url.URL
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualURL = r.URL
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `)]}'
			{
				"is_pure_revert" : false
			}`)
		})
		defer srv.Close()

		req := &gerritpb.GetPureRevertRequest{
			Number:  42,
			Project: "someproject",
		}
		res, err := c.GetPureRevert(ctx, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualURL.Path, should.Equal("/changes/someproject~42/pure_revert"))
		assert.Loosely(t, res, should.Resemble(&gerritpb.PureRevertInfo{
			IsPureRevert: false,
		}))
	})
}

func TestGerritError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Gerrit returns", t, func(t *ftt.Test) {
		// All APIs share the same error handling code path, so use SubmitChange as
		// an example.
		req := &gerritpb.SubmitChangeRequest{Number: 1}
		t.Run("HTTP 400", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(400)
				w.Header().Set("Content-Type", "text/plain")
				w.Write([]byte("invalid request: xyz is required"))
			})
			defer srv.Close()
			_, err := c.SubmitChange(ctx, req)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
		})
		t.Run("HTTP 403", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(403)
			})
			defer srv.Close()
			_, err := c.SubmitChange(ctx, req)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
		})
		t.Run("HTTP 404 ", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
			})
			defer srv.Close()
			_, err := c.SubmitChange(ctx, req)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
		})
		t.Run("HTTP 409 ", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(409)
				w.Header().Set("Content-Type", "text/plain")
				w.Write([]byte("block by Verified"))
			})
			defer srv.Close()
			_, err := c.SubmitChange(ctx, req)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("block by Verified"))
		})
		t.Run("HTTP 412 ", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(412)
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte("precondition failed"))
			})
			defer srv.Close()
			_, err := c.SubmitChange(ctx, req)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("precondition failed"))
		})
		t.Run("HTTP 429 ", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(429)
			})
			defer srv.Close()
			_, err := c.SubmitChange(ctx, req)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.ResourceExhausted))
		})
		t.Run("HTTP 503 ", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(503)
			})
			defer srv.Close()
			_, err := c.SubmitChange(ctx, req)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.Unavailable))
		})
	})
}

func TestGetMetaDiff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sampleResp := `)]}'{
	  "old_change_info": {
		"_number": 1,
		"project": "example/repo",
		"status": "NEW",
		"branch": "main",
		"attention_set": {
		  "22222": {
			"account": {
			  "_account_id": 22222
			},
			"last_update": "2014-01-02 18:37:10.000000000",
			"reason": "ps#1: CQ dry run succeeded."
		  }
		},
		"removed_from_attention_set": {
		  "11111": {
			"account": {
			  "_account_id": 11111
			},
			"last_update": "2022-05-31 19:02:58.000000000",
			"reason": "<GERRIT_ACCOUNT_11111> replied on the change",
			"reason_account": {
			  "_account_id": 11111
			}
		  }
		},
		"created": "2014-01-01 18:26:55.000000000",
		"updated": "2014-01-01 20:23:59.000000000",
		"meta_rev_id": "cafeefac",
		"owner": {
		  "_account_id": 1234567
		},
		"requirements": [
		  {
			"status": "OK",
			"fallback_text": "Code-Owners",
			"type": "code-owners"
		  }
		],
		"submit_records": [
		  {
			"rule_name": "Code-Owners",
			"status": "OK",
			"requirements": [
			  {
				"status": "OK",
				"fallback_text": "Code-Owners",
				"type": "code-owners"
			  }
			]
		  }
		]
	  },
	  "new_change_info": {
		"_number": 1,
		"project": "example/repo",
		"status": "NEW",
		"branch": "main",
		"attention_set": {
		  "33333": {
			"account": {
			  "_account_id": 33333
			},
			"last_update": "2014-01-02 20:25:28.000000000",
			"reason": "<GERRIT_ACCOUNT_33333> replied on the change",
			"reason_account": {
			  "_account_id": 33333
			}
		  }
		},
		"removed_from_attention_set": {
		  "22222": {
			"account": {
			  "_account_id": 22222
			},
			"last_update": "2014-01-02 20:25:28.000000000",
			"reason": "<GERRIT_ACCOUNT_22222> replied on the change",
			"reason_account": {
			  "_account_id": 22222
			}
		  }
		},
		"created": "2014-01-01 18:26:55.000000000",
		"updated": "2014-01-01 20:25:28.000000000",
		"meta_rev_id": "cafecafe",
		"owner": {
		  "_account_id": 1234567
		},
		"requirements": [
		  {
			"status": "OK",
			"fallback_text": "Code-Owners",
			"type": "code-owners"
		  }
		],
		"submit_records": [
		  {
			"rule_name": "Code-Owners",
			"status": "OK",
			"requirements": [
			  {
				"status": "OK",
				"fallback_text": "Code-Owners",
				"type": "code-owners"
			  }
			]
		  }
		]
	  },
	  "added": {
		"attention_set": {
		  "33333": {
			"account": {
			  "_account_id": 33333
			},
			"last_update": "2022-05-31 20:25:28.000000000",
			"reason": "<GERRIT_ACCOUNT_33333> replied on the change",
			"reason_account": {
			  "_account_id": 33333
			}
		  }
		},
		"removed_from_attention_set": {
		  "22222": {
			"account": {
			  "_account_id": 22222
			},
			"last_update": "2022-05-31 20:25:28.000000000",
			"reason": "<GERRIT_ACCOUNT_22222> replied on the change",
			"reason_account": {
			  "_account_id": 22222
			}
		  }
		},
		"updated": "2014-01-02 20:25:28.000000000",
		"meta_rev_id": "cafecafe"
	  },
	  "removed": {
		"attention_set": {
		  "1147264": {
			"account": {
			  "_account_id": 1147264
			},
			"last_update": "2022-05-31 18:37:10.000000000",
			"reason": "ps#1: CQ dry run succeeded."
		  }
		},
		"removed_from_attention_set": {
		  "22222": {
			"account": {
			  "_account_id": 22222
			},
			"last_update": "2014-01-02 19:02:58.000000000",
			"reason": "<GERRIT_ACCOUNT_22222> replied on the change",
			"reason_account": {
			  "_account_id": 22222
			}
		  }
		},
		"updated": "2014-01-02 20:23:59.000000000",
		"meta_rev_id": "cafeefac"
	  }
	}`

	ftt.Run("GetMetaDiff", t, func(t *ftt.Test) {
		t.Run("Validates args", func(t *ftt.Test) {
			srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {})
			defer srv.Close()
			_, err := c.GetMetaDiff(ctx, &gerritpb.GetMetaDiffRequest{})
			assert.Loosely(t, err, should.ErrLike("number must be positive"))
		})

		var actualRequest *http.Request
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			actualRequest = r
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, sampleResp)
		})
		defer srv.Close()
		req := &gerritpb.GetMetaDiffRequest{Number: 1}

		t.Run("Works", func(t *ftt.Test) {
			resp, err := c.GetMetaDiff(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.OldChangeInfo.Number, should.Equal(1))
			assert.Loosely(t, resp.NewChangeInfo.Number, should.Equal(1))
			assert.Loosely(t, resp.Added.MetaRevId, should.Equal(resp.NewChangeInfo.MetaRevId))
			assert.Loosely(t, resp.Removed.MetaRevId, should.Equal(resp.OldChangeInfo.MetaRevId))
		})

		t.Run("Passes old and meta", func(t *ftt.Test) {
			req.Old = "mehmeh"
			req.Meta = "booboo"
			_, err := c.GetMetaDiff(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRequest.URL.Query()["old"], should.Resemble([]string{"mehmeh"}))
			assert.Loosely(t, actualRequest.URL.Query()["meta"], should.Resemble([]string{"booboo"}))
		})
	})

}

func newMockPbClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, gerritpb.GerritClient) {
	// TODO(tandrii): rename this func once newMockClient name is no longer used in the same package.
	srv := httptest.NewServer(http.HandlerFunc(handler))
	return srv, &client{testBaseURL: srv.URL}
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

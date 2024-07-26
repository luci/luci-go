// Copyright 2024 The LUCI Authors.
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

package rpc

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/internal/status"
	"go.chromium.org/luci/tree_status/internal/testutil"
	pb "go.chromium.org/luci/tree_status/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStatus(t *testing.T) {
	Convey("With a Status server", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-tree-status-access", "googlers"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewTreeStatusServer()
		Convey("GetStatus", func() {
			Convey("Anonymous rejected", func() {
				ctx = fakeAuth().anonymous().setInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "log in")
				So(status, ShouldBeNil)
			})
			Convey("No read access rejected", func() {
				ctx = fakeAuth().setInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "not a member of luci-tree-status-access")
				So(status, ShouldBeNil)
			})
			Convey("Read latest when no status updates returns fallback", func() {
				ctx = fakeAuth().withReadAccess().withAuditAccess().setInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				So(err, ShouldBeNil)
				So(status.Name, ShouldEqual, "trees/chromium/status/fallback")
				So(status.GeneralState, ShouldEqual, pb.GeneralState_OPEN)
				So(status.Message, ShouldEqual, "Tree is open (fallback due to no status updates in past 140 days)")
			})
			Convey("Read latest", func() {
				ctx = fakeAuth().withReadAccess().withAuditAccess().setInContext(ctx)
				NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				So(err, ShouldBeNil)
				So(status, ShouldResembleProto, &pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", latest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "latest",
					CreateUser:   "someone@example.com",
					CreateTime:   timestamppb.New(latest.CreateTime),
				})
			})
			Convey("Read latest with no audit access hides username", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				So(err, ShouldBeNil)
				So(status, ShouldResembleProto, &pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", latest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "latest",
					CreateTime:   timestamppb.New(latest.CreateTime),
				})
			})
			Convey("Read by name", func() {
				ctx = fakeAuth().withReadAccess().withAuditAccess().setInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
				}
				status, err := server.GetStatus(ctx, request)

				So(err, ShouldBeNil)
				So(status, ShouldResembleProto, &pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "earliest",
					CreateUser:   "someone@example.com",
					CreateTime:   timestamppb.New(earliest.CreateTime),
				})
			})
			Convey("Read by name with no audit access hides username", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
				}
				status, err := server.GetStatus(ctx, request)

				So(err, ShouldBeNil)
				So(status, ShouldResembleProto, &pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "earliest",
					CreateTime:   timestamppb.New(earliest.CreateTime),
				})
			})
			Convey("Read of invalid id", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/INVALID",
				}
				_, err := server.GetStatus(ctx, request)

				So(err, ShouldBeRPCInvalidArgument, "name: expected format")
			})
			Convey("Read of non existing valid id", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/abcd1234abcd1234abcd1234abcd1234",
				}
				_, err := server.GetStatus(ctx, request)

				So(err, ShouldBeRPCNotFound, "status value was not found")
			})
		})

		Convey("Create", func() {
			Convey("Anonymous rejected", func() {
				ctx = fakeAuth().anonymous().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "log in")
				So(status, ShouldBeNil)
			})
			Convey("No access rejected", func() {
				ctx = fakeAuth().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "not a member of luci-tree-status-access")
				So(status, ShouldBeNil)
			})
			Convey("No write access rejected", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "you do not have permission to update the tree status")
				So(status, ShouldBeNil)
			})
			Convey("Successful Create", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState:       pb.GeneralState_CLOSED,
						Message:            "closed",
						ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeNil)
				So(actual, ShouldResembleProto, &pb.Status{
					Name:               actual.Name,
					GeneralState:       pb.GeneralState_CLOSED,
					Message:            "closed",
					CreateUser:         "someone@example.com",
					CreateTime:         actual.CreateTime,
					ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
				})
				So(actual.Name, ShouldContainSubstring, "trees/chromium/status/")
				So(time.Since(actual.CreateTime.AsTime()), ShouldBeLessThan, time.Minute)

				// Check it was actually written to the DB.
				s, err := status.ReadLatest(span.Single(ctx), "chromium")
				So(err, ShouldBeNil)
				So(s, ShouldResemble, &status.Status{
					TreeName:           "chromium",
					StatusID:           strings.Split(actual.Name, "/")[3],
					GeneralStatus:      pb.GeneralState_CLOSED,
					Message:            "closed",
					CreateUser:         "someone@example.com",
					CreateTime:         actual.CreateTime.AsTime(),
					ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
				})
			})
			Convey("Closing builder name ignored if status is not closed", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState:       pb.GeneralState_OPEN,
						Message:            "open",
						ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeNil)
				So(actual, ShouldResembleProto, &pb.Status{
					Name:         actual.Name,
					GeneralState: pb.GeneralState_OPEN,
					Message:      "open",
					CreateUser:   "someone@example.com",
					CreateTime:   actual.CreateTime,
				})
				So(actual.Name, ShouldContainSubstring, "trees/chromium/status/")
				So(time.Since(actual.CreateTime.AsTime()), ShouldBeLessThan, time.Minute)

				// Check it was actually written to the DB.
				s, err := status.ReadLatest(span.Single(ctx), "chromium")
				So(err, ShouldBeNil)
				So(s, ShouldResemble, &status.Status{
					TreeName:      "chromium",
					StatusID:      strings.Split(actual.Name, "/")[3],
					GeneralStatus: pb.GeneralState_OPEN,
					Message:       "open",
					CreateUser:    "someone@example.com",
					CreateTime:    actual.CreateTime.AsTime(),
				})
			})
			Convey("Invalid parent", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "chromium",
					Status: &pb.Status{
						GeneralState: pb.GeneralState_CLOSED,
						Message:      "closed",
					},
				}
				_, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeRPCInvalidArgument, "expected format: ^trees/")
			})
			Convey("Name ignored", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						Name:         "incorrect_format_name",
						GeneralState: pb.GeneralState_CLOSED,
						Message:      "closed",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeNil)
				So(actual.Name, ShouldContainSubstring, "trees/chromium/status/")
			})
			Convey("Empty general_state", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						Message: "closed",
					},
				}
				_, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeRPCInvalidArgument, "general_state: must be specified")
			})
			Convey("Empty message", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState: pb.GeneralState_CLOSED,
					},
				}
				_, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeRPCInvalidArgument, "message: must be specified")
			})
			Convey("User ignored", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState: pb.GeneralState_CLOSED,
						Message:      "closed",
						CreateUser:   "someoneelse@somewhere.com",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeNil)
				So(actual.CreateUser, ShouldEqual, "someone@example.com")
			})
			Convey("Time ignored", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState: pb.GeneralState_CLOSED,
						Message:      "closed",
						CreateTime:   timestamppb.New(time.Time{}),
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				So(err, ShouldBeNil)
				So(time.Since(actual.CreateTime.AsTime()), ShouldBeLessThan, time.Minute)
			})

		})

		Convey("List", func() {
			Convey("Anonymous rejected", func() {
				ctx = fakeAuth().anonymous().setInContext(ctx)

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.ListStatus(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "log in")
				So(status, ShouldBeNil)
			})
			Convey("No read access rejected", func() {
				ctx = fakeAuth().setInContext(ctx)

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.ListStatus(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "not a member of luci-tree-status-access")
				So(status, ShouldBeNil)
			})
			Convey("List with no values", func() {
				ctx = fakeAuth().withReadAccess().withAuditAccess().setInContext(ctx)

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				response, err := server.ListStatus(ctx, request)

				So(err, ShouldBeNil)
				So(response.NextPageToken, ShouldBeEmpty)
				So(response.Status, ShouldBeEmpty)
			})
			Convey("List with one page", func() {
				ctx = fakeAuth().withReadAccess().withAuditAccess().setInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.ListStatusRequest{
					Parent:   "trees/chromium/status",
					PageSize: 2,
				}
				response, err := server.ListStatus(ctx, request)

				So(err, ShouldBeNil)
				So(response.NextPageToken, ShouldBeEmpty)
				So(response, ShouldResembleProto, &pb.ListStatusResponse{
					Status: []*pb.Status{
						toStatusProto(latest, true),
						toStatusProto(earliest, true),
					},
				})
			})
			Convey("List with two pages", func() {
				ctx = fakeAuth().withReadAccess().withAuditAccess().setInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.ListStatusRequest{
					Parent:   "trees/chromium/status",
					PageSize: 1,
				}
				response1, err := server.ListStatus(ctx, request)
				So(err, ShouldBeNil)
				request.PageToken = response1.NextPageToken
				response2, err := server.ListStatus(ctx, request)
				So(err, ShouldBeNil)

				So(response1.NextPageToken, ShouldNotBeEmpty)
				So(response2.NextPageToken, ShouldBeEmpty)
				So(response1.Status[0], ShouldResembleProto, toStatusProto(latest, true))
				So(response2.Status[0], ShouldResembleProto, toStatusProto(earliest, true))
			})
		})
		Convey("List with no audit access hides usernames", func() {
			ctx = fakeAuth().withReadAccess().setInContext(ctx)
			expected := toStatusProto(NewStatusBuilder().CreateInDB(ctx), true)
			expected.CreateUser = ""

			request := &pb.ListStatusRequest{
				Parent: "trees/chromium/status",
			}
			response, err := server.ListStatus(ctx, request)

			So(err, ShouldBeNil)
			So(response, ShouldResembleProto, &pb.ListStatusResponse{
				Status: []*pb.Status{expected},
			})
		})
	})

}

type fakeAuthBuilder struct {
	state *authtest.FakeState
}

func fakeAuth() *fakeAuthBuilder {
	return &fakeAuthBuilder{
		state: &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{},
		},
	}
}
func (a *fakeAuthBuilder) anonymous() *fakeAuthBuilder {
	a.state.Identity = "anonymous:anonymous"
	return a
}
func (a *fakeAuthBuilder) withReadAccess() *fakeAuthBuilder {
	a.state.IdentityGroups = append(a.state.IdentityGroups, treeStatusAccessGroup)
	return a
}
func (a *fakeAuthBuilder) withAuditAccess() *fakeAuthBuilder {
	a.state.IdentityGroups = append(a.state.IdentityGroups, treeStatusAuditAccessGroup)
	return a
}
func (a *fakeAuthBuilder) withWriteAccess() *fakeAuthBuilder {
	a.state.IdentityGroups = append(a.state.IdentityGroups, treeStatusWriteAccessGroup)
	return a
}
func (a *fakeAuthBuilder) setInContext(ctx context.Context) context.Context {
	if a.state.Identity == "anonymous:anonymous" && len(a.state.IdentityGroups) > 0 {
		panic("You cannot call any of the with methods on fakeAuthBuilder if you call the anonymous method")
	}
	return auth.WithState(ctx, a.state)
}

type StatusBuilder struct {
	status status.Status
}

func NewStatusBuilder() *StatusBuilder {
	id, err := status.GenerateID()
	So(err, ShouldBeNil)
	return &StatusBuilder{status: status.Status{
		TreeName:      "chromium",
		StatusID:      id,
		GeneralStatus: pb.GeneralState_OPEN,
		Message:       "Tree is open!",
		CreateUser:    "someone@example.com",
		CreateTime:    spanner.CommitTimestamp,
	}}
}

func (b *StatusBuilder) WithMessage(message string) *StatusBuilder {
	b.status.Message = message
	return b
}

func (b *StatusBuilder) Build() *status.Status {
	s := b.status
	return &s
}

func (b *StatusBuilder) CreateInDB(ctx context.Context) *status.Status {
	s := b.Build()
	row := map[string]any{
		"TreeName":      s.TreeName,
		"StatusId":      s.StatusID,
		"GeneralStatus": int64(s.GeneralStatus),
		"Message":       s.Message,
		"CreateUser":    s.CreateUser,
		"CreateTime":    s.CreateTime,
	}
	m := spanner.InsertOrUpdateMap("Status", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	So(err, ShouldBeNil)
	if s.CreateTime == spanner.CommitTimestamp {
		s.CreateTime = ts.UTC()
	}
	return s
}

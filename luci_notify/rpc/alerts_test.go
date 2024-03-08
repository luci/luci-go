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

	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/internal/alerts"
	"go.chromium.org/luci/luci_notify/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAlerts(t *testing.T) {
	Convey("With an Alerts server", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		ctx = fakeAuth().withReadAccess().setInContext(ctx)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewAlertsServer()
		Convey("BatchGetAlerts", func() {
			Convey("Anonymous rejected", func() {
				ctx = fakeAuth().anonymous().setInContext(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/1234"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "log in")
				So(response, ShouldBeNil)
			})
			Convey("No read access rejected", func() {
				ctx = fakeAuth().setInContext(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/1234"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "not a member of luci-notify-access")
				So(response, ShouldBeNil)
			})
			Convey("Read when no info present returns fallback", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/1234"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				So(err, ShouldBeNil)
				So(len(response.Alerts), ShouldEqual, 1)
				So(response.Alerts[0].Name, ShouldEqual, "alerts/1234")
				So(response.Alerts[0].Bug, ShouldEqual, 0)
				So(response.Alerts[0].SilenceUntil, ShouldEqual, 0)
			})
			Convey("Read by name", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				first := NewAlertBuilder().WithAlertKey("1").WithBug(1).CreateInDB(ctx)
				second := NewAlertBuilder().WithAlertKey("2").WithBug(2).CreateInDB(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/1", "alerts/2"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				So(err, ShouldBeNil)
				So(response, ShouldResembleProto, &pb.BatchGetAlertsResponse{
					Alerts: []*pb.Alert{
						{
							Name:         "alerts/1",
							Bug:          1,
							SilenceUntil: 0,
							ModifyTime:   timestamppb.New(first.ModifyTime),
							Etag:         first.Etag(),
						},
						{
							Name:         "alerts/2",
							Bug:          2,
							SilenceUntil: 0,
							ModifyTime:   timestamppb.New(second.ModifyTime),
							Etag:         second.Etag(),
						},
					},
				})
			})
			Convey("Read of invalid id", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/non-existing"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				So(err, ShouldBeNil)
				So(response, ShouldResembleProto, &pb.BatchGetAlertsResponse{
					Alerts: []*pb.Alert{
						{
							Name:         "alerts/non-existing",
							Bug:          0,
							SilenceUntil: 0,
							ModifyTime:   timestamppb.New(time.Time{}),
							Etag:         (&alerts.Alert{}).Etag(),
						},
					},
				})
			})
		})

		Convey("BatchUpdateAlerts", func() {
			Convey("Anonymous rejected", func() {
				ctx = fakeAuth().anonymous().setInContext(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name: "alerts/1",
							},
						}},
				}
				response, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "log in")
				So(response, ShouldBeNil)
			})
			Convey("No access rejected", func() {
				ctx = fakeAuth().setInContext(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name: "alerts/1",
							},
						}},
				}
				response, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "not a member of luci-notify-access")
				So(response, ShouldBeNil)
			})
			Convey("No write access rejected", func() {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name: "alerts/1",
							},
						}},
				}
				response, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "you do not have permission to update alerts")
				So(response, ShouldBeNil)
			})
			Convey("Successful Create", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name:         "alerts/1",
								Bug:          1,
								SilenceUntil: 2,
							},
						}},
				}
				response, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeNil)
				So(response, ShouldResembleProto, &pb.BatchUpdateAlertsResponse{
					Alerts: []*pb.Alert{
						{
							Name:         "alerts/1",
							Bug:          1,
							SilenceUntil: 2,
							ModifyTime:   response.Alerts[0].ModifyTime,
							Etag:         (&alerts.Alert{Bug: 1, SilenceUntil: 2}).Etag(),
						},
					},
				})
				So(time.Since(response.Alerts[0].ModifyTime.AsTime()), ShouldBeLessThan, time.Minute)

				// Check it was actually written to the DB.
				s, err := alerts.ReadBatch(span.Single(ctx), []string{"1"})
				So(err, ShouldBeNil)
				So(s, ShouldResemble, []*alerts.Alert{{
					AlertKey:     "1",
					Bug:          1,
					SilenceUntil: 2,
					ModifyTime:   response.Alerts[0].ModifyTime.AsTime(),
				}})
			})
			Convey("Invalid Name", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name: "alerts/1/2",
								Bug:  1,
							},
						}},
				}
				_, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeRPCInvalidArgument, "name: expected format:")
			})
			Convey("invalid bug", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name: "alerts/1",
								Bug:  -1,
							},
						}},
				}
				_, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeRPCInvalidArgument, "bug: must be zero or positive")
			})
			Convey("invalid silence_until", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name:         "alerts/1",
								SilenceUntil: -1,
							},
						}},
				}
				_, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeRPCInvalidArgument, "silence_until: must be zero or positive")
			})
			Convey("Time ignored", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				oldTime, err := time.Parse(time.RFC3339, "2000-01-01T13:57:02Z")
				So(err, ShouldBeNil)
				oldTimePB := timestamppb.New(oldTime)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name:       "alerts/1",
								Bug:        1,
								ModifyTime: oldTimePB,
							},
						}},
				}
				response, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeNil)
				So(time.Since(response.Alerts[0].ModifyTime.AsTime()), ShouldBeLessThan, time.Minute)
			})
			Convey("With correct Etag", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)
				alert := NewAlertBuilder().WithAlertKey("1").WithBug(1).CreateInDB(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name: "alerts/1",
								Bug:  2,
								Etag: alert.Etag(),
							},
						}},
				}
				response, err := server.BatchUpdateAlerts(ctx, request)

				So(err, ShouldBeNil)
				So(response.Alerts[0].Bug, ShouldEqual, 2)
			})
			Convey("With wrong Etag", func() {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)
				_ = NewAlertBuilder().WithAlertKey("1").WithBug(1).CreateInDB(ctx)

				request := &pb.BatchUpdateAlertsRequest{
					Requests: []*pb.UpdateAlertRequest{
						{
							Alert: &pb.Alert{
								Name: "alerts/1",
								Bug:  2,
								Etag: "W/\"1234\"",
							},
						}},
				}
				_, err := server.BatchUpdateAlerts(ctx, request)
				So(err, ShouldBeRPCAborted, "etag")
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
	a.state.IdentityGroups = append(a.state.IdentityGroups, luciNotifyAccessGroup)
	return a
}
func (a *fakeAuthBuilder) withWriteAccess() *fakeAuthBuilder {
	a.state.IdentityGroups = append(a.state.IdentityGroups, luciNotifyWriteAccessGroup)
	return a
}
func (a *fakeAuthBuilder) setInContext(ctx context.Context) context.Context {
	if a.state.Identity == "anonymous:anonymous" && len(a.state.IdentityGroups) > 0 {
		panic("You cannot call any of the with methods on fakeAuthBuilder if you call the anonymous method")
	}
	return auth.WithState(ctx, a.state)
}

type AlertBuilder struct {
	alert alerts.Alert
}

func NewAlertBuilder() *AlertBuilder {
	return &AlertBuilder{alert: alerts.Alert{
		AlertKey:     "1234",
		Bug:          0,
		SilenceUntil: 0,
		ModifyTime:   spanner.CommitTimestamp,
	}}
}

func (b *AlertBuilder) WithAlertKey(key string) *AlertBuilder {
	b.alert.AlertKey = key
	return b
}

func (b *AlertBuilder) WithBug(bug int64) *AlertBuilder {
	b.alert.Bug = bug
	return b
}

func (b *AlertBuilder) WithsilenceUntil(silenceUntil int64) *AlertBuilder {
	b.alert.SilenceUntil = silenceUntil
	return b
}

func (b *AlertBuilder) Build() *alerts.Alert {
	s := b.alert
	return &s
}

func (b *AlertBuilder) CreateInDB(ctx context.Context) *alerts.Alert {
	s := b.Build()
	row := map[string]any{
		"AlertKey":     s.AlertKey,
		"Bug":          s.Bug,
		"SilenceUntil": s.SilenceUntil,
		"ModifyTime":   s.ModifyTime,
	}
	m := spanner.InsertOrUpdateMap("Alerts", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	So(err, ShouldBeNil)
	if s.ModifyTime == spanner.CommitTimestamp {
		s.ModifyTime = ts.UTC()
	}
	return s
}

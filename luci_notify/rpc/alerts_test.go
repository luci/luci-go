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
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/span"

	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/internal/alerts"
	"go.chromium.org/luci/luci_notify/internal/testutil"
)

func TestAlerts(t *testing.T) {
	ftt.Run("With an Alerts server", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		ctx = fakeAuth().withReadAccess().setInContext(ctx)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewAlertsServer()
		t.Run("BatchGetAlerts", func(t *ftt.Test) {
			t.Run("Anonymous rejected", func(t *ftt.Test) {
				ctx = fakeAuth().anonymous().setInContext(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/1234"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("log in"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("No read access rejected", func(t *ftt.Test) {
				ctx = fakeAuth().setInContext(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/1234"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("not a member of luci-notify-access"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Read when no info present returns fallback", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/1234"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(response.Alerts), should.Equal(1))
				assert.Loosely(t, response.Alerts[0].Name, should.Equal("alerts/1234"))
				assert.Loosely(t, response.Alerts[0].Bug, should.BeZero)
				assert.Loosely(t, response.Alerts[0].SilenceUntil, should.BeZero)
			})
			t.Run("Read by name", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				first := NewAlertBuilder().WithAlertKey("1").WithBug(1).CreateInDB(ctx, t)
				second := NewAlertBuilder().WithAlertKey("2").WithBug(2).CreateInDB(ctx, t)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/1", "alerts/2"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.BatchGetAlertsResponse{
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
				}))
			})
			t.Run("Read of invalid id", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)

				request := &pb.BatchGetAlertsRequest{
					Names: []string{"alerts/non-existing"},
				}
				response, err := server.BatchGetAlerts(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.BatchGetAlertsResponse{
					Alerts: []*pb.Alert{
						{
							Name:         "alerts/non-existing",
							Bug:          0,
							SilenceUntil: 0,
							ModifyTime:   timestamppb.New(time.Time{}),
							Etag:         (&alerts.Alert{}).Etag(),
						},
					},
				}))
			})
		})

		t.Run("BatchUpdateAlerts", func(t *ftt.Test) {
			t.Run("Anonymous rejected", func(t *ftt.Test) {
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

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("log in"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("No access rejected", func(t *ftt.Test) {
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

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("not a member of luci-notify-access"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("No write access rejected", func(t *ftt.Test) {
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

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("you do not have permission to update alerts"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Successful Create", func(t *ftt.Test) {
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

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.BatchUpdateAlertsResponse{
					Alerts: []*pb.Alert{
						{
							Name:         "alerts/1",
							Bug:          1,
							SilenceUntil: 2,
							ModifyTime:   response.Alerts[0].ModifyTime,
							Etag:         (&alerts.Alert{Bug: 1, SilenceUntil: 2}).Etag(),
						},
					},
				}))
				assert.Loosely(t, time.Since(response.Alerts[0].ModifyTime.AsTime()), should.BeLessThan(time.Minute))

				// Check it was actually written to the DB.
				s, err := alerts.ReadBatch(span.Single(ctx), []string{"1"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Match([]*alerts.Alert{{
					AlertKey:     "1",
					Bug:          1,
					SilenceUntil: 2,
					ModifyTime:   response.Alerts[0].ModifyTime.AsTime(),
				}}))
			})
			t.Run("Invalid Name", func(t *ftt.Test) {
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

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("name: expected format:"))
			})
			t.Run("invalid bug", func(t *ftt.Test) {
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

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bug: must be zero or positive"))
			})
			t.Run("invalid silence_until", func(t *ftt.Test) {
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

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("silence_until: must be zero or positive"))
			})
			t.Run("Time ignored", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)

				oldTime, err := time.Parse(time.RFC3339, "2000-01-01T13:57:02Z")
				assert.Loosely(t, err, should.BeNil)
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

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, time.Since(response.Alerts[0].ModifyTime.AsTime()), should.BeLessThan(time.Minute))
			})
			t.Run("With correct Etag", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)
				alert := NewAlertBuilder().WithAlertKey("1").WithBug(1).CreateInDB(ctx, t)

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

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response.Alerts[0].Bug, should.Equal(2))
			})
			t.Run("With wrong Etag", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().withWriteAccess().setInContext(ctx)
				_ = NewAlertBuilder().WithAlertKey("1").WithBug(1).CreateInDB(ctx, t)

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
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Aborted))
				assert.Loosely(t, err, should.ErrLike("etag"))
			})
		})
	})
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

func (b *AlertBuilder) CreateInDB(ctx context.Context, t testing.TB) *alerts.Alert {
	s := b.Build()
	row := map[string]any{
		"AlertKey":     s.AlertKey,
		"Bug":          s.Bug,
		"SilenceUntil": s.SilenceUntil,
		"ModifyTime":   s.ModifyTime,
	}
	m := spanner.InsertOrUpdateMap("Alerts", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	if s.ModifyTime.Equal(spanner.CommitTimestamp) {
		s.ModifyTime = ts.UTC()
	}
	return s
}

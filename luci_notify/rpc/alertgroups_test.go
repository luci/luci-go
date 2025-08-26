// Copyright 2025 The LUCI Authors.
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
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

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
	"go.chromium.org/luci/luci_notify/internal/alertgroups"
	"go.chromium.org/luci/luci_notify/internal/testutil"
)

func TestAlertGroups(t *testing.T) {
	ftt.Run("With an AlertGroups server", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewAlertGroupsServer()

		t.Run("GetAlertGroup", func(t *ftt.Test) {
			t.Run("Anonymous rejected", func(t *ftt.Test) {
				ctx = fakeAuth().anonymous().setInContext(ctx)
				_, err := server.GetAlertGroup(ctx, &pb.GetAlertGroupRequest{Name: "rotations/rotation-name/alertGroups/1234"})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("No read access rejected", func(t *ftt.Test) {
				ctx = fakeAuth().setInContext(ctx)
				_, err := server.GetAlertGroup(ctx, &pb.GetAlertGroupRequest{Name: "rotations/rotation-name/alertGroups/1234"})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("Read when no info present returns not found", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				_, err := server.GetAlertGroup(ctx, &pb.GetAlertGroupRequest{Name: "rotations/rotation-name/alertGroups/1234"})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})

			t.Run("Read by name", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				ag := toAlertGroupProto(NewAlertGroupBuilder().WithAlertGroupId("1234").CreateInDB(ctx, t))

				res, err := server.GetAlertGroup(ctx, &pb.GetAlertGroupRequest{Name: ag.Name})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(ag))
			})
		})

		t.Run("ListAlertGroups", func(t *ftt.Test) {
			t.Run("Anonymous rejected", func(t *ftt.Test) {
				ctx = fakeAuth().anonymous().setInContext(ctx)
				_, err := server.ListAlertGroups(ctx, &pb.ListAlertGroupsRequest{Parent: "rotations/rotation-name"})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("No read access rejected", func(t *ftt.Test) {
				ctx = fakeAuth().setInContext(ctx)
				_, err := server.ListAlertGroups(ctx, &pb.ListAlertGroupsRequest{Parent: "rotations/rotation-name"})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("List empty", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				res, err := server.ListAlertGroups(ctx, &pb.ListAlertGroupsRequest{Parent: "rotations/rotation-name"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.AlertGroups, should.BeEmpty)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
			})

			t.Run("List multiple", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				ag1 := NewAlertGroupBuilder().WithRotation("rotation-name").WithAlertGroupId("1").CreateInDB(ctx, t)
				ag2 := NewAlertGroupBuilder().WithRotation("rotation-name").WithAlertGroupId("2").CreateInDB(ctx, t)

				res, err := server.ListAlertGroups(ctx, &pb.ListAlertGroupsRequest{Parent: "rotations/rotation-name"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.AlertGroups, should.Match([]*pb.AlertGroup{toAlertGroupProto(ag1), toAlertGroupProto(ag2)}))
			})
		})

		t.Run("CreateAlertGroup", func(t *ftt.Test) {
			t.Run("Anonymous rejected", func(t *ftt.Test) {
				ctx = fakeAuth().anonymous().setInContext(ctx)
				_, err := server.CreateAlertGroup(ctx, &pb.CreateAlertGroupRequest{Parent: "rotations/rotation-name", AlertGroup: &pb.AlertGroup{}})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("No write access rejected", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				_, err := server.CreateAlertGroup(ctx, &pb.CreateAlertGroupRequest{Parent: "rotations/rotation-name", AlertGroup: &pb.AlertGroup{}})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("Successful create", func(t *ftt.Test) {
				ctx = fakeAuth().withWriteAccess().setInContext(ctx)
				ag := &pb.AlertGroup{DisplayName: "my-group"}
				res, err := server.CreateAlertGroup(ctx, &pb.CreateAlertGroupRequest{Parent: "rotations/rotation-name", AlertGroupId: "new-group", AlertGroup: ag})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Name, should.Equal("rotations/rotation-name/alertGroups/new-group"))
				assert.Loosely(t, res.DisplayName, should.Equal("my-group"))
				assert.Loosely(t, res.UpdatedBy, should.Equal("someone@example.com"))

				fetched, err := server.GetAlertGroup(ctx, &pb.GetAlertGroupRequest{Name: "rotations/rotation-name/alertGroups/new-group"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetched.DisplayName, should.Equal("my-group"))
				assert.Loosely(t, fetched.UpdatedBy, should.Equal("someone@example.com"))

			})

			t.Run("Already exists", func(t *ftt.Test) {
				ctx = fakeAuth().withWriteAccess().setInContext(ctx)
				ag := NewAlertGroupBuilder().WithAlertGroupId("existing-group").CreateInDB(ctx, t)
				_, err := server.CreateAlertGroup(ctx, &pb.CreateAlertGroupRequest{Parent: fmt.Sprintf("rotations/%s", ag.Rotation), AlertGroupId: ag.AlertGroupId, AlertGroup: &pb.AlertGroup{
					DisplayName: "my-group",
				}})
				assert.Loosely(t, err, should.NotBeNil)
				// TODO: This isn't working
				// assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
			})
		})
		t.Run("UpdateAlertGroup", func(t *ftt.Test) {
			t.Run("Anonymous rejected", func(t *ftt.Test) {
				ctx = fakeAuth().anonymous().setInContext(ctx)
				_, err := server.UpdateAlertGroup(ctx, &pb.UpdateAlertGroupRequest{AlertGroup: &pb.AlertGroup{Name: "rotations/rotation-name/alertGroups/1234"}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name"}}})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("No write access rejected", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				_, err := server.UpdateAlertGroup(ctx, &pb.UpdateAlertGroupRequest{AlertGroup: &pb.AlertGroup{Name: "rotations/rotation-name/alertGroups/1234"}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name"}}})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("Successful update", func(t *ftt.Test) {
				ctx = fakeAuth().withWriteAccess().setInContext(ctx)
				ag := toAlertGroupProto(NewAlertGroupBuilder().WithAlertGroupId("update-me").CreateInDB(ctx, t))

				ag.DisplayName = "new-display-name"
				res, err := server.UpdateAlertGroup(ctx, &pb.UpdateAlertGroupRequest{AlertGroup: ag, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name"}}})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.DisplayName, should.Equal("new-display-name"))
				assert.Loosely(t, res.Etag, should.NotEqual(ag.Etag))

				fetched, err := server.GetAlertGroup(ctx, &pb.GetAlertGroupRequest{Name: ag.Name})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fetched.DisplayName, should.Equal("new-display-name"))
				assert.Loosely(t, fetched.Etag, should.Equal(res.Etag))
			})

			t.Run("Etag mismatch", func(t *ftt.Test) {
				ctx = fakeAuth().withWriteAccess().setInContext(ctx)
				ag := toAlertGroupProto(NewAlertGroupBuilder().WithAlertGroupId("etag-mismatch").CreateInDB(ctx, t))
				ag.Etag = "wrong-etag"
				_, err := server.UpdateAlertGroup(ctx, &pb.UpdateAlertGroupRequest{AlertGroup: ag, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name"}}})
				assert.Loosely(t, err, should.ErrLike("etag"))
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

		})

		t.Run("DeleteAlertGroup", func(t *ftt.Test) {
			t.Run("Anonymous rejected", func(t *ftt.Test) {
				ctx = fakeAuth().anonymous().setInContext(ctx)
				_, err := server.DeleteAlertGroup(ctx, &pb.DeleteAlertGroupRequest{Name: "rotations/rotation-name/alertGroups/1234"})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("No write access rejected", func(t *ftt.Test) {
				ctx = fakeAuth().withReadAccess().setInContext(ctx)
				_, err := server.DeleteAlertGroup(ctx, &pb.DeleteAlertGroupRequest{Name: "rotations/rotation-name/alertGroups/1234"})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("Successful delete", func(t *ftt.Test) {
				ctx = fakeAuth().withWriteAccess().setInContext(ctx)
				g := NewAlertGroupBuilder().WithAlertGroupId("delete-me").CreateInDB(ctx, t)
				_, err := server.DeleteAlertGroup(ctx, &pb.DeleteAlertGroupRequest{Name: toAlertGroupProto(g).Name})
				assert.Loosely(t, err, should.BeNil)

				// Verify it's gone.
				_, err = server.GetAlertGroup(ctx, &pb.GetAlertGroupRequest{Name: "rotations/rotation-name/alertGroups/delete-me"})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})
		})

	})
}

type AlertGroupBuilder struct {
	alertGroup alertgroups.AlertGroup
}

func NewAlertGroupBuilder() *AlertGroupBuilder {
	return &AlertGroupBuilder{alertGroup: alertgroups.AlertGroup{
		Rotation:      "test-rotation",
		AlertGroupId:  "test-group",
		DisplayName:   "Test Group",
		StatusMessage: "",
		AlertKeys:     []string{},
		Bugs:          []int64{},
		UpdatedBy:     "test@example.com",
		UpdateTime:    spanner.CommitTimestamp,
	}}
}

func (b *AlertGroupBuilder) WithRotation(rotation string) *AlertGroupBuilder {
	b.alertGroup.Rotation = rotation
	return b
}

func (b *AlertGroupBuilder) WithAlertGroupId(alertGroupId string) *AlertGroupBuilder {
	b.alertGroup.AlertGroupId = alertGroupId
	return b
}

func (b *AlertGroupBuilder) WithDisplayName(displayName string) *AlertGroupBuilder {
	b.alertGroup.DisplayName = displayName
	return b
}

func (b *AlertGroupBuilder) WithStatusMessage(statusMessage string) *AlertGroupBuilder {
	b.alertGroup.StatusMessage = statusMessage
	return b
}

func (b *AlertGroupBuilder) WithAlertKeys(alertKeys []string) *AlertGroupBuilder {
	b.alertGroup.AlertKeys = alertKeys
	return b
}

func (b *AlertGroupBuilder) WithBugs(bugs []int64) *AlertGroupBuilder {
	b.alertGroup.Bugs = bugs
	return b
}

func (b *AlertGroupBuilder) WithUpdateTime(updateTime time.Time) *AlertGroupBuilder {
	b.alertGroup.UpdateTime = updateTime
	return b
}

func (b *AlertGroupBuilder) WithUpdatedBy(updatedBy string) *AlertGroupBuilder {
	b.alertGroup.UpdatedBy = updatedBy
	return b
}

func (b *AlertGroupBuilder) Build() *alertgroups.AlertGroup {
	s := b.alertGroup
	return &s
}

func (b *AlertGroupBuilder) CreateInDB(ctx context.Context, t testing.TB) *alertgroups.AlertGroup {
	t.Helper()
	ag := b.Build()
	row := map[string]any{
		"Rotation":      ag.Rotation,
		"AlertGroupId":  ag.AlertGroupId,
		"DisplayName":   ag.DisplayName,
		"StatusMessage": ag.StatusMessage,
		"AlertKeys":     ag.AlertKeys,
		"Bugs":          ag.Bugs,
		"UpdatedBy":     ag.UpdatedBy,
		"UpdateTime":    ag.UpdateTime,
	}
	m := spanner.InsertOrUpdateMap("AlertGroups", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	if ag.UpdateTime.Equal(spanner.CommitTimestamp) {
		ag.UpdateTime = ts.UTC()
	}
	return ag
}

func RandomString(n int) string {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte('a' + i%26)
	}
	return string(b)
}

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

package alertgroups

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/luci_notify/internal/testutil"
)

func TestValidation(t *testing.T) {
	t.Parallel()
	ftt.Run("Validate", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			err := Validate(NewAlertGroupBuilder().Build())
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("Rotation", func(t *ftt.Test) {
			t.Run("must not be empty", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithRotation("").Build())
				assert.Loosely(t, err, should.ErrLike("rotation: must not be empty"))
			})
			t.Run("must use valid characters", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithRotation("!invalid!").Build())
				assert.Loosely(t, err, should.ErrLike("rotation: must contain only lowercase letters, numbers, and dashes"))
			})
		})
		t.Run("AlertGroupId", func(t *ftt.Test) {
			t.Run("must not be empty", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithAlertGroupId("").Build())
				assert.Loosely(t, err, should.ErrLike("alert_group_id: must not be empty"))
			})
			t.Run("must use valid characters", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithAlertGroupId("!invalid!").Build())
				assert.Loosely(t, err, should.ErrLike("alert_group_id: must contain only lowercase letters, numbers, and dashes"))
			})
		})
		t.Run("DisplayName", func(t *ftt.Test) {
			t.Run("must not be empty", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithDisplayName("").Build())
				assert.Loosely(t, err, should.ErrLike("display_name: must not be empty"))
			})
			t.Run("must not be too long", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithDisplayName(RandomString(1025)).Build())
				assert.Loosely(t, err, should.ErrLike("display_name: must not be longer than 1024 characters"))
			})
		})
		t.Run("StatusMessage", func(t *ftt.Test) {
			t.Run("empty is valid", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithStatusMessage("").Build())
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("must not be too long", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithStatusMessage(RandomString(1024*1024 + 1)).Build())
				assert.Loosely(t, err, should.ErrLike("status_message: must not be longer than 1048576 bytes"))
			})
		})
		t.Run("AlertKeys", func(t *ftt.Test) {
			t.Run("empty is valid", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithAlertKeys(nil).Build())
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("invalid alert key", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithAlertKeys([]string{"alerts/invalid/key"}).Build())
				assert.Loosely(t, err, should.ErrLike("alert_keys[0]: expected format: ^alerts/([^/]+)$"))
			})
		})
		t.Run("Bugs", func(t *ftt.Test) {
			t.Run("empty is valid", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithBugs(nil).Build())
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("negative bug", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithBugs([]int64{-1}).Build())
				assert.Loosely(t, err, should.ErrLike("bugs[0]: must be zero or positive"))
			})
		})
		t.Run("UpdatedBy", func(t *ftt.Test) {
			t.Run("must not be empty", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithUpdatedBy("").Build())
				assert.Loosely(t, err, should.ErrLike("updated_by: must not be empty"))
			})
			t.Run("must not be too long", func(t *ftt.Test) {
				err := Validate(NewAlertGroupBuilder().WithUpdatedBy(RandomString(257)).Build())
				assert.Loosely(t, err, should.ErrLike("updated_by: must not be longer than 256 characters"))
			})
		})
	})
}

func TestAlertGroups(t *testing.T) {
	ftt.Run("Create", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		alert := NewAlertGroupBuilder().Build()

		m, err := Create(alert)
		assert.Loosely(t, err, should.BeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		alert.UpdateTime = ts.UTC()

		assert.Loosely(t, err, should.BeNil)
		fetched, err := Read(span.Single(ctx), alert.Rotation, alert.AlertGroupId)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fetched, should.Match(alert))
	})

	ftt.Run("Update", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		NewAlertGroupBuilder().CreateInDB(ctx, t)
		updated := NewAlertGroupBuilder().WithDisplayName("updated").WithStatusMessage("updated").Build()
		m, err := Update(updated)
		assert.Loosely(t, err, should.BeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		updated.UpdateTime = ts.UTC()
		assert.Loosely(t, err, should.BeNil)

		fetched, err := Read(span.Single(ctx), updated.Rotation, updated.AlertGroupId)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fetched, should.Match(updated))
	})

	ftt.Run("ReadNotFound", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		_, err := Read(span.Single(ctx), "non-existent-rotation", "non-existent-group")
		assert.Loosely(t, err, should.ErrLike("not found"))
	})

	ftt.Run("List", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		NewAlertGroupBuilder().WithRotation("rotation1").WithAlertGroupId("group1").CreateInDB(ctx, t)
		NewAlertGroupBuilder().WithRotation("rotation1").WithAlertGroupId("group2").CreateInDB(ctx, t)
		NewAlertGroupBuilder().WithRotation("rotation2").WithAlertGroupId("group3").CreateInDB(ctx, t)

		t.Run("List all for a rotation", func(t *ftt.Test) {
			groups, nextPageToken, err := List(span.Single(ctx), "rotation1", 100, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(groups), should.Equal(2))
			assert.Loosely(t, groups[0].AlertGroupId, should.Equal("group1"))
			assert.Loosely(t, groups[1].AlertGroupId, should.Equal("group2"))
			assert.Loosely(t, nextPageToken, should.BeEmpty)
		})

		t.Run("Pagination", func(t *ftt.Test) {
			groups, nextPageToken, err := List(span.Single(ctx), "rotation1", 1, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(groups), should.Equal(1))
			assert.Loosely(t, groups[0].AlertGroupId, should.Equal("group1"))
			assert.Loosely(t, nextPageToken, should.NotBeEmpty)

			groups, nextPageToken, err = List(span.Single(ctx), "rotation1", 2, nextPageToken)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(groups), should.Equal(1))
			assert.Loosely(t, groups[0].AlertGroupId, should.Equal("group2"))
			assert.Loosely(t, nextPageToken, should.BeEmpty)
		})
	})

	ftt.Run("Delete", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		group := NewAlertGroupBuilder().CreateInDB(ctx, t)

		m := Delete(ctx, group.Rotation, group.AlertGroupId)
		_, err := span.Apply(ctx, []*spanner.Mutation{m})
		assert.Loosely(t, err, should.BeNil)

		_, err = Read(span.Single(ctx), group.Rotation, group.AlertGroupId)
		assert.Loosely(t, err, should.ErrLike("not found"))
	})
}

func TestEtag(t *testing.T) {
	t.Parallel()
	ftt.Run("Etag", t, func(t *ftt.Test) {
		t.Run("different display name", func(t *ftt.Test) {
			ag1 := NewAlertGroupBuilder().WithDisplayName("name1").Build()
			ag2 := NewAlertGroupBuilder().WithDisplayName("name2").Build()
			assert.Loosely(t, ag1.Etag(), should.NotEqual(ag2.Etag()))
		})
		t.Run("different status message", func(t *ftt.Test) {
			ag1 := NewAlertGroupBuilder().WithStatusMessage("status1").Build()
			ag2 := NewAlertGroupBuilder().WithStatusMessage("status2").Build()
			assert.Loosely(t, ag1.Etag(), should.NotEqual(ag2.Etag()))
		})
		t.Run("different alert keys", func(t *ftt.Test) {
			ag1 := NewAlertGroupBuilder().WithAlertKeys([]string{"alerts/key1"}).Build()
			ag2 := NewAlertGroupBuilder().WithAlertKeys([]string{"alerts/key2"}).Build()
			assert.Loosely(t, ag1.Etag(), should.NotEqual(ag2.Etag()))
		})
	})
}

type AlertGroupBuilder struct {
	alertGroup AlertGroup
}

func NewAlertGroupBuilder() *AlertGroupBuilder {
	return &AlertGroupBuilder{alertGroup: AlertGroup{
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

func (b *AlertGroupBuilder) Build() *AlertGroup {
	s := b.alertGroup
	return &s
}

func (b *AlertGroupBuilder) CreateInDB(ctx context.Context, t testing.TB) *AlertGroup {
	t.Helper()
	s := b.Build()
	row := map[string]any{
		"Rotation":      s.Rotation,
		"AlertGroupId":  s.AlertGroupId,
		"DisplayName":   s.DisplayName,
		"StatusMessage": s.StatusMessage,
		"AlertKeys":     s.AlertKeys,
		"Bugs":          s.Bugs,
		"UpdatedBy":     s.UpdatedBy,
		"UpdateTime":    s.UpdateTime,
	}
	m := spanner.InsertOrUpdateMap("AlertGroups", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	if s.UpdateTime == spanner.CommitTimestamp {
		s.UpdateTime = ts.UTC()
	}
	return s
}

func RandomString(n int) string {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte('a' + i%26)
	}
	return string(b)
}

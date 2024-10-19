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

package alerts

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
	ftt.Run("Validate", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			err := Validate(NewAlertBuilder().Build())
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("Bug", func(t *ftt.Test) {
			t.Run("must not be negative", func(t *ftt.Test) {
				err := Validate(NewAlertBuilder().WithBug(-1).Build())
				assert.Loosely(t, err, should.ErrLike("bug: must be zero or positive"))
			})
			t.Run("zero is valid", func(t *ftt.Test) {
				err := Validate(NewAlertBuilder().WithBug(0).Build())
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("GerritCL", func(t *ftt.Test) {
			t.Run("must not be negative", func(t *ftt.Test) {
				err := Validate(NewAlertBuilder().WithGerritCL(-1).Build())
				assert.Loosely(t, err, should.ErrLike("gerrit_cl: must be zero or positive"))
			})
			t.Run("zero is valid", func(t *ftt.Test) {
				err := Validate(NewAlertBuilder().WithGerritCL(0).Build())
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("SilenceUntil", func(t *ftt.Test) {
			t.Run("must not be negative", func(t *ftt.Test) {
				err := Validate(NewAlertBuilder().WithSilenceUntil(-1).Build())
				assert.Loosely(t, err, should.ErrLike("silence_until: must be zero or positive"))
			})
			t.Run("zero is valid", func(t *ftt.Test) {
				err := Validate(NewAlertBuilder().WithSilenceUntil(0).Build())
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestStatusTable(t *testing.T) {
	ftt.Run("Put", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		alert := NewAlertBuilder().WithBug(10).WithGerritCL(15).Build()

		m, err := Put(alert)
		assert.Loosely(t, err, should.BeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		alert.ModifyTime = ts.UTC()

		assert.Loosely(t, err, should.BeNil)
		fetched, err := ReadBatch(span.Single(ctx), []string{alert.AlertKey})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fetched, should.Resemble([]*Alert{alert}))
	})
	ftt.Run("Put deletes when all info is zero", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		alert := NewAlertBuilder().CreateInDB(ctx, t)

		m, err := Put(alert)
		assert.Loosely(t, err, should.BeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		alert.ModifyTime = ts.UTC()

		assert.Loosely(t, err, should.BeNil)
		// When we read a non-existent entry, a fake entry will be returned, so the only
		// way to tell it was actually deleted is a zero timestamp.
		fetched, err := ReadBatch(span.Single(ctx), []string{alert.AlertKey})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(fetched), should.Equal(1))
		assert.Loosely(t, fetched[0].ModifyTime, should.Match(time.Time{}))
	})

	ftt.Run("ReadBatch", t, func(t *ftt.Test) {
		t.Run("Single", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			alert := NewAlertBuilder().CreateInDB(ctx, t)

			fetched, err := ReadBatch(span.Single(ctx), []string{alert.AlertKey})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched, should.Resemble([]*Alert{alert}))
		})

		t.Run("NotPresent returns empty fields", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			_ = NewAlertBuilder().CreateInDB(ctx, t)

			fetched, err := ReadBatch(span.Single(ctx), []string{"not-present"})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched, should.Resemble([]*Alert{{
				AlertKey:     "not-present",
				Bug:          0,
				GerritCL:     0,
				SilenceUntil: 0,
				ModifyTime:   time.Time{},
			}}))
		})
		t.Run("Multiple returned in order", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			alert1 := NewAlertBuilder().WithAlertKey("alert1").WithBug(1).CreateInDB(ctx, t)
			alert2 := NewAlertBuilder().WithAlertKey("alert2").WithBug(2).CreateInDB(ctx, t)

			forwards, err := ReadBatch(span.Single(ctx), []string{"alert1", "alert2"})
			backwards, err2 := ReadBatch(span.Single(ctx), []string{"alert2", "alert1"})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, err2, should.BeNil)
			assert.Loosely(t, forwards, should.Resemble([]*Alert{alert1, alert2}))
			assert.Loosely(t, backwards, should.Resemble([]*Alert{alert2, alert1}))
		})
	})
}

type AlertBuilder struct {
	alert Alert
}

func NewAlertBuilder() *AlertBuilder {
	return &AlertBuilder{alert: Alert{
		AlertKey:     "test-alert",
		Bug:          0,
		GerritCL:     0,
		SilenceUntil: 0,
		ModifyTime:   spanner.CommitTimestamp,
	}}
}

func (b *AlertBuilder) WithAlertKey(alertKey string) *AlertBuilder {
	b.alert.AlertKey = alertKey
	return b
}

func (b *AlertBuilder) WithBug(bug int64) *AlertBuilder {
	b.alert.Bug = bug
	return b
}

func (b *AlertBuilder) WithGerritCL(gerritCL int64) *AlertBuilder {
	b.alert.GerritCL = gerritCL
	return b
}

func (b *AlertBuilder) WithSilenceUntil(silenceUntil int64) *AlertBuilder {
	b.alert.SilenceUntil = silenceUntil
	return b
}

func (b *AlertBuilder) WithModifyTime(modifyTime time.Time) *AlertBuilder {
	b.alert.ModifyTime = modifyTime
	return b
}

func (b *AlertBuilder) Build() *Alert {
	s := b.alert
	return &s
}

func (b *AlertBuilder) CreateInDB(ctx context.Context, t testing.TB) *Alert {
	t.Helper()
	s := b.Build()
	row := map[string]any{
		"AlertKey":     s.AlertKey,
		"Bug":          s.Bug,
		"GerritCL":     s.GerritCL,
		"SilenceUntil": s.SilenceUntil,
		"ModifyTime":   s.ModifyTime,
	}
	m := spanner.InsertOrUpdateMap("Alerts", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	if s.ModifyTime == spanner.CommitTimestamp {
		s.ModifyTime = ts.UTC()
	}
	return s
}

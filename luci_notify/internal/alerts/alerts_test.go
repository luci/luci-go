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
	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/luci_notify/internal/testutil"
	"go.chromium.org/luci/server/span"
)

func TestValidation(t *testing.T) {
	Convey("Validate", t, func() {
		Convey("valid", func() {
			err := Validate(NewAlertBuilder().Build())
			So(err, ShouldBeNil)
		})
		Convey("bug", func() {
			Convey("must not be negative", func() {
				err := Validate(NewAlertBuilder().WithBug(-1).Build())
				So(err, ShouldErrLike, "bug: must be zero or positive")
			})
			Convey("zero is valid", func() {
				err := Validate(NewAlertBuilder().WithBug(0).Build())
				So(err, ShouldBeNil)
			})
		})
		Convey("SilenceUntil", func() {
			Convey("must not be negative", func() {
				err := Validate(NewAlertBuilder().WithSilenceUntil(-1).Build())
				So(err, ShouldErrLike, "silence_until: must be zero or positive")
			})
			Convey("zero is valid", func() {
				err := Validate(NewAlertBuilder().WithSilenceUntil(0).Build())
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestStatusTable(t *testing.T) {
	Convey("Put", t, func() {
		ctx := testutil.SpannerTestContext(t)
		alert := NewAlertBuilder().WithBug(10).Build()

		m, err := Put(alert)
		So(err, ShouldBeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		alert.ModifyTime = ts.UTC()

		So(err, ShouldBeNil)
		fetched, err := ReadBatch(span.Single(ctx), []string{alert.AlertKey})
		So(err, ShouldBeNil)
		So(fetched, ShouldEqual, []*Alert{alert})
	})
	Convey("Put deletes when all info is zero", t, func() {
		ctx := testutil.SpannerTestContext(t)
		alert := NewAlertBuilder().CreateInDB(ctx)

		m, err := Put(alert)
		So(err, ShouldBeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		alert.ModifyTime = ts.UTC()

		So(err, ShouldBeNil)
		// When we read a non-existent entry, a fake entry will be returned, so the only
		// way to tell it was actually deleted is a zero timestamp.
		fetched, err := ReadBatch(span.Single(ctx), []string{alert.AlertKey})
		So(err, ShouldBeNil)
		So(len(fetched), ShouldEqual, 1)
		So(fetched[0].ModifyTime, ShouldEqual, time.Time{})
	})

	Convey("ReadBatch", t, func() {
		Convey("Single", func() {
			ctx := testutil.SpannerTestContext(t)
			alert := NewAlertBuilder().CreateInDB(ctx)

			fetched, err := ReadBatch(span.Single(ctx), []string{alert.AlertKey})

			So(err, ShouldBeNil)
			So(fetched, ShouldEqual, []*Alert{alert})
		})

		Convey("NotPresent returns empty fields", func() {
			ctx := testutil.SpannerTestContext(t)
			_ = NewAlertBuilder().CreateInDB(ctx)

			fetched, err := ReadBatch(span.Single(ctx), []string{"not-present"})

			So(err, ShouldBeNil)
			So(fetched, ShouldEqual, []*Alert{{
				AlertKey:     "not-present",
				Bug:          0,
				SilenceUntil: 0,
				ModifyTime:   time.Time{},
			}})
		})
		Convey("Multiple returned in order", func() {
			ctx := testutil.SpannerTestContext(t)
			alert1 := NewAlertBuilder().WithAlertKey("alert1").WithBug(1).CreateInDB(ctx)
			alert2 := NewAlertBuilder().WithAlertKey("alert2").WithBug(2).CreateInDB(ctx)

			forwards, err := ReadBatch(span.Single(ctx), []string{"alert1", "alert2"})
			backwards, err2 := ReadBatch(span.Single(ctx), []string{"alert2", "alert1"})

			So(err, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(forwards, ShouldEqual, []*Alert{alert1, alert2})
			So(backwards, ShouldEqual, []*Alert{alert2, alert1})
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

func (b *AlertBuilder) CreateInDB(ctx context.Context) *Alert {
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

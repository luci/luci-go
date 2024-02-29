// Copyright 2023 The LUCI Authors.
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

package model

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskID(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx := memory.Use(context.Background())

		Convey("Key to string", func() {
			key := datastore.NewKey(ctx, "TaskRequest", "", 8787878774240697582, nil)
			So("60b2ed0a43023110", ShouldEqual, RequestKeyToTaskID(key, AsRequest))
			So("60b2ed0a43023111", ShouldEqual, RequestKeyToTaskID(key, AsRunResult))
		})

		Convey("String to key: AsRequest", func() {
			key, err := TaskIDToRequestKey(ctx, "60b2ed0a43023110")
			So(err, ShouldBeNil)
			So(key.IntID(), ShouldEqual, 8787878774240697582)
		})

		Convey("String to key: AsRunResult", func() {
			key, err := TaskIDToRequestKey(ctx, "60b2ed0a43023111")
			So(err, ShouldBeNil)
			So(key.IntID(), ShouldEqual, 8787878774240697582)
		})

		Convey("Bad hex", func() {
			_, err := TaskIDToRequestKey(ctx, "60b2ed0a4302311z")
			So(err, ShouldErrLike, "bad task ID: bad lowercase hex string")
		})

		Convey("Empty", func() {
			_, err := TaskIDToRequestKey(ctx, "")
			So(err, ShouldErrLike, "bad task ID: too small")
		})

		Convey("Overflow", func() {
			_, err := TaskIDToRequestKey(ctx, "ff60b2ed0a4302311f")
			So(err, ShouldErrLike, "value out of range")
		})
	})
}

func TestTimestampToRequestKey(t *testing.T) {
	t.Parallel()

	Convey("TestTimestampToRequestKey", t, func() {
		ctx := memory.Use(context.Background())

		Convey("ok", func() {
			in := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
			resp, err := TimestampToRequestKey(ctx, in, 0)
			So(err, ShouldBeNil)
			So(resp.IntID(), ShouldEqual, 8756616465961975806)
		})

		Convey("ok; use timestamppb", func() {
			in := timestamppb.New(time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC))
			resp, err := TimestampToRequestKey(ctx, in.AsTime(), 0)
			So(err, ShouldBeNil)
			So(resp.IntID(), ShouldEqual, 8756616465961975806)
		})

		Convey("two timestamps to keys maintain order", func() {
			ts1 := time.Date(2023, 2, 9, 0, 0, 0, 0, time.UTC)
			ts2 := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
			key1, err := TimestampToRequestKey(ctx, ts1, 0)
			So(err, ShouldBeNil)
			key2, err := TimestampToRequestKey(ctx, ts2, 0)
			So(err, ShouldBeNil)
			// The keys are in reverse chronological order, so the inequality is
			// the opposite of what we expect it to be.
			So(key1.IntID(), ShouldBeGreaterThan, key2.IntID())
		})

		Convey("not ok; bad timestamp", func() {
			in := timestamppb.New(time.Date(2000, 2, 9, 0, 0, 0, 0, time.UTC))
			resp, err := TimestampToRequestKey(ctx, in.AsTime(), 0)
			So(err, ShouldErrLike, "time 2000-02-09 00:00:00 +0000 UTC is set to before 1262304000")
			So(resp, ShouldBeNil)
		})

		Convey("not ok; bad suffix", func() {
			in := timestamppb.New(time.Date(2025, 2, 9, 0, 0, 0, 0, time.UTC))
			resp, err := TimestampToRequestKey(ctx, in.AsTime(), -1)
			So(err, ShouldErrLike, "invalid suffix")
			So(resp, ShouldBeNil)
		})
	})
}

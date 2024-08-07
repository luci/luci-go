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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestTaskID(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("Key to string", func(t *ftt.Test) {
			key := datastore.NewKey(ctx, "TaskRequest", "", 8787878774240697582, nil)
			assert.Loosely(t, "60b2ed0a43023110", should.Equal(RequestKeyToTaskID(key, AsRequest)))
			assert.Loosely(t, "60b2ed0a43023111", should.Equal(RequestKeyToTaskID(key, AsRunResult)))
		})

		t.Run("String to key: AsRequest", func(t *ftt.Test) {
			key, err := TaskIDToRequestKey(ctx, "60b2ed0a43023110")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, key.IntID(), should.Equal(8787878774240697582))
		})

		t.Run("String to key: AsRunResult", func(t *ftt.Test) {
			key, err := TaskIDToRequestKey(ctx, "60b2ed0a43023111")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, key.IntID(), should.Equal(8787878774240697582))
		})

		t.Run("Bad hex", func(t *ftt.Test) {
			_, err := TaskIDToRequestKey(ctx, "60b2ed0a4302311z")
			assert.Loosely(t, err, should.ErrLike("bad task ID: bad lowercase hex string"))
		})

		t.Run("Empty", func(t *ftt.Test) {
			_, err := TaskIDToRequestKey(ctx, "")
			assert.Loosely(t, err, should.ErrLike("bad task ID: too small"))
		})

		t.Run("Overflow", func(t *ftt.Test) {
			_, err := TaskIDToRequestKey(ctx, "ff60b2ed0a4302311f")
			assert.Loosely(t, err, should.ErrLike("value out of range"))
		})
	})
}

func TestTimestampToRequestKey(t *testing.T) {
	t.Parallel()

	ftt.Run("TestTimestampToRequestKey", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("ok", func(t *ftt.Test) {
			in := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
			resp, err := TimestampToRequestKey(ctx, in, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.IntID(), should.Equal(8756616465961975806))
		})

		t.Run("ok; use timestamppb", func(t *ftt.Test) {
			in := timestamppb.New(time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC))
			resp, err := TimestampToRequestKey(ctx, in.AsTime(), 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.IntID(), should.Equal(8756616465961975806))
		})

		t.Run("two timestamps to keys maintain order", func(t *ftt.Test) {
			ts1 := time.Date(2023, 2, 9, 0, 0, 0, 0, time.UTC)
			ts2 := time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC)
			key1, err := TimestampToRequestKey(ctx, ts1, 0)
			assert.Loosely(t, err, should.BeNil)
			key2, err := TimestampToRequestKey(ctx, ts2, 0)
			assert.Loosely(t, err, should.BeNil)
			// The keys are in reverse chronological order, so the inequality is
			// the opposite of what we expect it to be.
			assert.Loosely(t, key1.IntID(), should.BeGreaterThan(key2.IntID()))
		})

		t.Run("not ok; bad timestamp", func(t *ftt.Test) {
			in := timestamppb.New(time.Date(2000, 2, 9, 0, 0, 0, 0, time.UTC))
			resp, err := TimestampToRequestKey(ctx, in.AsTime(), 0)
			assert.Loosely(t, err, should.ErrLike("time 2000-02-09 00:00:00 +0000 UTC is before epoch 2010-01-01 00:00:00 +0000 UTC"))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("not ok; bad suffix", func(t *ftt.Test) {
			in := timestamppb.New(time.Date(2025, 2, 9, 0, 0, 0, 0, time.UTC))
			resp, err := TimestampToRequestKey(ctx, in.AsTime(), -1)
			assert.Loosely(t, err, should.ErrLike("invalid suffix"))
			assert.Loosely(t, resp, should.BeNil)
		})
	})
}

// Copyright 2026 The LUCI Authors.
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

package datastore_test

import (
	"context"
	"encoding/base64"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/impl/dummy"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	mc "go.chromium.org/luci/gae/service/datastore/internal/protos/multicursor"
)

type mockRawCursor string

func (m mockRawCursor) String() string {
	return string(m)
}

type mockRawCursorDecoder struct {
	dummy.Datastore
}

func (m mockRawCursorDecoder) DecodeCursor(curs string) (datastore.RawCursor, error) {
	return mockRawCursor(curs), nil
}

func TestCursorSerialization(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).Consistent(true)

	// Populate some data
	for i := 0; i < 5; i++ {
		assert.NoErr(t, datastore.Put(ctx, &TestIterRecord{ID: string(rune('a' + i)), Value: "val"}))
	}

	t.Run("Single cursor round-trip", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		count := 0
		var cur datastore.Cursor
		for _, err := range it.Results {
			assert.NoErr(t, err)
			count++
			if count == 2 {
				var curErr error
				cur, curErr = it.Cursor()
				assert.NoErr(t, curErr)
				break
			}
		}
		assert.Loosely(t, len(cur), should.Equal(1))
		curStr := cur.String()
		assert.Loosely(t, curStr, should.NotBeEmpty)

		// Decode cursor
		decoded, err := datastore.DecodeCursor(ctx, curStr)
		assert.NoErr(t, err)
		assert.Loosely(t, len(decoded), should.Equal(1))
		assert.Loosely(t, decoded[0].String(), should.Equal(cur[0].String()))

		// Resume query from decoded cursor
		qResumed := datastore.NewQuery("TestIterRecord").Start(decoded)
		itResumed := datastore.RunQuery[*TestIterRecord](ctx, qResumed)
		var remaining []string
		for r, err := range itResumed.Results {
			assert.NoErr(t, err)
			remaining = append(remaining, r.ID)
		}
		assert.Loosely(t, remaining, should.Match([]string{"c", "d", "e"}))
	})

	t.Run("Single-cursor is passed through, multi gets proto wrapper", func(t *testing.T) {
		raw := mockRawCursor("hello")
		c := datastore.Cursor{raw}
		assert.That(t, string(raw), should.Match(c.String()))

		c = datastore.Cursor{raw, raw}
		ctx := datastore.SetRaw(ctx, mockRawCursorDecoder{})

		decoded, err := datastore.DecodeCursor(ctx, c.String())
		assert.NoErr(t, err)
		assert.Loosely(t, decoded, should.HaveLength(2))
	})

	t.Run("Multi cursor with nil elements round-trip", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		for _, err := range it.Results {
			assert.NoErr(t, err)
			break
		}
		rawCur, err := it.Cursor()
		assert.NoErr(t, err)

		multiCur := datastore.Cursor{rawCur[0], nil, rawCur[0]}
		curStr := multiCur.String()

		decoded, err := datastore.DecodeCursor(ctx, curStr)
		assert.NoErr(t, err)
		assert.Loosely(t, len(decoded), should.Equal(3))
		assert.Loosely(t, decoded[0].String(), should.Equal(rawCur[0].String()))
		assert.Loosely(t, decoded[1], should.BeNil)
		assert.Loosely(t, decoded[2].String(), should.Equal(rawCur[0].String()))
	})

	t.Run("Nil cursor panics on String()", func(t *testing.T) {
		var c datastore.Cursor
		assert.Loosely(t, func() { _ = c.String() }, should.Panic)
	})

	t.Run("Empty cursor round-trip", func(t *testing.T) {
		c := datastore.Cursor{}
		str := c.String()
		decoded, err := datastore.DecodeCursor(ctx, str)
		assert.NoErr(t, err)
		assert.Loosely(t, len(decoded), should.Equal(0))
	})
}

func TestDecodeCursorErrors(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	t.Run("Version mismatch", func(t *testing.T) {
		bytes, err := proto.Marshal(&mc.Cursors{
			Cursors:     []string{"some-cursor"},
			MagicNumber: 0xA455,
			Version:     999,
		})
		assert.NoErr(t, err)
		str := base64.StdEncoding.EncodeToString(bytes)

		_, err = datastore.DecodeCursor(ctx, str)
		assert.Loosely(t, err, should.ErrLike("Cursor version mismatch"))
	})

	t.Run("Invalid raw fallback error", func(t *testing.T) {
		_, err := datastore.DecodeCursor(ctx, "completely-invalid-cursor!@#$")
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestApplyCursors(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).Consistent(true)

	assert.NoErr(t, datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val1"}))
	assert.NoErr(t, datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val1"}))
	assert.NoErr(t, datastore.Put(ctx, &TestIterRecord{ID: "c", Value: "val2"}))
	assert.NoErr(t, datastore.Put(ctx, &TestIterRecord{ID: "d", Value: "val2"}))

	q1 := datastore.NewQuery("TestIterRecord").Eq("value", "val1")
	q2 := datastore.NewQuery("TestIterRecord").Eq("value", "val2")

	queries := []*datastore.Query{q1, q2}

	it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
	var cur datastore.Cursor
	for _, err := range it.Results {
		assert.NoErr(t, err)
		var curErr error
		cur, curErr = it.Cursor()
		assert.NoErr(t, curErr)
		break
	}
	assert.Loosely(t, len(cur), should.Equal(2))

	t.Run("ApplyCursors valid", func(t *testing.T) {
		applied, err := datastore.ApplyCursors(ctx, queries, cur)
		assert.NoErr(t, err)
		assert.Loosely(t, len(applied), should.Equal(2))
	})

	t.Run("ApplyCursors length mismatch", func(t *testing.T) {
		_, err := datastore.ApplyCursors(ctx, []*datastore.Query{q1}, cur)
		assert.Loosely(t, err, should.ErrLike("Length mismatch"))
	})

	t.Run("ApplyCursorString valid", func(t *testing.T) {
		applied, err := datastore.ApplyCursorString(ctx, queries, cur.String())
		assert.NoErr(t, err)
		assert.Loosely(t, len(applied), should.Equal(2))
	})

	t.Run("ApplyCursorString invalid", func(t *testing.T) {
		_, err := datastore.ApplyCursorString(ctx, queries, "invalid-cursor-string")
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestQueryStartEnd(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	q := datastore.NewQuery("TestIterRecord")
	it := datastore.RunQuery[*TestIterRecord](ctx, q)
	cur, err := it.Cursor()
	assert.NoErr(t, err)

	t.Run("Query.Start with Cursor", func(t *testing.T) {
		q2 := q.Start(cur)
		assert.Loosely(t, q2, should.NotBeNil)
	})

	t.Run("Query.End with Cursor", func(t *testing.T) {
		q2 := q.End(cur)
		assert.Loosely(t, q2, should.NotBeNil)
	})

	t.Run("Query.Start with empty Cursor", func(t *testing.T) {
		q2 := q.Start(datastore.Cursor{})
		assert.Loosely(t, q2, should.NotBeNil)
	})
}

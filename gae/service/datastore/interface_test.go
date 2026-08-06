// Copyright 2015 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

const SimpleRecordKind = "SimpleRecord"

type SimpleRecord struct {
	_kind string `gae:"$kind,SimpleRecord"`
	key   string `gae:"$id"`
	value string `gae:"value"`
}

func getAll(ctx context.Context, n int) ([]*SimpleRecord, error) {
	var out []*SimpleRecord
	query := datastore.NewQuery(SimpleRecordKind)
	var err error
	if n <= 0 {
		err = datastore.GetAll(ctx, query, &out)
	} else {
		err = datastore.GetAllWithLimit(ctx, query, &out, n)
	}
	return out, err
}

func TestGetAll_Empty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	records, err := getAll(ctx, 1)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, len(records), should.Equal(0))
}

func TestGetAll_Singleton(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &SimpleRecord{key: "a", value: "b"})
	assert.Loosely(t, err, should.BeNil)

	records, err := getAll(ctx, 1)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, len(records), should.Equal(1))
}

func TestGetAll_Doubleton(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &SimpleRecord{key: "a", value: "b"})
	assert.Loosely(t, err, should.BeNil)

	err = datastore.Put(ctx, &SimpleRecord{key: "a1", value: "b1"})
	assert.Loosely(t, err, should.BeNil)

	records, err := getAll(ctx, 1)
	assert.Loosely(t, err, should.ErrLike(datastore.ErrLimitExceeded))
	assert.Loosely(t, len(records), should.Equal(1))

	records, err = getAll(ctx, 2)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, len(records), should.Equal(2))
}

type TestIterRecord struct {
	Kind  string `gae:"$kind,TestIterRecord"`
	ID    string `gae:"$id"`
	Value string `gae:"value"`
}

func TestRunQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val_a"})
	assert.Loosely(t, err, should.BeNil)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.Loosely(t, err, should.BeNil)

	t.Run("PointerToStruct", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		var got []*TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))
		assert.Loosely(t, got[1].Value, should.Equal("val_b"))
	})

	t.Run("StructValue", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[TestIterRecord](ctx, q)
		var got []TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))
		assert.Loosely(t, got[1].Value, should.Equal("val_b"))
	})

	t.Run("KeysOnly", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*datastore.Key](ctx, q)
		var got []*datastore.Key
		for k, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, k)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].StringID(), should.Equal("a"))
		assert.Loosely(t, got[1].StringID(), should.Equal("b"))
	})

	t.Run("Cursor", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord").Limit(1)
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		var got []*TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(1))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))

		cur, err := it.Cursor()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cur, should.NotBeNil)

		q2 := datastore.NewQuery("TestIterRecord").Start(cur)
		it2 := datastore.RunQuery[*TestIterRecord](ctx, q2)
		var got2 []*TestIterRecord
		for r, err := range it2.Results {
			assert.Loosely(t, err, should.BeNil)
			got2 = append(got2, r)
		}
		assert.Loosely(t, len(got2), should.Equal(1))
		assert.Loosely(t, got2[0].Value, should.Equal("val_b"))
	})

	t.Run("InvalidQuery", func(t *testing.T) {
		q := datastore.NewQuery("").Lt("invalid", nil)
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		for _, err := range it.Results {
			assert.Loosely(t, err, should.NotBeNil)
		}
	})
}

func TestRunMultiQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val_a"})
	assert.Loosely(t, err, should.BeNil)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.Loosely(t, err, should.BeNil)

	t.Run("PointerToStruct", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		var got []*TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))
		assert.Loosely(t, got[1].Value, should.Equal("val_b"))
	})

	t.Run("KeysOnly", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*datastore.Key](ctx, queries)
		var got []*datastore.Key
		for k, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, k)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].StringID(), should.Equal("a"))
		assert.Loosely(t, got[1].StringID(), should.Equal("b"))
	})
}

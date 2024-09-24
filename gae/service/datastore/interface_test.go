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

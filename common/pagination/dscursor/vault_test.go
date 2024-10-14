// Copyright 2021 The LUCI Authors.
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

package dscursor

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/pagination"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"
)

type TestEntity struct {
	ID    int    `gae:"$id"`
	Value string `gae:",noindex"`
}

func TestPagination(t *testing.T) {
	t.Parallel()

	ftt.Run("Vault", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).Consistent(true)

		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		for i := 1; i < 11; i++ {
			err := datastore.Put(ctx, &TestEntity{ID: i, Value: fmt.Sprintf("%d", i)})
			assert.Loosely(t, err, should.BeNil)
		}

		vault := NewVault([]byte("additional"))

		t.Run("Cursor() with an empty page token", func(t *ftt.Test) {
			cursor, err := vault.Cursor(ctx, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cursor, should.BeNil)
		})

		t.Run("PageToken() with an empty cursor", func(t *ftt.Test) {
			pageToken, err := vault.PageToken(ctx, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pageToken, should.BeEmpty)
		})

		t.Run("Cursor() with a page token created by PageToken()", func(t *ftt.Test) {
			q := datastore.NewQuery("TestEntity")

			var nextPageToken string
			counter := 0
			err := datastore.Run(ctx, q, func(e *TestEntity, cursorCB datastore.CursorCB) error {
				counter++
				assert.Loosely(t, e.ID, should.Equal(counter))
				if counter == 5 {
					cursor, err := cursorCB()
					assert.Loosely(t, err, should.BeNil)
					nextPageToken, err = vault.PageToken(ctx, cursor)
					assert.Loosely(t, err, should.BeNil)
					return datastore.Stop
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, counter, should.Equal(5))

			cursor, err := vault.Cursor(ctx, nextPageToken)
			assert.Loosely(t, err, should.BeNil)
			q = datastore.NewQuery("TestEntity").Start(cursor)
			err = datastore.Run(ctx, q, func(e *TestEntity) error {
				counter++
				assert.Loosely(t, e.ID, should.Equal(counter))
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Cursor() with a page token created by PageToken() from a different vault", func(t *ftt.Test) {
			anotherVault := NewVault([]byte("another additional"))
			q := datastore.NewQuery("TestEntity")

			var nextPageToken string
			counter := 0
			err := datastore.Run(ctx, q, func(e *TestEntity, cursorCB datastore.CursorCB) error {
				counter++
				assert.Loosely(t, e.ID, should.Equal(counter))
				if counter == 5 {
					cursor, err := cursorCB()
					assert.Loosely(t, err, should.BeNil)
					nextPageToken, err = anotherVault.PageToken(ctx, cursor)
					assert.Loosely(t, err, should.BeNil)
					return datastore.Stop
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, counter, should.Equal(5))

			cursor, err := vault.Cursor(ctx, nextPageToken)
			assert.Loosely(t, err, should.Equal(pagination.ErrInvalidPageToken))
			assert.Loosely(t, cursor, should.BeNil)
		})
	})
}

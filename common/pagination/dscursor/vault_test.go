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

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"
	"go.chromium.org/luci/common/pagination"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"

	. "github.com/smartystreets/goconvey/convey"
)

type TestEntity struct {
	ID    int    `gae:"$id"`
	Value string `gae:",noindex"`
}

func TestPagination(t *testing.T) {
	t.Parallel()

	Convey("Vault", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).Consistent(true)

		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		aead, err := aead.New(kh)
		So(err, ShouldBeNil)
		ctx = secrets.SetPrimaryTinkAEADForTest(ctx, aead)

		for i := 1; i < 11; i++ {
			err := datastore.Put(ctx, &TestEntity{ID: i, Value: fmt.Sprintf("%d", i)})
			So(err, ShouldBeNil)
		}

		vault := NewVault([]byte("additional"))

		Convey("Cursor() with an empty page token", func() {
			cursor, err := vault.Cursor(ctx, "")
			So(err, ShouldBeNil)
			So(cursor, ShouldBeNil)
		})

		Convey("PageToken() with an empty cursor", func() {
			pageToken, err := vault.PageToken(ctx, nil)
			So(err, ShouldBeNil)
			So(pageToken, ShouldBeEmpty)
		})

		Convey("Cursor() with a page token created by PageToken()", func() {
			q := datastore.NewQuery("TestEntity")

			var nextPageToken string
			counter := 0
			err := datastore.Run(ctx, q, func(e *TestEntity, cursorCB datastore.CursorCB) error {
				counter++
				So(e.ID, ShouldEqual, counter)
				if counter == 5 {
					cursor, err := cursorCB()
					So(err, ShouldBeNil)
					nextPageToken, err = vault.PageToken(ctx, cursor)
					So(err, ShouldBeNil)
					return datastore.Stop
				}
				return nil
			})
			So(err, ShouldBeNil)
			So(counter, ShouldEqual, 5)

			cursor, err := vault.Cursor(ctx, nextPageToken)
			So(err, ShouldBeNil)
			q = datastore.NewQuery("TestEntity").Start(cursor)
			err = datastore.Run(ctx, q, func(e *TestEntity) error {
				counter++
				So(e.ID, ShouldEqual, counter)
				return nil
			})
			So(err, ShouldBeNil)
		})

		Convey("Cursor() with a page token created by PageToken() from a different vault", func() {
			anotherVault := NewVault([]byte("another additional"))
			q := datastore.NewQuery("TestEntity")

			var nextPageToken string
			counter := 0
			err := datastore.Run(ctx, q, func(e *TestEntity, cursorCB datastore.CursorCB) error {
				counter++
				So(e.ID, ShouldEqual, counter)
				if counter == 5 {
					cursor, err := cursorCB()
					So(err, ShouldBeNil)
					nextPageToken, err = anotherVault.PageToken(ctx, cursor)
					So(err, ShouldBeNil)
					return datastore.Stop
				}
				return nil
			})
			So(err, ShouldBeNil)
			So(counter, ShouldEqual, 5)

			cursor, err := vault.Cursor(ctx, nextPageToken)
			So(err, ShouldEqual, pagination.ErrInvalidPageToken)
			So(cursor, ShouldBeNil)
		})
	})
}

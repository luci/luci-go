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

package bq

import (
	"context"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

type tableMock struct {
	fullyQualifiedName string

	md      *bigquery.TableMetadata
	mdCalls int
	mdErr   error

	createMD  *bigquery.TableMetadata
	createErr error

	updateMD  *bigquery.TableMetadataToUpdate
	updateErr error
}

func (t *tableMock) FullyQualifiedName() string {
	return t.fullyQualifiedName
}

func (t *tableMock) Metadata(ctx context.Context) (*bigquery.TableMetadata, error) {
	t.mdCalls++
	return t.md, t.mdErr
}

func (t *tableMock) Create(ctx context.Context, md *bigquery.TableMetadata) error {
	t.createMD = md
	return t.createErr
}

func (t *tableMock) Update(ctx context.Context, md bigquery.TableMetadataToUpdate, etag string) (*bigquery.TableMetadata, error) {
	t.updateMD = &md
	return t.md, t.updateErr
}

var cache = caching.RegisterLRUCache(50)

func TestBqTableCache(t *testing.T) {
	t.Parallel()
	Convey(`TestCheckBqTableCache`, t, func() {
		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx)

		t := &tableMock{
			fullyQualifiedName: "project.dataset.table",
			md:                 &bigquery.TableMetadata{},
		}

		sa := NewSchemaApplyer(cache)
		rowSchema := bigquery.Schema{
			{
				Name:   "exported",
				Type:   bigquery.RecordFieldType,
				Schema: bigquery.Schema{{Name: "id"}},
			},
			{
				Name:   "tags",
				Type:   bigquery.RecordFieldType,
				Schema: bigquery.Schema{{Name: "key"}, {Name: "value"}},
			},
			{
				Name: "created_time",
				Type: bigquery.TimestampFieldType,
			},
		}
		table := &bigquery.TableMetadata{
			Schema: rowSchema,
		}

		Convey(`Table does not exist`, func() {
			t.mdErr = &googleapi.Error{Code: http.StatusNotFound}
			err := sa.EnsureTable(ctx, t, table)
			So(err, ShouldBeNil)
			So(t.createMD.Schema, ShouldResemble, rowSchema)
		})

		Convey(`Table is missing fields`, func() {
			t.md.Schema = bigquery.Schema{
				{
					Name: "legacy",
				},
				{
					Name:   "exported",
					Schema: bigquery.Schema{{Name: "legacy"}},
				},
			}
			err := sa.EnsureTable(ctx, t, table)
			So(err, ShouldBeNil)

			So(t.updateMD, ShouldNotBeNil) // The table was updated.
			So(len(t.updateMD.Schema), ShouldBeGreaterThan, 3)
			So(t.updateMD.Schema[0].Name, ShouldEqual, "legacy")
			So(t.updateMD.Schema[1].Name, ShouldEqual, "exported")
			So(t.updateMD.Schema[1].Schema[0].Name, ShouldEqual, "legacy")
			So(t.updateMD.Schema[1].Schema[1].Name, ShouldEqual, "id") // new field
			So(t.updateMD.Schema[1].Schema[1].Required, ShouldBeFalse) // relaxed
		})

		Convey(`Table is up to date`, func() {
			t.md.Schema = rowSchema
			err := sa.EnsureTable(ctx, t, table)
			So(err, ShouldBeNil)
			So(t.updateMD, ShouldBeNil) // we did not try to update it
		})

		Convey(`Cache is working`, func() {
			err := sa.EnsureTable(ctx, t, table)
			So(err, ShouldBeNil)
			calls := t.mdCalls

			// Confirms the cache is working.
			err = sa.EnsureTable(ctx, t, table)
			So(err, ShouldBeNil)
			So(t.mdCalls, ShouldEqual, calls) // no more new calls were made.

			// Confirms the cache is expired as expected.
			tc.Add(6 * time.Minute)
			err = sa.EnsureTable(ctx, t, table)
			So(err, ShouldBeNil)
			So(t.mdCalls, ShouldBeGreaterThan, calls) // new calls were made.
		})
	})
}

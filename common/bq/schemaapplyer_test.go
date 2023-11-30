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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/caching"
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

func (t *tableMock) Metadata(ctx context.Context, opts ...bigquery.TableMetadataOption) (*bigquery.TableMetadata, error) {
	t.mdCalls++
	return t.md, t.mdErr
}

func (t *tableMock) Create(ctx context.Context, md *bigquery.TableMetadata) error {
	t.createMD = md
	return t.createErr
}

func (t *tableMock) Update(ctx context.Context, md bigquery.TableMetadataToUpdate, etag string, opts ...bigquery.TableUpdateOption) (*bigquery.TableMetadata, error) {
	t.updateMD = &md
	return t.md, t.updateErr
}

var cache = RegisterSchemaApplyerCache(50)

func TestBqTableCache(t *testing.T) {
	t.Parallel()
	Convey(`TestCheckBqTableCache`, t, func() {
		ctx := context.Background()
		referenceTime := time.Date(2030, time.February, 3, 4, 5, 6, 7, time.UTC)
		ctx, tc := testclock.UseTime(ctx, referenceTime)
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

		Convey(`Invalid attempt to convert regular table into view`, func() {
			table.ViewQuery = "SELECT * FROM a"
			err := sa.EnsureTable(ctx, t, table)
			So(err, ShouldErrLike, "cannot change a regular table into a view table")
			So(t.updateMD, ShouldBeNil)
		})

		Convey(`Views`, func() {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md: &bigquery.TableMetadata{
					Type:      bigquery.ViewTable,
					ViewQuery: "SELECT * FROM a",
				},
			}
			spec := &bigquery.TableMetadata{ViewQuery: "SELECT * FROM a"}

			Convey("With UpdateMetadata option", func() {
				mockTable.md.Labels = map[string]string{
					MetadataVersionKey: "9",
				}
				spec.Labels = map[string]string{
					MetadataVersionKey: "9",
				}

				Convey(`View is up to date`, func() {
					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
					So(err, ShouldBeNil)
					So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
				})

				Convey(`View requires update`, func() {
					spec.ViewQuery = "SELECT * FROM b"
					spec.Labels[MetadataVersionKey] = "10"
					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
					So(err, ShouldBeNil)

					expectedUpdate := &bigquery.TableMetadataToUpdate{
						ViewQuery: "SELECT * FROM b",
					}
					expectedUpdate.SetLabel(MetadataVersionKey, "10")
					So(mockTable.updateMD, ShouldResemble, expectedUpdate)
				})

				Convey(`View different but no new metadata version`, func() {
					spec.ViewQuery = "SELECT * FROM b"
					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
					So(err, ShouldBeNil)
					So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
				})

				Convey(`View requires update but not enforced`, func() {
					spec.ViewQuery = "SELECT * FROM b"
					spec.Labels[MetadataVersionKey] = "10"
					err := EnsureTable(ctx, mockTable, spec)
					So(err, ShouldBeNil)
					So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
				})

				Convey(`View is up to date, new metadata version and RefreshViewInterval enabled`, func() {
					mockTable.md.ViewQuery = "-- Indirect schema version: 2030-02-03T03:55:50Z\nSELECT * FROM a"
					spec.Labels[MetadataVersionKey] = "10"

					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata(), RefreshViewInterval(1*time.Hour))
					So(err, ShouldBeNil)

					// No update is applied except for the new metadata version.
					expectedUpdate := &bigquery.TableMetadataToUpdate{}
					expectedUpdate.SetLabel(MetadataVersionKey, "10")
					So(mockTable.updateMD, ShouldResemble, expectedUpdate)
				})

				Convey(`View requires update and RefreshViewInterval enabled`, func() {
					mockTable.md.ViewQuery = "-- Indirect schema version: 2030-02-03T03:55:50Z\nSELECT * FROM a"
					spec.ViewQuery = "SELECT * FROM b"
					spec.Labels[MetadataVersionKey] = "10"

					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata(), RefreshViewInterval(1*time.Hour))
					So(err, ShouldBeNil)

					expectedUpdate := &bigquery.TableMetadataToUpdate{
						ViewQuery: "-- Indirect schema version: 2030-02-03T04:05:06Z\nSELECT * FROM b",
					}
					expectedUpdate.SetLabel(MetadataVersionKey, "10")
					So(mockTable.updateMD, ShouldResemble, expectedUpdate)
				})

			})
			Convey(`With RefreshViewInterval option`, func() {
				spec.ViewQuery = "should be ignored as this option does not push spec.ViewQuery"

				Convey(`View is not stale`, func() {
					mockTable.md.ViewQuery = "-- Indirect schema version: 2030-02-03T03:55:50Z\nSELECT * FROM a"

					err := EnsureTable(ctx, mockTable, spec, RefreshViewInterval(1*time.Hour))
					So(err, ShouldBeNil)

					So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
				})
				Convey(`View is stale`, func() {
					mockTable.md.ViewQuery = "-- Indirect schema version: 2030-02-03T03:04:00Z\nSELECT * FROM a"

					err := EnsureTable(ctx, mockTable, spec, RefreshViewInterval(1*time.Hour))
					So(err, ShouldBeNil)

					expectedUpdate := &bigquery.TableMetadataToUpdate{
						ViewQuery: "-- Indirect schema version: 2030-02-03T04:05:06Z\nSELECT * FROM a",
					}
					So(mockTable.updateMD, ShouldResemble, expectedUpdate)
				})
				Convey(`View has no header`, func() {
					mockTable.md.ViewQuery = "SELECT * FROM a"

					err := EnsureTable(ctx, mockTable, spec, RefreshViewInterval(1*time.Hour))
					So(err, ShouldBeNil)

					expectedUpdate := &bigquery.TableMetadataToUpdate{
						ViewQuery: "-- Indirect schema version: 2030-02-03T04:05:06Z\nSELECT * FROM a",
					}
					So(mockTable.updateMD, ShouldResemble, expectedUpdate)
				})
			})
		})

		Convey(`Description`, func() {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md: &bigquery.TableMetadata{
					Type:        bigquery.ViewTable,
					ViewQuery:   "SELECT * FROM a",
					Description: "Description A",
					Labels: map[string]string{
						MetadataVersionKey: "1",
					},
				},
			}
			spec := &bigquery.TableMetadata{
				ViewQuery:   "SELECT * FROM a",
				Description: "Description A",
				Labels: map[string]string{
					MetadataVersionKey: "1",
				},
			}

			Convey(`Description is up to date`, func() {
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})

			Convey(`Description requires update`, func() {
				spec.Description = "Description B"
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)

				expectedUpdate := &bigquery.TableMetadataToUpdate{
					Description: "Description B",
				}
				expectedUpdate.SetLabel(MetadataVersionKey, "2")
				So(mockTable.updateMD, ShouldResemble, expectedUpdate)
			})

			Convey(`Description different but no new metadata version`, func() {
				spec.Description = "Description B"
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})

			Convey(`Description requires update but not enforced`, func() {
				spec.Description = "Description B"
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec)
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})
		})

		Convey(`Labels`, func() {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md: &bigquery.TableMetadata{
					Type:      bigquery.ViewTable,
					ViewQuery: "SELECT * FROM a",
					Labels:    map[string]string{MetadataVersionKey: "1", "key-a": "value-a", "key-b": "value-b", "key-c": "value-c"},
				},
			}
			spec := &bigquery.TableMetadata{
				ViewQuery: "SELECT * FROM a",
				Labels:    map[string]string{MetadataVersionKey: "1", "key-a": "value-a", "key-b": "value-b", "key-c": "value-c"},
			}

			Convey(`Labels are up to date`, func() {
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})

			Convey(`Labels require update`, func() {
				spec.Labels = map[string]string{MetadataVersionKey: "2", "key-a": "value-a", "key-b": "new-value-b", "key-d": "value-d"}
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)

				update := &bigquery.TableMetadataToUpdate{}
				update.DeleteLabel("key-c")
				update.SetLabel(MetadataVersionKey, "2")
				update.SetLabel("key-b", "new-value-b")
				update.SetLabel("key-d", "value-d")
				So(mockTable.updateMD, ShouldResemble, update)
			})

			Convey(`Labels require update but no new metadata version`, func() {
				spec.Labels = map[string]string{MetadataVersionKey: "1", "key-a": "value-a", "key-b": "new-value-b", "key-d": "value-d"}
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})

			Convey(`Labels require update but not enforced`, func() {
				spec.Labels = map[string]string{MetadataVersionKey: "2", "key-a": "value-a", "key-b": "new-value-b", "key-d": "value-d"}
				err := EnsureTable(ctx, mockTable, spec)
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})
		})

		Convey(`Clustering`, func() {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md: &bigquery.TableMetadata{
					Clustering: &bigquery.Clustering{Fields: []string{"field_a", "field_b"}},
					Labels: map[string]string{
						MetadataVersionKey: "1",
					},
				},
			}
			spec := &bigquery.TableMetadata{
				Clustering: &bigquery.Clustering{Fields: []string{"field_a", "field_b"}},
				Labels: map[string]string{
					MetadataVersionKey: "1",
				},
			}

			Convey(`Clustering is up to date`, func() {
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})

			Convey(`Clustering requires update`, func() {
				spec.Clustering = &bigquery.Clustering{Fields: []string{"field_c"}}
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)

				expectedUpdate := &bigquery.TableMetadataToUpdate{
					Clustering: &bigquery.Clustering{Fields: []string{"field_c"}},
				}
				expectedUpdate.SetLabel(MetadataVersionKey, "2")
				So(mockTable.updateMD, ShouldResemble, expectedUpdate)
			})

			Convey(`Clustering up to date but new metadata version`, func() {
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)

				expectedUpdate := &bigquery.TableMetadataToUpdate{}
				expectedUpdate.SetLabel(MetadataVersionKey, "2")
				So(mockTable.updateMD, ShouldResemble, expectedUpdate)
			})

			Convey(`Clustering different but no new metadata version`, func() {
				spec.Clustering = &bigquery.Clustering{Fields: []string{"field_c"}}
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})

			Convey(`Clustering requires update but not enforced`, func() {
				spec.Clustering = &bigquery.Clustering{Fields: []string{"field_c"}}
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec)
				So(err, ShouldBeNil)
				So(mockTable.updateMD, ShouldBeNil) // we did not try to update it
			})
		})

		Convey(`RefreshViewInterval is used on a table that is not a view`, func() {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md:                 nil,
			}
			spec := &bigquery.TableMetadata{}
			err := EnsureTable(ctx, mockTable, spec, RefreshViewInterval(time.Hour))
			So(err, ShouldEqual, errViewRefreshEnabledOnNonView)
		})

		Convey(`UpdateMetadata is used without spec having a metadata version`, func() {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md:                 nil,
			}
			spec := &bigquery.TableMetadata{}
			err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
			So(err, ShouldEqual, errMetadataVersionLabelMissing)
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

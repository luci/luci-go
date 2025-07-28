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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run(`TestCheckBqTableCache`, t, func(t *ftt.Test) {
		ctx := context.Background()
		referenceTime := time.Date(2030, time.February, 3, 4, 5, 6, 7, time.UTC)
		ctx, tc := testclock.UseTime(ctx, referenceTime)
		ctx = caching.WithEmptyProcessCache(ctx)

		tm := &tableMock{
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

		t.Run(`Table does not exist`, func(t *ftt.Test) {
			tm.mdErr = &googleapi.Error{Code: http.StatusNotFound}
			err := sa.EnsureTable(ctx, tm, table)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tm.createMD.Schema, should.Resemble(rowSchema))
		})

		t.Run(`Table is missing fields`, func(t *ftt.Test) {
			tm.md.Schema = bigquery.Schema{
				{
					Name: "legacy",
				},
				{
					Name:   "exported",
					Schema: bigquery.Schema{{Name: "legacy"}},
				},
			}
			err := sa.EnsureTable(ctx, tm, table)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, tm.updateMD, should.NotBeNil) // The table was updated.
			assert.Loosely(t, len(tm.updateMD.Schema), should.BeGreaterThan(3))
			assert.Loosely(t, tm.updateMD.Schema[0].Name, should.Equal("legacy"))
			assert.Loosely(t, tm.updateMD.Schema[1].Name, should.Equal("exported"))
			assert.Loosely(t, tm.updateMD.Schema[1].Schema[0].Name, should.Equal("legacy"))
			assert.Loosely(t, tm.updateMD.Schema[1].Schema[1].Name, should.Equal("id")) // new field
			assert.Loosely(t, tm.updateMD.Schema[1].Schema[1].Required, should.BeFalse) // relaxed
		})

		t.Run(`Table is up to date`, func(t *ftt.Test) {
			tm.md.Schema = rowSchema
			err := sa.EnsureTable(ctx, tm, table)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tm.updateMD, should.BeNil) // we did not try to update it
		})

		t.Run(`Invalid attempt to convert regular table into view`, func(t *ftt.Test) {
			table.ViewQuery = "SELECT * FROM a"
			err := sa.EnsureTable(ctx, tm, table)
			assert.Loosely(t, err, should.ErrLike("cannot change a regular table into a view table"))
			assert.Loosely(t, tm.updateMD, should.BeNil)
		})

		t.Run(`Views`, func(t *ftt.Test) {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md: &bigquery.TableMetadata{
					Type:      bigquery.ViewTable,
					ViewQuery: "SELECT * FROM a",
				},
			}
			spec := &bigquery.TableMetadata{ViewQuery: "SELECT * FROM a"}

			t.Run("With UpdateMetadata option", func(t *ftt.Test) {
				mockTable.md.Labels = map[string]string{
					MetadataVersionKey: "9",
				}
				spec.Labels = map[string]string{
					MetadataVersionKey: "9",
				}

				t.Run(`View is up to date`, func(t *ftt.Test) {
					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
				})

				t.Run(`View requires update`, func(t *ftt.Test) {
					spec.ViewQuery = "SELECT * FROM b"
					spec.Labels[MetadataVersionKey] = "10"
					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
					assert.Loosely(t, err, should.BeNil)

					expectedUpdate := &bigquery.TableMetadataToUpdate{
						ViewQuery: "SELECT * FROM b",
					}
					expectedUpdate.SetLabel(MetadataVersionKey, "10")
					assert.Loosely(t, mockTable.updateMD, should.Resemble(expectedUpdate))
				})

				t.Run(`View different but no new metadata version`, func(t *ftt.Test) {
					spec.ViewQuery = "SELECT * FROM b"
					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
				})

				t.Run(`View requires update but not enforced`, func(t *ftt.Test) {
					spec.ViewQuery = "SELECT * FROM b"
					spec.Labels[MetadataVersionKey] = "10"
					err := EnsureTable(ctx, mockTable, spec)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
				})

				t.Run(`View is up to date, new metadata version and RefreshViewInterval enabled`, func(t *ftt.Test) {
					mockTable.md.ViewQuery = "-- Indirect schema version: 2030-02-03T03:55:50Z\nSELECT * FROM a"
					spec.Labels[MetadataVersionKey] = "10"

					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata(), RefreshViewInterval(1*time.Hour))
					assert.Loosely(t, err, should.BeNil)

					// No update is applied except for the new metadata version.
					expectedUpdate := &bigquery.TableMetadataToUpdate{}
					expectedUpdate.SetLabel(MetadataVersionKey, "10")
					assert.Loosely(t, mockTable.updateMD, should.Resemble(expectedUpdate))
				})

				t.Run(`View requires update and RefreshViewInterval enabled`, func(t *ftt.Test) {
					mockTable.md.ViewQuery = "-- Indirect schema version: 2030-02-03T03:55:50Z\nSELECT * FROM a"
					spec.ViewQuery = "SELECT * FROM b"
					spec.Labels[MetadataVersionKey] = "10"

					err := EnsureTable(ctx, mockTable, spec, UpdateMetadata(), RefreshViewInterval(1*time.Hour))
					assert.Loosely(t, err, should.BeNil)

					expectedUpdate := &bigquery.TableMetadataToUpdate{
						ViewQuery: "-- Indirect schema version: 2030-02-03T04:05:06Z\nSELECT * FROM b",
					}
					expectedUpdate.SetLabel(MetadataVersionKey, "10")
					assert.Loosely(t, mockTable.updateMD, should.Resemble(expectedUpdate))
				})
			})
			t.Run(`With RefreshViewInterval option`, func(t *ftt.Test) {
				spec.ViewQuery = "should be ignored as this option does not push spec.ViewQuery"

				t.Run(`View is not stale`, func(t *ftt.Test) {
					mockTable.md.ViewQuery = "-- Indirect schema version: 2030-02-03T03:55:50Z\nSELECT * FROM a"

					err := EnsureTable(ctx, mockTable, spec, RefreshViewInterval(1*time.Hour))
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
				})
				t.Run(`View is stale`, func(t *ftt.Test) {
					mockTable.md.ViewQuery = "-- Indirect schema version: 2030-02-03T03:04:00Z\nSELECT * FROM a"

					err := EnsureTable(ctx, mockTable, spec, RefreshViewInterval(1*time.Hour))
					assert.Loosely(t, err, should.BeNil)

					expectedUpdate := &bigquery.TableMetadataToUpdate{
						ViewQuery: "-- Indirect schema version: 2030-02-03T04:05:06Z\nSELECT * FROM a",
					}
					assert.Loosely(t, mockTable.updateMD, should.Resemble(expectedUpdate))
				})
				t.Run(`View has no header`, func(t *ftt.Test) {
					mockTable.md.ViewQuery = "SELECT * FROM a"

					err := EnsureTable(ctx, mockTable, spec, RefreshViewInterval(1*time.Hour))
					assert.Loosely(t, err, should.BeNil)

					expectedUpdate := &bigquery.TableMetadataToUpdate{
						ViewQuery: "-- Indirect schema version: 2030-02-03T04:05:06Z\nSELECT * FROM a",
					}
					assert.Loosely(t, mockTable.updateMD, should.Resemble(expectedUpdate))
				})
			})
		})

		t.Run(`Description`, func(t *ftt.Test) {
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

			t.Run(`Description is up to date`, func(t *ftt.Test) {
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})

			t.Run(`Description requires update`, func(t *ftt.Test) {
				spec.Description = "Description B"
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)

				expectedUpdate := &bigquery.TableMetadataToUpdate{
					Description: "Description B",
				}
				expectedUpdate.SetLabel(MetadataVersionKey, "2")
				assert.Loosely(t, mockTable.updateMD, should.Resemble(expectedUpdate))
			})

			t.Run(`Description different but no new metadata version`, func(t *ftt.Test) {
				spec.Description = "Description B"
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})

			t.Run(`Description requires update but not enforced`, func(t *ftt.Test) {
				spec.Description = "Description B"
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})
		})

		t.Run(`Labels`, func(t *ftt.Test) {
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

			t.Run(`Labels are up to date`, func(t *ftt.Test) {
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})

			t.Run(`Labels require update`, func(t *ftt.Test) {
				spec.Labels = map[string]string{MetadataVersionKey: "2", "key-a": "value-a", "key-b": "new-value-b", "key-d": "value-d"}
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)

				update := &bigquery.TableMetadataToUpdate{}
				update.DeleteLabel("key-c")
				update.SetLabel(MetadataVersionKey, "2")
				update.SetLabel("key-b", "new-value-b")
				update.SetLabel("key-d", "value-d")
				assert.Loosely(t, mockTable.updateMD, should.Resemble(update))
			})

			t.Run(`Labels require update but no new metadata version`, func(t *ftt.Test) {
				spec.Labels = map[string]string{MetadataVersionKey: "1", "key-a": "value-a", "key-b": "new-value-b", "key-d": "value-d"}
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})

			t.Run(`Labels require update but not enforced`, func(t *ftt.Test) {
				spec.Labels = map[string]string{MetadataVersionKey: "2", "key-a": "value-a", "key-b": "new-value-b", "key-d": "value-d"}
				err := EnsureTable(ctx, mockTable, spec)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})
		})

		t.Run(`Clustering`, func(t *ftt.Test) {
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

			t.Run(`Clustering is up to date`, func(t *ftt.Test) {
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})

			t.Run(`Clustering requires update`, func(t *ftt.Test) {
				spec.Clustering = &bigquery.Clustering{Fields: []string{"field_c"}}
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)

				expectedUpdate := &bigquery.TableMetadataToUpdate{
					Clustering: &bigquery.Clustering{Fields: []string{"field_c"}},
				}
				expectedUpdate.SetLabel(MetadataVersionKey, "2")
				assert.Loosely(t, mockTable.updateMD, should.Resemble(expectedUpdate))
			})

			t.Run(`Clustering up to date but new metadata version`, func(t *ftt.Test) {
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)

				expectedUpdate := &bigquery.TableMetadataToUpdate{}
				expectedUpdate.SetLabel(MetadataVersionKey, "2")
				assert.Loosely(t, mockTable.updateMD, should.Resemble(expectedUpdate))
			})

			t.Run(`Clustering different but no new metadata version`, func(t *ftt.Test) {
				spec.Clustering = &bigquery.Clustering{Fields: []string{"field_c"}}
				err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})

			t.Run(`Clustering requires update but not enforced`, func(t *ftt.Test) {
				spec.Clustering = &bigquery.Clustering{Fields: []string{"field_c"}}
				spec.Labels[MetadataVersionKey] = "2"
				err := EnsureTable(ctx, mockTable, spec)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mockTable.updateMD, should.BeNil) // we did not try to update it
			})
		})

		t.Run(`RefreshViewInterval is used on a table that is not a view`, func(t *ftt.Test) {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md:                 nil,
			}
			spec := &bigquery.TableMetadata{}
			err := EnsureTable(ctx, mockTable, spec, RefreshViewInterval(time.Hour))
			assert.Loosely(t, err, should.Equal(errViewRefreshEnabledOnNonView))
		})

		t.Run(`UpdateMetadata is used without spec having a metadata version`, func(t *ftt.Test) {
			mockTable := &tableMock{
				fullyQualifiedName: "project.dataset.table",
				md:                 nil,
			}
			spec := &bigquery.TableMetadata{}
			err := EnsureTable(ctx, mockTable, spec, UpdateMetadata())
			assert.Loosely(t, err, should.Equal(errMetadataVersionLabelMissing))
		})

		t.Run(`Cache is working`, func(t *ftt.Test) {
			err := sa.EnsureTable(ctx, tm, table)
			assert.Loosely(t, err, should.BeNil)
			calls := tm.mdCalls

			// Confirms the cache is working.
			err = sa.EnsureTable(ctx, tm, table)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tm.mdCalls, should.Equal(calls)) // no more new calls were made.

			// Confirms the cache is expired as expected.
			tc.Add(6 * time.Minute)
			err = sa.EnsureTable(ctx, tm, table)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tm.mdCalls, should.BeGreaterThan(calls)) // new calls were made.
		})
	})
}

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

package builders

import (
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/luci_notify/internal/testutil"
)

func TestList(t *testing.T) {
	ftt.Run("List", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		// Pre-insert some builders.
		m1 := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
			"BuilderKey":      "chromium/ci/builder-1",
			"Project":         "chromium",
			"Bucket":          "ci",
			"Builder":         "builder-1",
			"Realm":           "chromium:ci",
			"Status":          "FAILURE",
			"UpdateTime":      spanner.CommitTimestamp,
			"BuildId":         int64(100),
			"OnCallRotations": []string{"angle"},
		})
		m2 := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
			"BuilderKey":      "chromium/ci/builder-2",
			"Project":         "chromium",
			"Bucket":          "ci",
			"Builder":         "builder-2",
			"Realm":           "chromium:ci",
			"Status":          "SUCCESS",
			"UpdateTime":      spanner.CommitTimestamp,
			"BuildId":         int64(200),
			"OnCallRotations": []string{"gardener"},
		})
		m3 := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
			"BuilderKey":      "fuchsia/ci/builder-3",
			"Project":         "fuchsia",
			"Bucket":          "ci",
			"Builder":         "builder-3",
			"Realm":           "fuchsia:ci",
			"Status":          "FAILURE",
			"UpdateTime":      spanner.CommitTimestamp,
			"BuildId":         int64(300),
			"OnCallRotations": []string{"fuchsia"},
		})
		_, err := span.Apply(ctx, []*spanner.Mutation{m1, m2, m3})
		assert.Loosely(t, err, should.BeNil)

		t.Run("List all", func(t *ftt.Test) {
			opts := ListOptions{
				FullRealms: []string{"chromium:ci", "fuchsia:ci"},
			}
			builders, nextPageToken, err := List(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(builders), should.Equal(3))
			assert.Loosely(t, nextPageToken, should.Equal(""))
		})

		t.Run("Filter by project", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`project = "chromium"`)
			assert.Loosely(t, err, should.BeNil)

			opts := ListOptions{
				FullRealms: []string{"chromium:ci", "fuchsia:ci"},
				Filter:     filter,
			}
			builders, _, err := List(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(builders), should.Equal(2))
			assert.Loosely(t, builders[0].Project, should.Equal("chromium"))
			assert.Loosely(t, builders[1].Project, should.Equal("chromium"))
		})

		t.Run("Filter by status", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`status = "FAILURE"`)
			assert.Loosely(t, err, should.BeNil)

			opts := ListOptions{
				FullRealms: []string{"chromium:ci", "fuchsia:ci"},
				Filter:     filter,
			}
			builders, _, err := List(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(builders), should.Equal(2))
			assert.Loosely(t, builders[0].Status, should.Equal("FAILURE"))
			assert.Loosely(t, builders[1].Status, should.Equal("FAILURE"))
		})

		t.Run("Pagination", func(t *ftt.Test) {
			opts := ListOptions{
				FullRealms: []string{"chromium:ci", "fuchsia:ci"},
				PageSize:   1,
			}
			builders, nextPageToken, err := List(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(builders), should.Equal(1))
			assert.Loosely(t, nextPageToken, should.NotEqual(""))

			opts2 := ListOptions{
				FullRealms: []string{"chromium:ci", "fuchsia:ci"},
				PageSize:   1,
				PageToken:  nextPageToken,
			}
			builders2, _, err := List(span.Single(ctx), opts2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(builders2), should.Equal(1))
			assert.Loosely(t, builders2[0].BuilderKey, should.NotEqual(builders[0].BuilderKey))
		})

		t.Run("ACL Filtering", func(t *ftt.Test) {
			t.Run("Specific realm", func(t *ftt.Test) {
				opts := ListOptions{
					FullRealms: []string{"chromium:ci"},
				}
				builders, _, err := List(span.Single(ctx), opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(builders), should.Equal(2))
				assert.Loosely(t, builders[0].Project, should.Equal("chromium"))
				assert.Loosely(t, builders[1].Project, should.Equal("chromium"))
			})

			t.Run("Wildcard project", func(t *ftt.Test) {
				opts := ListOptions{
					WildcardProjects: []string{"chromium"},
				}
				builders, _, err := List(span.Single(ctx), opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(builders), should.Equal(2))
			})
		})
	})
}

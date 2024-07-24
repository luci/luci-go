// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/server/caching"
)

func TestUpdateBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestUpdateBuilder`, t, func(t *ftt.Test) {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")

		builder := &BuilderSummary{BuilderID: "fake"}

		// Populate a few BuildSummaries. For convenience, ordered by creation time.
		builds := make([]*BuildSummary, 10)
		for i := 0; i < 10; i++ {
			builds[i] = &BuildSummary{
				BuildKey:  datastore.MakeKey(c, "fakeBuild", i),
				BuilderID: builder.BuilderID,
				BuildID:   fmt.Sprintf("build_id/%d", i),
				Created:   testclock.TestRecentTimeUTC.Add(time.Duration(i) * time.Hour),
			}
		}

		c = caching.WithRequestCache(c)

		updateBuilder := func(build *BuildSummary) {
			err := datastore.RunInTransaction(c, func(c context.Context) error {
				return UpdateBuilderForBuild(c, build)
			}, nil)
			assert.Loosely(t, err, should.BeNil)
			err = datastore.Get(c, builder)
			assert.Loosely(t, err, should.BeNil)
		}

		t.Run("Updating appropriate builder having existing last finished build", func(t *ftt.Test) {
			builder.LastFinishedCreated = builds[5].Created
			builder.LastFinishedStatus = milostatus.Success
			builder.LastFinishedBuildID = builds[5].BuildID
			err := datastore.Put(c, builder)
			assert.Loosely(t, err, should.BeNil)

			t.Run("with finished build should not update last finished build info", func(t *ftt.Test) {
				builds[6].Summary.Status = milostatus.Failure
				updateBuilder(builds[6])
				assert.Loosely(t, builder.LastFinishedStatus, should.Equal(milostatus.Failure))
				assert.Loosely(t, builder.LastFinishedBuildID, should.Equal(builds[6].BuildID))
			})

			t.Run("for build created earlier than last finished", func(t *ftt.Test) {
				builds[4].Summary.Status = milostatus.Failure
				updateBuilder(builds[4])
				assert.Loosely(t, builder.LastFinishedStatus, should.Equal(milostatus.Success))
				assert.Loosely(t, builder.LastFinishedBuildID, should.Equal(builds[5].BuildID))
			})

			t.Run("for build created later than last finished", func(t *ftt.Test) {
				builds[6].Summary.Status = milostatus.NotRun
				updateBuilder(builds[6])
				assert.Loosely(t, builder.LastFinishedStatus, should.Equal(milostatus.Success))
				assert.Loosely(t, builder.LastFinishedBuildID, should.Equal(builds[5].BuildID))
			})
		})

		t.Run("Updating appropriate builder with no last finished build should initialize it", func(t *ftt.Test) {
			builds[5].Summary.Status = milostatus.Failure
			updateBuilder(builds[5])
			assert.Loosely(t, builder.LastFinishedStatus, should.Equal(milostatus.Failure))
			assert.Loosely(t, builder.LastFinishedBuildID, should.Equal(builds[5].BuildID))
		})
	})
}

func TestBuildIDLink(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestLastFinishedBuildIDLink`, t, func(t *ftt.Test) {
		t.Run("Buildbot build gets expected link", func(t *ftt.Test) {
			t.Run("with valid BuildID", func(t *ftt.Test) {
				buildID, project := "buildbot/buildergroup/builder/number", "proj"
				assert.Loosely(t, buildIDLink(buildID, project), should.Equal("/buildbot/buildergroup/builder/number"))
			})

			t.Run("with invalid BuildID", func(t *ftt.Test) {
				t.Run("with too few tokens", func(t *ftt.Test) {
					buildID, project := "buildbot/wat", "proj"
					assert.Loosely(t, buildIDLink(buildID, project), should.Equal("#invalid-build-id"))
				})

				t.Run("with too many tokens", func(t *ftt.Test) {
					buildID, project := "buildbot/wat/wat/wat/wat", "proj"
					assert.Loosely(t, buildIDLink(buildID, project), should.Equal("#invalid-build-id"))
				})
			})
		})

		t.Run("Buildbucket build gets expected link", func(t *ftt.Test) {
			t.Run("with bucket info", func(t *ftt.Test) {
				buildID, project := "buildbucket/luci.proj.bucket/builder/123", ""
				assert.Loosely(t,
					buildIDLink(buildID, project),
					should.Equal(
						"/p/proj/builders/bucket/builder/123"))
			})

			t.Run("with only ID info", func(t *ftt.Test) {
				buildID, project := "buildbucket/123", "proj"
				assert.Loosely(t, buildIDLink(buildID, project), should.Equal("/b/123"))
			})

			t.Run("with invalid BuildID", func(t *ftt.Test) {
				t.Run("due to missing bucket info", func(t *ftt.Test) {
					buildID, project := "buildbucket/", "proj"
					assert.Loosely(t, buildIDLink(buildID, project), should.Equal("#invalid-build-id"))
				})
			})
		})

		t.Run("Invalid BuildID gets expected link", func(t *ftt.Test) {
			t.Run("with unknown source gets expected link", func(t *ftt.Test) {
				buildID, project := "unknown/1", "proj"
				assert.Loosely(t, buildIDLink(buildID, project), should.Equal("#invalid-build-id"))
			})

			t.Run("with too few tokens", func(t *ftt.Test) {
				buildID, project := "source", "proj"
				assert.Loosely(t, buildIDLink(buildID, project), should.Equal("#invalid-build-id"))
			})
		})
	})
}

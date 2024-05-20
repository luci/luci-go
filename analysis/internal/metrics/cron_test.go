// Copyright 2022 The LUCI Authors.
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

package metrics

import (
	"testing"
	"time"

	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/ingestion/controllegacy"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGlobalMetrics(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.IntegrationTestContext(t)

		ctx = memory.Use(ctx) // For project config in datastore.

		// Setup project configuration.
		projectCfgs := map[string]*configpb.ProjectConfig{
			"project-a": {},
			"project-b": {},
		}
		So(config.SetTestProjectConfig(ctx, projectCfgs), ShouldBeNil)

		// Create some active rules.
		rulesToCreate := []*rules.Entry{
			rules.NewRule(0).WithProject("project-a").WithActive(true).Build(),
			rules.NewRule(1).WithProject("project-a").WithActive(true).Build(),
		}
		err := rules.SetForTesting(ctx, rulesToCreate)
		So(err, ShouldBeNil)

		// Create some ingestion control records.
		reference := time.Now().Add(-1 * time.Minute)
		entriesToCreate := []*controllegacy.Entry{
			controllegacy.NewEntry(0).
				WithBuildProject("project-a").
				WithPresubmitProject("project-b").
				WithInvocationProject("project-c").
				WithBuildJoinedTime(reference).
				WithPresubmitJoinedTime(reference).
				WithInvocationJoinedTime(reference).
				Build(),
			controllegacy.NewEntry(1).
				WithBuildProject("project-d").
				WithBuildJoinedTime(reference).
				WithInvocationResult(nil).
				WithPresubmitResult(nil).Build(),
			controllegacy.NewEntry(2).
				WithPresubmitProject("project-e").
				WithPresubmitJoinedTime(reference).
				WithBuildResult(nil).
				WithInvocationResult(nil).Build(),
			controllegacy.NewEntry(3).
				WithInvocationProject("project-f").
				WithInvocationJoinedTime(reference).
				WithBuildResult(nil).
				WithPresubmitResult(nil).Build(),
		}
		_, err = controllegacy.SetEntriesForTesting(ctx, entriesToCreate...)
		So(err, ShouldBeNil)

		err = GlobalMetrics(ctx)
		So(err, ShouldBeNil)
	})
}

// Copyright 2024 The LUCI Authors.
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

package commitingester

import (
	"testing"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/source_index/internal/commitingester/taskspb"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestProcessSourceRepoEvent(t *testing.T) {
	Convey(`ProcessSourceRepoEvent`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		ctx = memory.Use(ctx)
		err := config.SetTestConfig(ctx, config.TestCfg)
		So(err, ShouldBeNil)

		gitilesHost := "chromium.googlesource.com"
		event := &gerritpb.SourceRepoEvent{
			Name: "projects/chromium-gerrit/repos/chromium/src",
			Url:  "https://source.developers.google.com/p/chromium-gerrit/r/chromium/src",
			Event: &gerritpb.SourceRepoEvent_RefUpdateEvent_{
				RefUpdateEvent: &gerritpb.SourceRepoEvent_RefUpdateEvent{
					Email: "committer@google.com",
					RefUpdates: map[string]*gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate{
						"refs/heads/main": {
							RefName:    "refs/heads/main",
							UpdateType: gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate_UPDATE_FAST_FORWARD,
							NewId:      "94f4b5c7c0bacc03caf215987a068db54b88af20",
							OldId:      "0b0f8af119cc5f68b94a9d4485c1ae44205d1823",
						},
						"refs/heads/another-branch": {
							RefName:    "refs/heads/another-branch",
							UpdateType: gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate_UPDATE_NON_FAST_FORWARD,
							NewId:      "7b27e2d6c6df37c17eb984b44115674572d1ed99",
							OldId:      "298f4fd6813db06dd03fe5c1d20c67b4005234b5",
						},
						"refs/branch-heads/feature-1": {
							RefName:    "refs/branch-heads/feature-1",
							UpdateType: gerritpb.SourceRepoEvent_RefUpdateEvent_RefUpdate_CREATE,
							NewId:      "960e7620ed8a5368f8ef170e38d4d0f1d7690a17",
							OldId:      "",
						},
					},
				},
			},
		}

		expectedTasks := map[string]*taskspb.IngestCommits{
			"94f4b5c7c0bacc03caf215987a068db54b88af20": {
				Host:       "chromium.googlesource.com",
				Repository: "chromium/src",
				Commitish:  "94f4b5c7c0bacc03caf215987a068db54b88af20",
				PageToken:  "",
				TaskIndex:  0,
			},
			"960e7620ed8a5368f8ef170e38d4d0f1d7690a17": {
				Host:       "chromium.googlesource.com",
				Repository: "chromium/src",
				Commitish:  "960e7620ed8a5368f8ef170e38d4d0f1d7690a17",
				PageToken:  "",
				TaskIndex:  0,
			},
		}

		assertTasksExpected := func() string {
			actualTasks := make(map[string]*taskspb.IngestCommits, len(skdr.Tasks().Payloads()))
			for _, payload := range skdr.Tasks().Payloads() {
				t := payload.(*taskspb.IngestCommits)
				actualTasks[t.Commitish] = t
			}

			return ShouldResembleProto(actualTasks, expectedTasks)
		}

		Convey(`With repo that should be ingested`, func() {
			err := processSourceRepoEvent(ctx, gitilesHost, event)

			So(err, ShouldBeNil)
			assertTasksExpected()
		})

		Convey(`With repo that should not be ingested`, func() {
			event.Name = "projects/chromium-gerrit/repos/another-repo"
			event.Url = "https://source.developers.google.com/p/chromium-gerrit/r/another-repo"
			expectedTasks = make(map[string]*taskspb.IngestCommits)

			err := processSourceRepoEvent(ctx, gitilesHost, event)

			So(err, ShouldBeNil)
			So(assertTasksExpected(), ShouldBeEmpty)
		})

		Convey(`With host that is not configured`, func() {
			gitilesHost = "another-host.googlesource.com"
			expectedTasks = make(map[string]*taskspb.IngestCommits)

			err := processSourceRepoEvent(ctx, gitilesHost, event)

			So(err, ShouldNotBeNil)
			So(assertTasksExpected(), ShouldBeEmpty)
		})
	})
}

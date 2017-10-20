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

package buildbucket

import (
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuild(t *testing.T) {
	t.Parallel()
	Convey("Build", t, func() {

		// Load test message.
		msgBytes, err := ioutil.ReadFile("testdata/build.json")
		So(err, ShouldBeNil)
		msg := &buildbucket.ApiCommonBuildMessage{}
		err = json.Unmarshal(msgBytes, msg)
		So(err, ShouldBeNil)

		num := 4124
		build := Build{
			ID:           8967467172028179648,
			Address:      "luci.chromium.try/linux_chromium_rel_ng/4124",
			CreationTime: time.Date(2017, 9, 25, 15, 38, 17, 28510000, time.UTC),
			CreatedBy:    "user:luci-migration@appspot.gserviceaccount.com",
			Bucket:       "luci.chromium.try",
			Builder:      "linux_chromium_rel_ng",
			Number:       &num,
			BuildSets:    []BuildSet{&GerritChange{"chromium-review.googlesource.com", 678507, 3}},
			Tags: strpair.Map{
				"build_address":                    []string{"luci.chromium.try/linux_chromium_rel_ng/4124"},
				"builder":                          []string{"linux_chromium_rel_ng"},
				"buildset":                         []string{"patch/gerrit/chromium-review.googlesource.com/678507/3"},
				"luci_migration_attempt":           []string{"0"},
				"luci_migration_buildbot_build_id": []string{"8967467703804786960"},
				"swarming_dimension": []string{
					"cpu:x86-64",
					"os:Ubuntu-14.04",
					"pool:Chrome.LUCI",
				},
				"swarming_hostname": []string{"chromium-swarm.appspot.com"},
				"swarming_tag": []string{
					"build_address:luci.chromium.try/linux_chromium_rel_ng/4124",
					"buildbucket_bucket:luci.chromium.try",
					"buildbucket_build_id:8967467172028179648",
					"buildbucket_hostname:cr-buildbucket.appspot.com",
					"buildbucket_template_revision:e345c8ccccd935552f9d58c0a64beeb88dcc320d",
				},
				"swarming_task_id": []string{"38d281e8c20fd510"},
				"user_agent":       []string{"luci-migration"},
			},
			Input: Input{
				Properties: map[string]interface{}{
					"attempt_start_ts":     json.Number("1506353363000000.0"),
					"category":             "cq_experimental",
					"master":               "master.tryserver.chromium.linux",
					"patch_gerrit_url":     "https://chromium-review.googlesource.com",
					"patch_issue":          json.Number("678507"),
					"patch_project":        "chromium/src",
					"patch_ref":            "refs/changes/07/678507/3",
					"patch_repository_url": "https://chromium.googlesource.com/chromium/src",
					"patch_set":            json.Number("3"),
					"patch_storage":        "gerrit",
					"reason":               "CQ",
					"revision":             "ea69a470367f45ef9c98b9cb79d540c689154d0d",
				},
			},
			Output: Output{
				Properties: map[string]interface{}{
					"got_revision": "ea69a470367f45ef9c98b9cb79d540c689154d0d",
				},
			},
			CanaryPreference: CanaryAllowed,
			StartTime:        time.Date(2017, 9, 25, 15, 38, 19, 345070000, time.UTC),
			Status:           StatusSuccess,
			StatusChangeTime: time.Date(2017, 9, 25, 15, 44, 52, 983790000, time.UTC),
			URL:              "https://ci.chromium.org/swarming/task/38d281e8c20fd510?server=chromium-swarm.appspot.com",
			UpdateTime:       time.Date(2017, 9, 25, 15, 44, 52, 984620000, time.UTC),
			Canary:           true,
			CompletionTime:   time.Date(2017, 9, 25, 15, 44, 52, 983790000, time.UTC),
		}

		Convey("ParseMessage", func() {
			var actual Build
			err := actual.ParseMessage(msg)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, build)
		})

		Convey("ParseMessage with structured properties", func() {
			type props struct {
				Category string
				Master   string
				Revision string
			}
			var actual Build
			actual.Input.Properties = &props{}
			err := actual.ParseMessage(msg)
			So(err, ShouldBeNil)
			So(actual.Input.Properties, ShouldResemble, &props{
				Category: "cq_experimental",
				Master:   "master.tryserver.chromium.linux",
				Revision: "ea69a470367f45ef9c98b9cb79d540c689154d0d",
			})
		})

		Convey("ParseMessage a build with an error", func() {
			msg.Result = "FAILURE"
			msg.FailureReason = "INFRA_FAILURE"
			msg.ResultDetailsJson = `{"error": {"message": "bad"}}`

			var actual Build
			err := actual.ParseMessage(msg)
			So(err, ShouldBeNil)
			So(actual.Status, ShouldEqual, StatusError)
			So(actual.Output.Err, ShouldErrLike, "bad")
		})

		Convey("PutRequest", func() {
			paramsJSON, err := json.Marshal(map[string]interface{}{
				"builder_name": "linux_chromium_rel_ng",
				"properties":   build.Input.Properties,
			})
			So(err, ShouldBeNil)
			actual, err := build.PutRequest()
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, &buildbucket.ApiPutRequestMessage{
				Bucket:           "luci.chromium.try",
				CanaryPreference: "AUTO",
				ParametersJson:   string(paramsJSON),
				Tags: []string{
					"build_address:luci.chromium.try/linux_chromium_rel_ng/4124",
					"buildset:patch/gerrit/chromium-review.googlesource.com/678507/3",
					"luci_migration_attempt:0",
					"luci_migration_buildbot_build_id:8967467703804786960",
					"swarming_dimension:cpu:x86-64",
					"swarming_dimension:os:Ubuntu-14.04",
					"swarming_dimension:pool:Chrome.LUCI",
					"swarming_hostname:chromium-swarm.appspot.com",
					"swarming_tag:build_address:luci.chromium.try/linux_chromium_rel_ng/4124",
					"swarming_tag:buildbucket_bucket:luci.chromium.try",
					"swarming_tag:buildbucket_build_id:8967467172028179648",
					"swarming_tag:buildbucket_hostname:cr-buildbucket.appspot.com",
					"swarming_tag:buildbucket_template_revision:e345c8ccccd935552f9d58c0a64beeb88dcc320d",
					"swarming_task_id:38d281e8c20fd510",
					"user_agent:luci-migration",
				},
			})
		})
	})
}

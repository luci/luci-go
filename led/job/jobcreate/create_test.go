// Copyright 2020 The LUCI Authors.
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

package jobcreate

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/types/known/durationpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/led/job"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var train = flag.Bool("train", false, "If set, write testdata/*out.json")

func readTestFixture(fixtureBaseName string) *job.Definition {
	data, err := os.ReadFile(fmt.Sprintf("testdata/%s.json", fixtureBaseName))
	So(err, ShouldBeNil)

	req := &swarmingpb.NewTaskRequest{}
	So(json.NewDecoder(bytes.NewReader(data)).Decode(req), ShouldBeNil)

	jd, err := FromNewTaskRequest(
		context.Background(), req,
		"test_name", "swarming.example.com",
		job.NoKitchenSupport(), 10, nil, nil, nil)
	So(err, ShouldBeNil)
	So(jd, ShouldNotBeNil)

	outFile := fmt.Sprintf("testdata/%s.job.json", fixtureBaseName)
	marshaler := &jsonpb.Marshaler{
		OrigName: true,
		Indent:   "  ",
	}
	if *train {
		oFile, err := os.Create(outFile)
		So(err, ShouldBeNil)
		defer oFile.Close()

		So(marshaler.Marshal(oFile, jd), ShouldBeNil)
	} else {
		current, err := os.ReadFile(outFile)
		So(err, ShouldBeNil)

		actual, err := marshaler.MarshalToString(jd)
		So(err, ShouldBeNil)

		So(actual, ShouldEqual, string(current))
	}

	return jd
}

func TestCreateSwarmRaw(t *testing.T) {
	t.Parallel()

	Convey(`consume non-buildbucket swarming task with RBE-CAS prop`, t, func() {
		jd := readTestFixture("raw_cas")

		So(jd.GetSwarming(), ShouldNotBeNil)
		So(jd.Info().SwarmingHostname(), ShouldEqual, "swarming.example.com")
		So(jd.Info().TaskName(), ShouldEqual, "led: test_name")
	})

	Convey(`consume non-buildbucket swarming task with resultdb enabling`, t, func() {
		jd := readTestFixture("raw_cas")
		Convey(`realm unset`, func() {
			So(jd.FlattenToSwarming(context.Background(), "username", "parent_task_id", job.NoKitchenSupport(), "on"), ShouldErrLike,
				"ResultDB cannot be enabled on raw swarming tasks if the realm field is unset")
			So(jd.GetSwarming().GetTask().GetResultdb().GetEnable(), ShouldBeFalse)
		})
		Convey(`realm set`, func() {
			jd.GetSwarming().GetTask().Realm = "some:realm"
			So(jd.FlattenToSwarming(context.Background(), "username", "parent_task_id", job.NoKitchenSupport(), "on"), ShouldBeNil)
			So(jd.GetSwarming().GetTask().GetResultdb().GetEnable(), ShouldBeTrue)
		})
	})
}

func TestCreateBBagent(t *testing.T) {
	t.Parallel()

	Convey(`consume bbagent buildbucket swarming task with RBE-CAS prop`, t, func() {
		jd := readTestFixture("bbagent_cas")

		So(jd.GetBuildbucket(), ShouldNotBeNil)
		So(jd.Info().SwarmingHostname(), ShouldEqual, "chromium-swarm-dev.appspot.com")
		So(jd.Info().TaskName(), ShouldEqual, "led: test_name")
	})

	Convey(`consume bbagent buildbucket swarming task with build`, t, func() {
		bld := &bbpb.Build{
			Builder: &bbpb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Infra: &bbpb.BuildInfra{
				Bbagent: &bbpb.BuildInfra_BBAgent{
					PayloadPath:            "path",
					CacheDir:               "dir",
					KnownPublicGerritHosts: []string{"host"},
				},
				Swarming: &bbpb.BuildInfra_Swarming{
					Priority: 25,
				},
				Buildbucket: &bbpb.BuildInfra_Buildbucket{
					KnownPublicGerritHosts: []string{"host"},
				},
				Logdog: &bbpb.BuildInfra_LogDog{},
			},
			Input:             &bbpb.Build_Input{},
			Exe:               &bbpb.Executable{},
			SchedulingTimeout: durationpb.New(time.Hour),
			ExecutionTimeout:  durationpb.New(2 * time.Hour),
			GracePeriod:       durationpb.New(time.Minute),
			Tags: []*bbpb.StringPair{
				{
					Key:   "k",
					Value: "v",
				},
			},
		}
		data, err := os.ReadFile(fmt.Sprintf("testdata/%s.json", "bbagent_cas"))
		So(err, ShouldBeNil)

		req := &swarmingpb.NewTaskRequest{}
		So(json.NewDecoder(bytes.NewReader(data)).Decode(req), ShouldBeNil)
		req.TaskSlices[0].Properties.Command = []string{
			"bbagent${EXECUTABLE_SUFFIX}",
			"-host",
			"cr-buildbucket.appspot.com",
			"-build-id",
			"123"}

		jd, err := FromNewTaskRequest(
			context.Background(), req,
			"test_name", "swarming.example.com",
			job.NoKitchenSupport(), 10, bld, []string{"k:v"}, nil)
		So(err, ShouldBeNil)
		So(jd, ShouldNotBeNil)

		So(jd.GetBuildbucket(), ShouldNotBeNil)
		So(jd.Info().TaskName(), ShouldEqual, "led: test_name")
		So(jd.GetBuildbucket().BbagentArgs, ShouldResembleProto, &bbpb.BBAgentArgs{
			PayloadPath:            bld.Infra.Bbagent.PayloadPath,
			CacheDir:               bld.Infra.Bbagent.CacheDir,
			KnownPublicGerritHosts: bld.Infra.Bbagent.KnownPublicGerritHosts,
			Build:                  bld,
		})
	})

	Convey(`consume bbagent buildbucket swarming task led job`, t, func() {
		jd := readTestFixture("bbagent_led")

		So(jd.GetBuildbucket(), ShouldNotBeNil)
		So(jd.Info().SwarmingHostname(), ShouldEqual, "chromium-swarm-dev.appspot.com")
		So(jd.Info().TaskName(), ShouldEqual, "led: test_name")
	})
}

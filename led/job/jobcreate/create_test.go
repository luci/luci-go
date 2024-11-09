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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job"
)

var train = flag.Bool("train", false, "If set, write testdata/*out.json")

func readTestFixture(t testing.TB, fixtureBaseName string) *job.Definition {
	t.Helper()

	data, err := os.ReadFile(fmt.Sprintf("testdata/%s.json", fixtureBaseName))
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	req := &swarmingpb.NewTaskRequest{}
	assert.Loosely(t, json.NewDecoder(bytes.NewReader(data)).Decode(req), should.BeNil, truth.LineContext())

	jd, err := FromNewTaskRequest(
		context.Background(), req,
		"test_name", "swarming.example.com",
		job.NoKitchenSupport(), 10, nil, nil, nil)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, jd, should.NotBeNil, truth.LineContext())

	outFile := fmt.Sprintf("testdata/%s.job.json", fixtureBaseName)
	marshaler := &jsonpb.Marshaler{
		OrigName: true,
		Indent:   "  ",
	}
	if *train {
		oFile, err := os.Create(outFile)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())
		defer oFile.Close()

		assert.Loosely(t, marshaler.Marshal(oFile, jd), should.BeNil, truth.LineContext())
	} else {
		current, err := os.ReadFile(outFile)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())

		actual, err := marshaler.MarshalToString(jd)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())

		assert.Loosely(t, actual, should.Equal(string(current)), truth.LineContext())
	}

	return jd
}

func TestCreateSwarmRaw(t *testing.T) {
	t.Parallel()

	ftt.Run(`consume non-buildbucket swarming task with RBE-CAS prop`, t, func(t *ftt.Test) {
		jd := readTestFixture(t, "raw_cas")

		assert.Loosely(t, jd.GetSwarming(), should.NotBeNil)
		assert.Loosely(t, jd.Info().SwarmingHostname(), should.Equal("swarming.example.com"))
		assert.Loosely(t, jd.Info().TaskName(), should.Equal("led: test_name"))
	})

	ftt.Run(`consume non-buildbucket swarming task with resultdb enabling`, t, func(t *ftt.Test) {
		jd := readTestFixture(t, "raw_cas")
		t.Run(`realm unset`, func(t *ftt.Test) {
			assert.Loosely(t, jd.FlattenToSwarming(context.Background(), "username", "parent_task_id", job.NoKitchenSupport(), "on"), should.ErrLike(
				"ResultDB cannot be enabled on raw swarming tasks if the realm field is unset"))
			assert.Loosely(t, jd.GetSwarming().GetTask().GetResultdb().GetEnable(), should.BeFalse)
		})
		t.Run(`realm set`, func(t *ftt.Test) {
			jd.GetSwarming().GetTask().Realm = "some:realm"
			assert.Loosely(t, jd.FlattenToSwarming(context.Background(), "username", "parent_task_id", job.NoKitchenSupport(), "on"), should.BeNil)
			assert.Loosely(t, jd.GetSwarming().GetTask().GetResultdb().GetEnable(), should.BeTrue)
		})
	})
}

func TestCreateBBagent(t *testing.T) {
	t.Parallel()

	ftt.Run(`consume bbagent buildbucket swarming task with RBE-CAS prop`, t, func(t *ftt.Test) {
		jd := readTestFixture(t, "bbagent_cas")

		assert.Loosely(t, jd.GetBuildbucket(), should.NotBeNil)
		assert.Loosely(t, jd.Info().SwarmingHostname(), should.Equal("chromium-swarm-dev.appspot.com"))
		assert.Loosely(t, jd.Info().TaskName(), should.Equal("led: test_name"))
	})

	ftt.Run(`consume bbagent buildbucket swarming task with build`, t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		req := &swarmingpb.NewTaskRequest{}
		assert.Loosely(t, json.NewDecoder(bytes.NewReader(data)).Decode(req), should.BeNil)
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, jd, should.NotBeNil)

		assert.Loosely(t, jd.GetBuildbucket(), should.NotBeNil)
		assert.Loosely(t, jd.Info().TaskName(), should.Equal("led: test_name"))
		assert.Loosely(t, jd.GetBuildbucket().BbagentArgs, should.Resemble(&bbpb.BBAgentArgs{
			PayloadPath:            bld.Infra.Bbagent.PayloadPath,
			CacheDir:               bld.Infra.Bbagent.CacheDir,
			KnownPublicGerritHosts: bld.Infra.Bbagent.KnownPublicGerritHosts,
			Build:                  bld,
		}))
	})

	ftt.Run(`consume bbagent buildbucket swarming task led job`, t, func(t *ftt.Test) {
		jd := readTestFixture(t, "bbagent_led")

		assert.Loosely(t, jd.GetBuildbucket(), should.NotBeNil)
		assert.Loosely(t, jd.Info().SwarmingHostname(), should.Equal("chromium-swarm-dev.appspot.com"))
		assert.Loosely(t, jd.Info().TaskName(), should.Equal("led: test_name"))
	})
}

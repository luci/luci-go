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

package jobexport

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/led/job"
)

var train = flag.Bool("train", false, "If set, write testdata/*.swarm.json")

func readTestFixture(fixtureBaseName string) *swarming.SwarmingRpcsNewTaskRequest {
	jobFile, err := os.Open(fmt.Sprintf("testdata/%s.job.json", fixtureBaseName))
	So(err, ShouldBeNil)
	defer jobFile.Close()

	jd := &job.Definition{}
	So(jsonpb.Unmarshal(jobFile, jd), ShouldBeNil)
	So(jd, ShouldNotBeNil)

	ctx := cryptorand.MockForTest(context.Background(), 0)
	ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
	So(jd.FlattenToSwarming(ctx, "testuser@example.com", "293109284abc", job.NoKitchenSupport(), "off"),
		ShouldBeNil)

	ret, err := ToSwarmingNewTask(jd.GetSwarming())
	So(err, ShouldBeNil)

	outFile := fmt.Sprintf("testdata/%s.swarm.json", fixtureBaseName)
	if *train {
		oFile, err := os.Create(outFile)
		So(err, ShouldBeNil)
		defer oFile.Close()

		enc := json.NewEncoder(oFile)
		enc.SetIndent("", "  ")
		So(enc.Encode(ret), ShouldBeNil)
	} else {
		current, err := os.ReadFile(outFile)
		So(err, ShouldBeNil)

		actual, err := json.MarshalIndent(ret, "", "  ")
		So(err, ShouldBeNil)

		So(string(actual)+"\n", ShouldEqual, string(current))
	}

	return ret
}

func TestExportRaw(t *testing.T) {
	t.Parallel()

	Convey(`export raw swarming task with rbe-cas input`, t, func() {
		req := readTestFixture("raw_cas")
		So(req, ShouldNotBeNil)
	})
}

func TestExportBBagent(t *testing.T) {
	t.Parallel()

	Convey(`export bbagent task with rbe-cas input`, t, func() {
		req := readTestFixture("bbagent_cas")
		So(req, ShouldNotBeNil)
	})
}

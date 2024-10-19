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

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/led/job"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

var train = flag.Bool("train", false, "If set, write testdata/*.swarm.json")

func readTestFixture(t testing.TB, fixtureBaseName string) *swarmingpb.NewTaskRequest {
	t.Helper()

	jobFile, err := os.Open(fmt.Sprintf("testdata/%s.job.json", fixtureBaseName))
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	defer jobFile.Close()

	jd := &job.Definition{}
	assert.Loosely(t, jsonpb.Unmarshal(jobFile, jd), should.BeNil, truth.LineContext())
	assert.Loosely(t, jd, should.NotBeNil, truth.LineContext())

	ctx := cryptorand.MockForTest(context.Background(), 0)
	ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
	assert.Loosely(t, jd.FlattenToSwarming(ctx, "testuser@example.com", "293109284abc", job.NoKitchenSupport(), "off"),
		should.BeNil, truth.LineContext())

	ret := jd.GetSwarming().GetTask()
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	outFile := fmt.Sprintf("testdata/%s.swarm.json", fixtureBaseName)
	if *train {
		oFile, err := os.Create(outFile)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())
		defer oFile.Close()

		enc := json.NewEncoder(oFile)
		enc.SetIndent("", "  ")
		assert.Loosely(t, enc.Encode(ret), should.BeNil, truth.LineContext())
	} else {
		current, err := os.ReadFile(outFile)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())

		actual, err := json.MarshalIndent(ret, "", "  ")
		assert.Loosely(t, err, should.BeNil, truth.LineContext())

		assert.Loosely(t, string(actual)+"\n", should.Equal(string(current)), truth.LineContext())
	}

	return ret
}

func TestExportRaw(t *testing.T) {
	t.Parallel()

	ftt.Run(`export raw swarming task with rbe-cas input`, t, func(t *ftt.Test) {
		req := readTestFixture(t, "raw_cas")
		assert.Loosely(t, req, should.NotBeNil)
	})
}

func TestExportBBagent(t *testing.T) {
	t.Parallel()

	ftt.Run(`export bbagent task with rbe-cas input`, t, func(t *ftt.Test) {
		req := readTestFixture(t, "bbagent_cas")
		assert.Loosely(t, req, should.NotBeNil)
	})
}

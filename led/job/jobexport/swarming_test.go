// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package jobexport

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
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
	ret, err := ToSwarmingNewTask(ctx, jd, "testuser@example.com", job.NoKitchenSupport())
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
		current, err := ioutil.ReadFile(outFile)
		So(err, ShouldBeNil)

		actual, err := json.MarshalIndent(ret, "", "  ")
		So(err, ShouldBeNil)

		So(string(current), ShouldEqual, string(actual)+"\n")
	}

	return ret
}

func TestExportRaw(t *testing.T) {
	t.Parallel()

	Convey(`export raw swarming task`, t, func() {
		req := readTestFixture("raw")
		So(req, ShouldNotBeNil)
	})
}

func TestExportBBagent(t *testing.T) {
	t.Parallel()

	Convey(`export bbagent task`, t, func() {
		req := readTestFixture("bbagent")
		So(req, ShouldNotBeNil)
	})
}

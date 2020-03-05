// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package jobcreate

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	. "github.com/smartystreets/goconvey/convey"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/led/job"
)

var train = flag.Bool("train", false, "If set, write testdata/*out.json")

func readTestFixture(fixtureBaseName string) *job.Definition {
	data, err := ioutil.ReadFile(fmt.Sprintf("testdata/%s.json", fixtureBaseName))
	So(err, ShouldBeNil)

	req := &swarming.SwarmingRpcsNewTaskRequest{}
	So(json.NewDecoder(bytes.NewReader(data)).Decode(req), ShouldBeNil)

	jd, err := FromNewTaskRequest(
		context.Background(), req,
		"test_name", "swarming.example.com",
		job.NoKitchenSupport())
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
		current, err := ioutil.ReadFile(outFile)
		So(err, ShouldBeNil)

		actual, err := marshaler.MarshalToString(jd)
		So(err, ShouldBeNil)

		So(string(current), ShouldEqual, actual)
	}

	return jd
}

func TestCreateSwarmRaw(t *testing.T) {
	t.Parallel()

	Convey(`consume non-buildbucket swarming task`, t, func() {
		jd := readTestFixture("raw")

		So(jd.GetSwarming(), ShouldNotBeNil)
		So(jd.Info().SwarmingHostname(), ShouldEqual, "swarming.example.com")
		So(jd.Info().TaskName(), ShouldEqual, "led: test_name")
	})
}

func TestCreateBBagent(t *testing.T) {
	t.Parallel()

	Convey(`consume bbagent buildbucket swarming task`, t, func() {
		jd := readTestFixture("bbagent")

		So(jd.GetBuildbucket(), ShouldNotBeNil)
		So(jd.Info().SwarmingHostname(), ShouldEqual, "chromium-swarm-dev.appspot.com")
		So(jd.Info().TaskName(), ShouldEqual, "led: test_name")
	})
}

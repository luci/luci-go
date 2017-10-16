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

package buildstore

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/milo/api/buildbot"

	. "github.com/smartystreets/goconvey/convey"
)

var generate = flag.Bool("test.generate", false, "Generate expectations instead of running tests.")

func TestAnnotations(t *testing.T) {
	t.Parallel()

	Convey("Annotations", t, func() {
		c := context.Background()

		txtBytes, err := ioutil.ReadFile("testdata/annotation.pb.txt")
		So(err, ShouldBeNil)
		var annotations milo.Step
		err = proto.UnmarshalText(string(txtBytes), &annotations)
		So(err, ShouldBeNil)

		conv := annotationConverter{
			logdogServer:       "logdog.example.com",
			logdogPrefix:       "chromium/prefix",
			buildCompletedTime: testclock.TestRecentTimeUTC,
		}
		var actual []buildbot.Step
		conv.addSteps(c, &actual, annotations.GetSubstep(), "")

		step0Start, err := ptypes.Timestamp(annotations.GetSubstep()[0].GetStep().GetStarted())
		So(err, ShouldBeNil)
		So(actual[0].Times.Start.Time, ShouldEqual, step0Start)

		step0End, err := ptypes.Timestamp(annotations.GetSubstep()[0].GetStep().GetEnded())
		So(err, ShouldBeNil)
		So(actual[0].Times.Finish.Time, ShouldEqual, step0End)

		// Truncate times at microseconds. JSON encoding has low time resolution
		// because it stores time as float.
		for i := range actual {
			s := &actual[i]
			s.Times.Finish.Time = s.Times.Finish.Truncate(time.Microsecond)
			s.Times.Start.Time = s.Times.Start.Truncate(time.Microsecond)
		}

		expectationFilePath := "expectations/steps.json"
		if *generate {
			stepsJSON, err := json.MarshalIndent(actual, "", "  ")
			So(err, ShouldBeNil)
			err = ioutil.WriteFile(expectationFilePath, stepsJSON, 0644)
			So(err, ShouldBeNil)
			return
		}

		stepsJSON, err := ioutil.ReadFile(expectationFilePath)
		So(err, ShouldBeNil)
		var expected []buildbot.Step
		err = json.Unmarshal(stepsJSON, &expected)
		So(err, ShouldBeNil)
		So(expected[0].Times.Start.Time.Before(expected[0].Times.Finish.Time), ShouldBeTrue)

		So(actual[0], ShouldResemble, expected[0])
		So(actual, ShouldResemble, expected)
	})
}

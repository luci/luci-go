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

package job

import (
	"sort"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	api "go.chromium.org/luci/swarming/proto/api"
)

type testCase struct {
	name string
	fn   func(*Definition)

	// These control if the test is disabled for one of the test types.
	skipBB bool
	// If true, the job should be for a v2 Buildbucket build.
	v2Build bool

	skipSW       bool // skips all swarming tests
	skipSWEmpty  bool // skips just swarming tests with an empty Task
	skipSWSlices bool // skips just swarming tests with a 3-slice Task
}

// Time from the beginning of the task request until the given slice expires.
//
// Note that swarming slice expirations in the Definition are stored relative to
// the previous slice.
const (
	swSlice1ExpSecs = 60
	swSlice2ExpSecs = 240
	swSlice3ExpSecs = 600

	swSlice1Exp = swSlice1ExpSecs * time.Second // relative: 60-0    = 60
	swSlice2Exp = swSlice2ExpSecs * time.Second // relative: 240-60  = 180
	swSlice3Exp = swSlice3ExpSecs * time.Second // relative: 600-240 = 360
)

func testBBJob(v2Build bool) *Definition {
	jd := &Definition{JobType: &Definition_Buildbucket{
		Buildbucket: &Buildbucket{
			Name: "default-task-name",
		},
	}}
	if v2Build {
		jd.GetBuildbucket().BbagentArgs = &bbpb.BBAgentArgs{
			Build: &bbpb.Build{
				Input: &bbpb.Build_Input{
					Experiments: []string{
						buildbucket.ExperimentBBAgentDownloadCipd,
					},
				},
			},
		}
		jd.GetBuildbucket().LegacyKitchen = false
	}
	return jd
}

func testSWJob(sliceExps ...time.Duration) *Definition {
	ret := &Definition{JobType: &Definition_Swarming{
		Swarming: &Swarming{},
	}}
	if len(sliceExps) > 0 {
		sw := ret.GetSwarming()
		sw.Task = &api.TaskRequest{
			Name: "default-task-name",
		}

		for _, exp := range sliceExps {
			sw.Task.TaskSlices = append(sw.Task.TaskSlices, &api.TaskSlice{
				Expiration: durationpb.New(exp),
			})
		}
	}
	return ret
}

func must(value any, err error) any {
	So(err, ShouldBeNil)
	return value
}

// runCases runs some 'edit' tests for the given operation name (e.g. a method
// of the Editor interface).
//
// This will invoke every function in `tests` with an empty Buildbucket
// or Swarming Definition (so, every function in `tests` is called exactly
// twice).
//
// If a given test function would like to add type-specific verification, it
// should switch on the type of the Definition.
func runCases(t *testing.T, opName string, tests []testCase) {
	Convey(opName, t, func() {
		Convey(`bb`, func() {
			for _, tc := range tests {
				ConveyIf(!tc.skipBB, tc.name, func() {
					tc.fn(testBBJob(tc.v2Build))
				})
			}
		})
		Convey(`sw (empty)`, func() {
			for _, tc := range tests {
				ConveyIf(!(tc.skipSW || tc.skipSWEmpty), tc.name, func() {
					tc.fn(testSWJob())
				})
			}
		})
		Convey(`sw (slices)`, func() {
			for _, tc := range tests {
				ConveyIf(!(tc.skipSW || tc.skipSWSlices), tc.name, func() {
					// Make a swarming job which expires in 600 seconds.
					tc.fn(testSWJob(
						swSlice1Exp,
						swSlice2Exp-swSlice1Exp,
						swSlice3Exp-swSlice2Exp,
					))
				})
			}
		})
	})
}

func ConveyIf(cond bool, items ...any) {
	if cond {
		Convey(items...)
	} else {
		SkipConvey(items...)
	}
}

func SoEdit(jd *Definition, cb func(Editor)) {
	So(jd.Edit(cb), ShouldBeNil)
}

func SoHLEdit(jd *Definition, cb func(HighLevelEditor)) {
	So(jd.HighLevelEdit(cb), ShouldBeNil)
}

func mustGetDimensions(jd *Definition) ExpiringDimensions {
	ret, err := jd.Info().Dimensions()
	So(err, ShouldBeNil)
	return ret
}

// baselineDims sets some baseline dimensions on jd and returns the
// ExpiringDimensions which Info().GetDimensions() should now return.
func baselineDims(jd *Definition) ExpiringDimensions {
	toSet := ExpiringDimensions{
		"key": []ExpiringValue{
			{Value: "B", Expiration: swSlice2Exp},
			{Value: "Z"},
			{Value: "A", Expiration: swSlice1Exp},
			{Value: "AA", Expiration: swSlice1Exp},
			{Value: "C", Expiration: swSlice3Exp},
		},
	}

	SoEdit(jd, func(je Editor) {
		// set in a non-sorted order
		je.SetDimensions(toSet)
	})

	// Info().GetDimensions() always returns filled expirations.
	toSet["key"][1].Expiration = swSlice3Exp

	sort.Slice(toSet["key"], func(i, j int) bool {
		return toSet["key"][i].Value < toSet["key"][j].Value
	})

	return toSet
}

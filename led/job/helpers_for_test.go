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

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	api "go.chromium.org/luci/swarming/proto/api_v2"
)

type testCase struct {
	name string
	fn   func(*ftt.Test, *Definition)

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
		sw.Task = &api.NewTaskRequest{
			Name: "default-task-name",
		}

		for _, exp := range sliceExps {
			sw.Task.TaskSlices = append(sw.Task.TaskSlices, &api.TaskSlice{
				ExpirationSecs: int32(exp.Seconds()),
			})
		}
	}
	return ret
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
	ftt.Run(opName, t, func(t *ftt.Test) {
		t.Run(`bb`, func(t *ftt.Test) {
			for _, tc := range tests {
				t.Run(tc.name, func(t *ftt.Test) {
					if tc.skipBB {
						t.Skip("skipBB")
					}
					tc.fn(t, testBBJob(tc.v2Build))
				})
			}
		})
		t.Run(`sw (empty)`, func(t *ftt.Test) {
			for _, tc := range tests {
				t.Run(tc.name, func(t *ftt.Test) {
					if tc.skipSW {
						t.Skip("skipSW")
					}
					if tc.skipSWEmpty {
						t.Skip("skipSWEmpty")
					}
					tc.fn(t, testSWJob())
				})
			}
		})
		t.Run(`sw (slices)`, func(t *ftt.Test) {
			for _, tc := range tests {
				t.Run(tc.name, func(t *ftt.Test) {
					if tc.skipSW {
						t.Skip("skipSW")
					}
					if tc.skipSWSlices {
						t.Skip("skipSWSlices")
					}
					// Make a swarming job which expires in 600 seconds.
					tc.fn(t, testSWJob(
						swSlice1Exp,
						swSlice2Exp-swSlice1Exp,
						swSlice3Exp-swSlice2Exp,
					))
				})
			}
		})
	})
}

func MustEdit(t testing.TB, jd *Definition, cb func(Editor)) {
	t.Helper()
	assert.Loosely(t, jd.Edit(cb), should.BeNil, truth.LineContext())
}

func MustHLEdit(t testing.TB, jd *Definition, cb func(HighLevelEditor)) {
	t.Helper()
	assert.Loosely(t, jd.HighLevelEdit(cb), should.BeNil, truth.LineContext())
}

func mustGetDimensions(t testing.TB, jd *Definition) ExpiringDimensions {
	t.Helper()
	ret, err := jd.Info().Dimensions()
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return ret
}

// baselineDims sets some baseline dimensions on jd and returns the
// ExpiringDimensions which Info().GetDimensions() should now return.
func baselineDims(t testing.TB, jd *Definition) ExpiringDimensions {
	t.Helper()

	toSet := ExpiringDimensions{
		"key": []ExpiringValue{
			{Value: "B", Expiration: swSlice2Exp},
			{Value: "Z"},
			{Value: "A", Expiration: swSlice1Exp},
			{Value: "AA", Expiration: swSlice1Exp},
			{Value: "C", Expiration: swSlice3Exp},
		},
	}

	MustEdit(t, jd, func(je Editor) {
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

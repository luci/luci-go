// Copyright 2019 The LUCI Authors.
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

package results

import (
	"testing"

	resultspb "go.chromium.org/luci/results/proto/v1"
	"go.chromium.org/luci/results/util"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInvocationUtils(t *testing.T) {
	Convey(`Normalization works`, t, func() {
		inv := &resultspb.Invocation{
			Tests: []*resultspb.Invocation_Test{
				{
					Path: "c4/c2/t1",
					Variants: []*resultspb.Invocation_TestVariant{
						{
							VariantId: "d16357edf00d",
							Results: []*resultspb.TestResult{
								{
									ResultId: "1",
									Tags: util.StringPairs(
										"k2", "v3",
										"k1", "v1",
										"k2", "v2",
									),
								},
								{
									ResultId: "2",
									Tags: util.StringPairs(
										"k2", "v2",
										"k3", "v3",
										"k1", "v1",
									),
								},
							},
						},
						{
							VariantId: "600df00d",
						},
						{
							VariantId: "baadc0ffee",
						},
					},
				},
				{
					Path: "c3/c1/t2",
				},
				{
					Path: "c4/c1/t3",
				},
			},
			Tags: util.StringPairs(
				"k2", "v21",
				"k2", "v20",
				"k3", "v30",
				"k1", "v1",
				"k3", "v31",
			),
		}

		NormalizeInvocation(inv)

		Convey(`with ordered tags`, func() {
			So(inv.Tags, ShouldResembleProto, util.StringPairs(
				"k1", "v1",
				"k2", "v20",
				"k2", "v21",
				"k3", "v30",
				"k3", "v31",
			))
		})

		Convey(`with ordered tests`, func() {
			testPaths := make([]string, len(inv.Tests))
			for i, test := range inv.Tests {
				testPaths[i] = test.Path
			}
			So(testPaths, ShouldResemble, []string{
				"c3/c1/t2",
				"c4/c1/t3",
				"c4/c2/t1",
			})

			Convey(`and ordered variants`, func() {
				test := inv.Tests[2]
				So(test, ShouldNotBeNil)
				testVariants := make([]string, len(test.Variants))
				for i, variant := range test.Variants {
					testVariants[i] = variant.VariantId
				}
				So(testVariants, ShouldResemble, []string{
					"600df00d",
					"baadc0ffee",
					"d16357edf00d",
				})
			})
		})
	})
}

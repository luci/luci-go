// Copyright 2022 The LUCI Authors.
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

package graph

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestReachableInvocations(t *testing.T) {
	Convey(`ReachableInvocations`, t, func() {
		invs := NewReachableInvocations()

		src1 := testutil.TestSourcesWithChangelistNumbers(12)
		src2 := testutil.TestSourcesWithChangelistNumbers(13)
		invs.Sources[HashSources(src1)] = src1
		invs.Sources[HashSources(src2)] = src2

		invs.Invocations["0"] = ReachableInvocation{HasTestResults: true, HasTestExonerations: true, Realm: "testproject:testrealmA", SourceHash: HashSources(src1)}
		invs.Invocations["1"] = ReachableInvocation{HasTestResults: true, HasTestExonerations: false, Realm: "testproject:testrealmB"}
		invs.Invocations["2"] = ReachableInvocation{HasTestResults: true, HasTestExonerations: true, Realm: "testproject:testrealmC"}
		invs.Invocations["3"] = ReachableInvocation{HasTestResults: false, HasTestExonerations: false, Realm: "testproject:testrealmC", SourceHash: HashSources(src1)}
		invs.Invocations["4"] = ReachableInvocation{HasTestResults: false, HasTestExonerations: true, Realm: "testproject:testrealmB", SourceHash: HashSources(src2)}
		invs.Invocations["5"] = ReachableInvocation{HasTestResults: false, HasTestExonerations: false, Realm: "testproject:testrealmA"}

		Convey(`Batches`, func() {
			results := invs.batches(2)
			So(results[0].Invocations, ShouldResemble, map[invocations.ID]ReachableInvocation{
				"3": {HasTestResults: false, HasTestExonerations: false, Realm: "testproject:testrealmC", SourceHash: HashSources(src1)},
				"4": {HasTestResults: false, HasTestExonerations: true, Realm: "testproject:testrealmB", SourceHash: HashSources(src2)},
			})
			So(results[0].Sources, ShouldHaveLength, 2)
			So(results[0].Sources[HashSources(src1)], ShouldResembleProto, src1)
			So(results[0].Sources[HashSources(src2)], ShouldResembleProto, src2)

			So(results[1].Invocations, ShouldResemble, map[invocations.ID]ReachableInvocation{
				"0": {HasTestResults: true, HasTestExonerations: true, Realm: "testproject:testrealmA", SourceHash: HashSources(src1)},
				"1": {HasTestResults: true, HasTestExonerations: false, Realm: "testproject:testrealmB"},
			})
			So(results[1].Sources, ShouldHaveLength, 1)
			So(results[1].Sources[HashSources(src1)], ShouldResembleProto, src1)

			So(results[2].Invocations, ShouldResemble, map[invocations.ID]ReachableInvocation{
				"2": {HasTestResults: true, HasTestExonerations: true, Realm: "testproject:testrealmC"},
				"5": {HasTestResults: false, HasTestExonerations: false, Realm: "testproject:testrealmA"},
			})
			So(results[2].Sources, ShouldHaveLength, 0)
		})
		Convey(`Marshal and unmarshal`, func() {
			b, err := invs.marshal()
			So(err, ShouldBeNil)

			result, err := unmarshalReachableInvocations(b)
			So(err, ShouldBeNil)

			So(result.Invocations, ShouldResemble, invs.Invocations)
			So(result.Sources, ShouldHaveLength, len(invs.Sources))
			for key, value := range invs.Sources {
				So(result.Sources[key], ShouldResembleProto, value)
			}
		})
		Convey(`IDSet`, func() {
			invIDs, err := invs.IDSet()
			So(err, ShouldBeNil)
			So(invIDs, ShouldResemble, invocations.NewIDSet("0", "1", "2", "3", "4", "5"))
		})
		Convey(`WithTestResultsIDSet`, func() {
			invIDs, err := invs.WithTestResultsIDSet()
			So(err, ShouldBeNil)
			So(invIDs, ShouldResemble, invocations.NewIDSet("0", "1", "2"))
		})
		Convey(`WithExonerationsIDSet`, func() {
			invIDs, err := invs.WithExonerationsIDSet()
			So(err, ShouldBeNil)
			So(invIDs, ShouldResemble, invocations.NewIDSet("0", "2", "4"))
		})
	})
}

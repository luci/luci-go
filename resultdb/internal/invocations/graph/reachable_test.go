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

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReachableInvocations(t *testing.T) {
	Convey(`ReachableInvocations`, t, func() {
		invs := NewReachableInvocations()

		src1 := testutil.TestSourcesWithChangelistNumbers(12)
		src2 := testutil.TestSourcesWithChangelistNumbers(13)
		invs.Sources[HashSources(src1)] = src1
		invs.Sources[HashSources(src2)] = src2

		invs.Invocations["0"] = ReachableInvocation{HasTestResults: true, HasTestExonerations: true, Realm: "testproject:testrealmA", SourceHash: HashSources(src1), IncludedInvocationIDs: []invocations.ID{}}
		invs.Invocations["1"] = ReachableInvocation{HasTestResults: true, HasTestExonerations: false, Realm: "testproject:testrealmB", IncludedInvocationIDs: []invocations.ID{}}
		invs.Invocations["2"] = ReachableInvocation{HasTestResults: true, HasTestExonerations: true, Realm: "testproject:testrealmC", IncludedInvocationIDs: []invocations.ID{}}
		invs.Invocations["3"] = ReachableInvocation{HasTestResults: false, HasTestExonerations: false, Realm: "testproject:testrealmC", SourceHash: HashSources(src1), IncludedInvocationIDs: []invocations.ID{}}
		invs.Invocations["4"] = ReachableInvocation{HasTestResults: false, HasTestExonerations: true, Realm: "testproject:testrealmB", SourceHash: HashSources(src2), IncludedInvocationIDs: []invocations.ID{}}
		invs.Invocations["5"] = ReachableInvocation{HasTestResults: false, HasTestExonerations: false, Realm: "testproject:testrealmA", IncludedInvocationIDs: []invocations.ID{}}

		Convey(`Batches`, func() {
			results := invs.batches(2)
			So(results[0].Invocations, ShouldResemble, map[invocations.ID]ReachableInvocation{
				"3": {HasTestResults: false, HasTestExonerations: false, Realm: "testproject:testrealmC", SourceHash: HashSources(src1), IncludedInvocationIDs: []invocations.ID{}},
				"4": {HasTestResults: false, HasTestExonerations: true, Realm: "testproject:testrealmB", SourceHash: HashSources(src2), IncludedInvocationIDs: []invocations.ID{}},
			})
			So(results[0].Sources, ShouldHaveLength, 2)
			So(results[0].Sources[HashSources(src1)], ShouldResembleProto, src1)
			So(results[0].Sources[HashSources(src2)], ShouldResembleProto, src2)

			So(results[1].Invocations, ShouldResemble, map[invocations.ID]ReachableInvocation{
				"0": {HasTestResults: true, HasTestExonerations: true, Realm: "testproject:testrealmA", SourceHash: HashSources(src1), IncludedInvocationIDs: []invocations.ID{}},
				"1": {HasTestResults: true, HasTestExonerations: false, Realm: "testproject:testrealmB", IncludedInvocationIDs: []invocations.ID{}},
			})
			So(results[1].Sources, ShouldHaveLength, 1)
			So(results[1].Sources[HashSources(src1)], ShouldResembleProto, src1)

			So(results[2].Invocations, ShouldResemble, map[invocations.ID]ReachableInvocation{
				"2": {HasTestResults: true, HasTestExonerations: true, Realm: "testproject:testrealmC", IncludedInvocationIDs: []invocations.ID{}},
				"5": {HasTestResults: false, HasTestExonerations: false, Realm: "testproject:testrealmA", IncludedInvocationIDs: []invocations.ID{}},
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
	Convey(`Union`, t, func() {
		a := NewReachableInvocations()
		a.Invocations = map[invocations.ID]ReachableInvocation{
			"inv1": {
				HasTestResults:        true,
				SourceHash:            "source1",
				Realm:                 "realm1",
				IncludedInvocationIDs: []invocations.ID{"inv2"},
			},
		}
		a.Sources = map[SourceHash]*pb.Sources{
			"hash1": {
				GitilesCommit: &pb.GitilesCommit{
					Host: "host1",
				},
			},
		}
		b := NewReachableInvocations()
		b.Invocations = map[invocations.ID]ReachableInvocation{
			"inv2": {
				HasTestResults:        true,
				SourceHash:            "source2",
				Realm:                 "realm2",
				IncludedInvocationIDs: []invocations.ID{"inv3"},
			},
		}
		b.Sources = map[SourceHash]*pb.Sources{
			"hash2": {
				GitilesCommit: &pb.GitilesCommit{
					Host: "host2",
				},
			},
		}
		a.Union(b)
		So(a, ShouldResembleProto, ReachableInvocations{
			Invocations: map[invocations.ID]ReachableInvocation{
				"inv1": {
					HasTestResults:        true,
					SourceHash:            "source1",
					Realm:                 "realm1",
					IncludedInvocationIDs: []invocations.ID{"inv2"},
				},
				"inv2": {
					HasTestResults:        true,
					SourceHash:            "source2",
					Realm:                 "realm2",
					IncludedInvocationIDs: []invocations.ID{"inv3"},
				},
			},
			Sources: map[SourceHash]*pb.Sources{
				"hash1": {
					GitilesCommit: &pb.GitilesCommit{
						Host: "host1",
					},
				},
				"hash2": {
					GitilesCommit: &pb.GitilesCommit{
						Host: "host2",
					},
				},
			},
		})
	})
	Convey(`InstructionMap`, t, func() {
		invs := NewReachableInvocations()

		invs.Invocations["inv0"] = ReachableInvocation{
			IncludedInvocationIDs: []invocations.ID{"inv1", "inv2", "inv6", "inv8"},
			Instructions: &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:   "instruction0",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						InstructionFilter: &pb.InstructionFilter{
							FilterType: &pb.InstructionFilter_InvocationIds{
								InvocationIds: &pb.InstructionFilterByInvocationID{
									InvocationIds: []string{
										"inv1",
									},
									// Recursive. Should affect inv1, inv3.
									Recursive: true,
								},
							},
						},
					},
					{
						Id:   "instruction1",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						InstructionFilter: &pb.InstructionFilter{
							FilterType: &pb.InstructionFilter_InvocationIds{
								InvocationIds: &pb.InstructionFilterByInvocationID{
									InvocationIds: []string{
										// Not recursive. Should only affect inv6, not inv7.
										"inv6",
									},
								},
							},
						},
					},
					{
						Id: "instruction2",
						// Step instruction should have no effect
						Type: pb.InstructionType_STEP_INSTRUCTION,
					},
					{
						Id:   "instruction3",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						InstructionFilter: &pb.InstructionFilter{
							FilterType: &pb.InstructionFilter_InvocationIds{
								InvocationIds: &pb.InstructionFilterByInvocationID{
									InvocationIds: []string{
										"some non existent instruction",
									},
								},
							},
						},
					},
				},
			},
		}

		invs.Invocations["inv1"] = ReachableInvocation{
			IncludedInvocationIDs: []invocations.ID{"inv3"},
		}

		invs.Invocations["inv2"] = ReachableInvocation{
			IncludedInvocationIDs: []invocations.ID{"inv4", "inv5"},
			Instructions: &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:   "instruction0",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						// No filter. Should affect inv2, inv4, inv5.
					},
				},
			},
		}

		invs.Invocations["inv3"] = ReachableInvocation{}
		invs.Invocations["inv4"] = ReachableInvocation{}
		invs.Invocations["inv5"] = ReachableInvocation{}
		invs.Invocations["inv6"] = ReachableInvocation{
			IncludedInvocationIDs: []invocations.ID{"inv7"},
		}
		invs.Invocations["inv7"] = ReachableInvocation{}
		invs.Invocations["inv8"] = ReachableInvocation{}

		instructionMap, err := invs.InstructionMap()
		So(err, ShouldBeNil)
		So(instructionMap, ShouldResembleProto, map[invocations.ID]*pb.VerdictInstruction{
			"inv1": {
				Instruction: "invocations/inv0/instructions/instruction0",
			},
			"inv3": {
				Instruction: "invocations/inv0/instructions/instruction0",
			},
			"inv6": {
				Instruction: "invocations/inv0/instructions/instruction1",
			},
			"inv2": {
				Instruction: "invocations/inv2/instructions/instruction0",
			},
			"inv4": {
				Instruction: "invocations/inv2/instructions/instruction0",
			},
			"inv5": {
				Instruction: "invocations/inv2/instructions/instruction0",
			},
		})
	})
}

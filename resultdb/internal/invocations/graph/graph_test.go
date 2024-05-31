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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReachable(t *testing.T) {
	Convey(`Reachable`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		read := func(roots ...invocations.ID) (ReachableInvocations, error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return Reachable(ctx, invocations.NewIDSet(roots...))
		}

		mustRead := func(roots ...invocations.ID) ReachableInvocations {
			invs, err := read(roots...)
			So(err, ShouldBeNil)
			return invs
		}

		instructions := &pb.Instructions{
			Instructions: []*pb.Instruction{
				{
					Id:   "step_instruction",
					Type: pb.InstructionType_STEP_INSTRUCTION,
				},
				{
					Id:   "test_instruction",
					Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "content",
							Dependencies: []*pb.InstructionDependency{
								{
									InvocationId:  "inv",
									InstructionId: "ins",
								},
							},
						},
					},
					InstructionFilter: &pb.InstructionFilter{
						FilterType: &pb.InstructionFilter_InvocationIds{
							InvocationIds: &pb.InstructionFilterByInvocationID{
								InvocationIds: []string{"some inv"},
							},
						},
					},
				},
			},
		}

		filteredInstruction := &pb.Instructions{
			Instructions: []*pb.Instruction{
				{
					Id:   "test_instruction",
					Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Dependencies: []*pb.InstructionDependency{
								{
									InvocationId:  "inv",
									InstructionId: "ins",
								},
							},
						},
					},
					InstructionFilter: &pb.InstructionFilter{
						FilterType: &pb.InstructionFilter_InvocationIds{
							InvocationIds: &pb.InstructionFilterByInvocationID{
								InvocationIds: []string{"some inv"},
							},
						},
					},
				},
			},
		}

		Convey(`a -> []`, func() {
			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						Realm:                 insert.TestRealm,
						IncludedInvocationIDs: []invocations.ID{},
						Instructions:          filteredInstruction,
					},
				},
				Sources: make(map[SourceHash]*pb.Sources),
			}
			Convey(`Root has no sources`, func() {
				testutil.MustApply(ctx, node("a", newExtraValueBuilder().withInstructions(instructions).Build())...)

				So(mustRead("a"), ShouldResembleReachable, expected)
			})
			Convey(`Root has no instructions`, func() {
				testutil.MustApply(ctx, node("a", nil)...)

				So(mustRead("a"), ShouldResembleReachable, ReachableInvocations{
					Invocations: map[invocations.ID]ReachableInvocation{
						"a": {
							Realm:                 insert.TestRealm,
							IncludedInvocationIDs: []invocations.ID{},
						},
					},
					Sources: make(map[SourceHash]*pb.Sources),
				})
			})

			Convey(`Root has inherit sources`, func() {
				testutil.MustApply(ctx, node("a", newExtraValueBuilder().withInheritSources().withInstructions(instructions).Build())...)

				So(mustRead("a"), ShouldResembleReachable, expected)
			})
			Convey(`Root has concrete sources`, func() {
				testutil.MustApply(ctx, node("a", newExtraValueBuilder().withSources(1).withInstructions(instructions).Build())...)

				expected.Invocations["a"] = ReachableInvocation{
					Realm:                 insert.TestRealm,
					SourceHash:            HashSources(sources(1)),
					IncludedInvocationIDs: []invocations.ID{},
					Instructions:          filteredInstruction,
				}
				expected.Sources[HashSources(sources(1))] = sources(1)

				So(mustRead("a"), ShouldResembleReachable, expected)
			})
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", newExtraValueBuilder().withSources(1).Build(), "b", "c"),
				node("b", newExtraValueBuilder().withInheritSources().Build()),
				node("c", nil),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults("c", "Z", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestExonerations("c", "Z", nil, pb.ExonerationReason_NOT_CRITICAL),
			)...)

			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						HasTestExonerations:   true,
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(1)),
						IncludedInvocationIDs: []invocations.ID{"b", "c"},
					},
					"b": {
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(1)),
						IncludedInvocationIDs: []invocations.ID{},
					},
					"c": {
						HasTestResults:        true,
						HasTestExonerations:   true,
						Realm:                 insert.TestRealm,
						IncludedInvocationIDs: []invocations.ID{},
					},
				},
				Sources: map[SourceHash]*pb.Sources{
					HashSources(sources(1)): sources(1),
				},
			}

			So(mustRead("a"), ShouldResembleReachable, expected)
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", newExtraValueBuilder().withSources(1).Build(), "b"),
				node("b", newExtraValueBuilder().withInheritSources().Build(), "c"),
				node("c", newExtraValueBuilder().withInheritSources().Build()),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults("c", "Z", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestExonerations("c", "Z", nil, pb.ExonerationReason_NOT_CRITICAL),
			)...)
			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						HasTestExonerations:   true,
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(1)),
						IncludedInvocationIDs: []invocations.ID{"b"},
					},
					"b": {
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(1)),
						IncludedInvocationIDs: []invocations.ID{"c"},
					},
					"c": {
						HasTestResults:        true,
						HasTestExonerations:   true,
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(1)),
						IncludedInvocationIDs: []invocations.ID{},
					},
				},
				Sources: map[SourceHash]*pb.Sources{
					HashSources(sources(1)): sources(1),
				},
			}

			So(mustRead("a"), ShouldResembleReachable, expected)
		})

		Convey(`a -> [b1 -> b2, c, d] -> e`, func() {
			// e is included through three paths:
			// a -> b1 -> b2 -> e
			// a -> c -> e
			// a -> d -> e
			//
			// As e is set to inherit sources, the sources
			// resolved for e shall be from one of these three
			// paths. In practice we advise clients not to
			// use multiple inclusion paths like this.
			//
			// We test here that we behave deterministically,
			// selecting the invocation to inherit based on:
			// 1. Shortest path to the root, then
			// 2. Minimal invocation name.
			// In this case, e should inherit sources from c.
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", newExtraValueBuilder().withSources(1).Build(), "b1", "c", "d"),
				node("b1", newExtraValueBuilder().withInheritSources().Build(), "b2"),
				node("b2", newExtraValueBuilder().withInheritSources().Build(), "e"),
				node("c", newExtraValueBuilder().withSources(2).Build(), "e"),
				node("d", newExtraValueBuilder().withSources(3).Build(), "e"),
				node("e", newExtraValueBuilder().withInheritSources().Build()),
			)...)

			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(1)),
						IncludedInvocationIDs: []invocations.ID{"b1", "c", "d"},
					},
					"b1": {
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(1)),
						IncludedInvocationIDs: []invocations.ID{"b2"},
					},
					"b2": {
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(1)),
						IncludedInvocationIDs: []invocations.ID{"e"},
					},
					"c": {
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(2)),
						IncludedInvocationIDs: []invocations.ID{"e"},
					},
					"d": {
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(3)),
						IncludedInvocationIDs: []invocations.ID{"e"},
					},
					"e": {
						Realm:                 insert.TestRealm,
						SourceHash:            HashSources(sources(2)),
						IncludedInvocationIDs: []invocations.ID{},
					},
				},
				Sources: map[SourceHash]*pb.Sources{
					HashSources(sources(1)): sources(1),
					HashSources(sources(2)): sources(2),
					HashSources(sources(3)): sources(3),
				},
			}

			So(mustRead("a"), ShouldResembleReachable, expected)
		})
		Convey(`a -> b -> a`, func() {
			// Test a graph with cycles to make sure
			// source resolution always terminates.
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", newExtraValueBuilder().withInheritSources().Build(), "b"),
				node("b", newExtraValueBuilder().withInheritSources().Build(), "a"),
			)...)

			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						Realm:                 insert.TestRealm,
						IncludedInvocationIDs: []invocations.ID{"b"},
					},
					"b": {
						Realm:                 insert.TestRealm,
						IncludedInvocationIDs: []invocations.ID{"a"},
					},
				},
				Sources: map[SourceHash]*pb.Sources{},
			}

			So(mustRead("a"), ShouldResembleReachable, expected)
		})

		Convey(`a -> [100 invocations]`, func() {
			nodes := [][]*spanner.Mutation{}
			nodeSet := []invocations.ID{}
			childInvs := []invocations.ID{}
			for i := 0; i < 100; i++ {
				name := invocations.ID("b" + strconv.FormatInt(int64(i), 10))
				childInvs = append(childInvs, name)
				nodes = append(nodes, node(name, nil))
				nodes = append(nodes, insert.TestResults(string(name), "testID", nil, pb.TestStatus_SKIP))
				nodes = append(nodes, insert.TestExonerations(name, "testID", nil, pb.ExonerationReason_NOT_CRITICAL))
				nodeSet = append(nodeSet, name)
			}
			nodes = append(nodes, node("a", nil, nodeSet...))
			testutil.MustApply(ctx, testutil.CombineMutations(
				nodes...,
			)...)

			sort.Slice(childInvs, func(i, j int) bool {
				return childInvs[i] < childInvs[j]
			})
			expectedInvs := NewReachableInvocations()
			expectedInvs.Invocations["a"] = ReachableInvocation{
				Realm:                 insert.TestRealm,
				IncludedInvocationIDs: childInvs,
			}

			for _, id := range nodeSet {
				expectedInvs.Invocations[id] = ReachableInvocation{
					HasTestResults:        true,
					HasTestExonerations:   true,
					Realm:                 insert.TestRealm,
					IncludedInvocationIDs: []invocations.ID{},
				}
			}
			So(mustRead("a"), ShouldResembleReachable, expectedInvs)
		})
	})
}

func node(id invocations.ID, extraValues map[string]any, included ...invocations.ID) []*spanner.Mutation {
	return insert.InvocationWithInclusions(id, pb.Invocation_ACTIVE, extraValues, included...)
}

// BenchmarkChainFetch measures performance of a fetching a graph
// with a 10 linear inclusions.
func BenchmarkChainFetch(b *testing.B) {
	ctx := testutil.SpannerTestContext(b)

	var ms []*spanner.Mutation
	var prev invocations.ID
	for i := 0; i < 10; i++ {
		var included []invocations.ID
		if prev != "" {
			included = append(included, prev)
		}
		id := invocations.ID(fmt.Sprintf("inv%d", i))
		prev = id
		ms = append(ms, node(id, nil, included...)...)
	}

	if _, err := span.Apply(ctx, ms); err != nil {
		b.Fatal(err)
	}

	read := func() {
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		if _, err := Reachable(ctx, invocations.NewIDSet(prev)); err != nil {
			b.Fatal(err)
		}
	}

	// Run fetch a few times before starting measuring.
	for i := 0; i < 5; i++ {
		read()
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		read()
	}
}

type redisConn struct {
	redis.Conn
	reply    any
	replyErr error
	received [][]any
}

func (c *redisConn) Send(cmd string, args ...any) error {
	c.received = append(c.received, append([]any{cmd}, args...))
	return nil
}

func (c *redisConn) Do(cmd string, args ...any) (reply any, err error) {
	if cmd != "" {
		So(c.Send(cmd, args...), ShouldBeNil)
	}
	return c.reply, c.replyErr
}

func (c *redisConn) Err() error { return nil }

func (c *redisConn) Close() error { return nil }

func TestReachCache(t *testing.T) {
	t.Parallel()

	Convey(`TestReachCache`, t, func(c C) {
		ctx := testutil.TestingContext()

		// Stub Redis.
		conn := &redisConn{}
		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return conn, nil
			},
		})

		cache := reachCache("inv")

		invs := NewReachableInvocations()

		source1 := &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "myproject.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: strings.Repeat("a", 40),
				Position:   105,
			},
		}
		invs.Sources[HashSources(source1)] = source1

		invs.Invocations["inv"] = ReachableInvocation{
			HasTestResults:      true,
			HasTestExonerations: true,
			Realm:               insert.TestRealm,
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
								},
							},
						},
					},
				},
			},
			IncludedInvocationIDs: []invocations.ID{"a", "b"},
		}
		invs.Invocations["a"] = ReachableInvocation{
			HasTestResults:        true,
			Realm:                 insert.TestRealm,
			SourceHash:            HashSources(source1),
			IncludedInvocationIDs: []invocations.ID{},
		}
		invs.Invocations["b"] = ReachableInvocation{
			HasTestExonerations:   true,
			Realm:                 insert.TestRealm,
			IncludedInvocationIDs: []invocations.ID{},
		}

		Convey(`Read`, func() {
			var err error
			conn.reply, err = invs.marshal()
			So(err, ShouldBeNil)
			actual, err := cache.Read(ctx)
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, invs)
			So(conn.received, ShouldResemble, [][]any{
				{"GET", "reach4:inv"},
			})
		})

		Convey(`Read, cache miss`, func() {
			conn.replyErr = redis.ErrNil
			_, err := cache.Read(ctx)
			So(err, ShouldEqual, ErrUnknownReach)
		})

		Convey(`Write`, func() {
			err := cache.Write(ctx, invs)
			So(err, ShouldBeNil)

			So(conn.received, ShouldResemble, [][]any{
				{"SET", "reach4:inv", conn.received[0][2]},
				{"EXPIRE", "reach4:inv", 2592000},
			})
			actual, err := unmarshalReachableInvocations(conn.received[0][2].([]byte))
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, invs)
		})
	})
}

func ShouldResembleReachable(actual any, expected ...any) string {
	a, ok := actual.(ReachableInvocations)
	if !ok {
		return "expected actual to be of type ReachableInvocations"
	}
	if len(expected) != 1 {
		return "expected expected to be of length one"
	}
	e, ok := expected[0].(ReachableInvocations)
	if !ok {
		return "expected expected to be of type ReachableInvocations"
	}

	if msg := ShouldResembleProto(a.Invocations, e.Invocations); msg != "" {
		return msg
	}
	if msg := ShouldEqual(len(a.Sources), len(e.Sources)); msg != "" {
		return fmt.Sprintf("comparing sources: %s", msg)
	}
	for key := range e.Sources {
		if msg := ShouldResembleProto(a.Sources[key], e.Sources[key]); msg != "" {
			return fmt.Sprintf("comparing sources[%s]: %s", key, msg)
		}
	}
	return ""
}

type ExtraValueBuilder struct {
	values map[string]any
}

func newExtraValueBuilder() *ExtraValueBuilder {
	return &ExtraValueBuilder{
		values: map[string]any{},
	}
}

func (b *ExtraValueBuilder) withInheritSources() *ExtraValueBuilder {
	b.values["InheritSources"] = true
	return b
}

func (b *ExtraValueBuilder) withSources(number int) *ExtraValueBuilder {
	b.values["Sources"] = spanutil.Compress(pbutil.MustMarshal(sources(number)))
	return b
}

func (b *ExtraValueBuilder) withInstructions(instructions *pb.Instructions) *ExtraValueBuilder {
	b.values["Instructions"] = spanutil.Compressed(pbutil.MustMarshal(instructions))
	return b
}

func (b *ExtraValueBuilder) Build() map[string]any {
	return b.values
}

func sources(number int) *pb.Sources {
	return testutil.TestSourcesWithChangelistNumbers(number)
}

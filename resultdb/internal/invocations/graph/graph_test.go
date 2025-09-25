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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestReachable(t *testing.T) {
	ftt.Run(`Reachable`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		read := func(roots ...invocations.ID) (ReachableInvocations, error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return Reachable(ctx, invocations.NewIDSet(roots...))
		}

		mustRead := func(roots ...invocations.ID) ReachableInvocations {
			invs, err := read(roots...)
			assert.Loosely(t, err, should.BeNil)
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

		t.Run(`a -> []`, func(t *ftt.Test) {
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
			t.Run(`Root has no sources`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t, node("a", newExtraValueBuilder().withInstructions(instructions).Build())...)

				ShouldBeReachable(t, mustRead("a"), expected)
			})
			t.Run(`Root has no instructions`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t, node("a", nil)...)

				ShouldBeReachable(t, mustRead("a"), ReachableInvocations{
					Invocations: map[invocations.ID]ReachableInvocation{
						"a": {
							Realm:                 insert.TestRealm,
							IncludedInvocationIDs: []invocations.ID{},
						},
					},
					Sources: make(map[SourceHash]*pb.Sources),
				})
			})

			t.Run(`Root has inherit sources`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t, node("a", newExtraValueBuilder().withInheritSources().withInstructions(instructions).Build())...)

				ShouldBeReachable(t, mustRead("a"), expected)
			})
			t.Run(`Root has concrete sources`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t, node("a", newExtraValueBuilder().withSources(1).withInstructions(instructions).Build())...)

				expected.Invocations["a"] = ReachableInvocation{
					Realm:                 insert.TestRealm,
					SourceHash:            HashSources(sources(1)),
					IncludedInvocationIDs: []invocations.ID{},
					Instructions:          filteredInstruction,
				}
				expected.Sources[HashSources(sources(1))] = sources(1)

				ShouldBeReachable(t, mustRead("a"), expected)
			})
		})

		t.Run(`a -> [b, c]`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				node("a", newExtraValueBuilder().withSources(1).Build(), "b", "c"),
				node("b", newExtraValueBuilder().withInheritSources().Build()),
				node("c", nil),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults(t, "c", "Z", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
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

			ShouldBeReachable(t, mustRead("a"), expected)
		})

		t.Run(`a -> b -> c`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				node("a", newExtraValueBuilder().withSources(1).Build(), "b"),
				node("b", newExtraValueBuilder().withInheritSources().Build(), "c"),
				node("c", newExtraValueBuilder().withInheritSources().Build()),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults(t, "c", "Z", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
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

			ShouldBeReachable(t, mustRead("a"), expected)
		})

		t.Run(`a -> [b1 -> b2, c, d] -> e`, func(t *ftt.Test) {
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
			testutil.MustApply(ctx, t, testutil.CombineMutations(
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

			ShouldBeReachable(t, mustRead("a"), expected)
		})
		t.Run(`a -> b -> a`, func(t *ftt.Test) {
			// Test a graph with cycles to make sure
			// source resolution always terminates.
			testutil.MustApply(ctx, t, testutil.CombineMutations(
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

			ShouldBeReachable(t, mustRead("a"), expected)
		})

		t.Run(`a -> [100 invocations]`, func(t *ftt.Test) {
			nodes := [][]*spanner.Mutation{}
			nodeSet := []invocations.ID{}
			childInvs := []invocations.ID{}
			for i := range 100 {
				name := invocations.ID("b" + strconv.FormatInt(int64(i), 10))
				childInvs = append(childInvs, name)
				nodes = append(nodes, node(name, nil))
				nodes = append(nodes, insert.TestResults(t, name, "testID", nil, pb.TestResult_SKIPPED))
				nodes = append(nodes, insert.TestExonerations(name, "testID", nil, pb.ExonerationReason_NOT_CRITICAL))
				nodeSet = append(nodeSet, name)
			}
			nodes = append(nodes, node("a", nil, nodeSet...))
			testutil.MustApply(ctx, t, testutil.CombineMutations(
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
			ShouldBeReachable(t, mustRead("a"), expectedInvs)
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
	for i := range 10 {
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
	for range 5 {
		read()
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		read()
	}
}

type redisConn struct {
	redis.Conn
	t        testing.TB
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
		assert.Loosely(c.t, c.Send(cmd, args...), should.BeNil)
	}
	return c.reply, c.replyErr
}

func (c *redisConn) Err() error { return nil }

func (c *redisConn) Close() error { return nil }

func TestReachCache(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestReachCache`, t, func(c *ftt.Test) {
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
			BaseSources: &pb.Sources_GitilesCommit{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "myproject.googlesource.com",
					Project:    "myproject/src",
					Ref:        "refs/heads/main",
					CommitHash: strings.Repeat("a", 40),
					Position:   105,
				},
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

		c.Run(`Read`, func(c *ftt.Test) {
			var err error
			conn.reply, err = invs.marshal()
			assert.Loosely(c, err, should.BeNil)
			actual, err := cache.Read(ctx)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, actual, should.Match(invs))
			assert.Loosely(c, conn.received, should.Match([][]any{
				{"GET", "reach4:inv"},
			}))
		})

		c.Run(`Read, cache miss`, func(c *ftt.Test) {
			conn.replyErr = redis.ErrNil
			_, err := cache.Read(ctx)
			assert.Loosely(c, err, should.Equal(ErrUnknownReach))
		})

		c.Run(`Write`, func(c *ftt.Test) {
			err := cache.Write(ctx, invs)
			assert.Loosely(c, err, should.BeNil)

			assert.Loosely(c, conn.received, should.Match([][]any{
				{"SET", "reach4:inv", conn.received[0][2]},
				{"EXPIRE", "reach4:inv", 2592000},
			}))
			actual, err := unmarshalReachableInvocations(conn.received[0][2].([]byte))
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, actual, should.Match(invs))
		})
	})
}

func ShouldBeReachable(t testing.TB, actual, expected ReachableInvocations) {
	t.Helper()
	assert.That(t, actual.Invocations, should.Match(expected.Invocations), truth.LineContext())
	assert.Loosely(t, actual.Sources, should.HaveLength(len(expected.Sources)), truth.LineContext())
	for key := range expected.Sources {
		assert.That(t, actual.Sources[key], should.Match(expected.Sources[key]), truth.LineContext())
	}
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

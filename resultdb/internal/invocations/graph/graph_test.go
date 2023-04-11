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
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/span"

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

		readBatched := func(roots ...invocations.ID) (ReachableInvocations, error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			invs := NewReachableInvocations()
			process := func(ctx context.Context, batch ReachableInvocations) error {
				invs.Union(batch)
				return nil
			}
			if err := BatchedReachable(ctx, invocations.NewIDSet(roots...), process); err != nil {
				return ReachableInvocations{}, err
			}
			return invs, nil
		}

		mustReadBatched := func(roots ...invocations.ID) ReachableInvocations {
			invs, err := readBatched(roots...)
			So(err, ShouldBeNil)
			return invs
		}

		withInheritSources := map[string]any{
			"InheritSources": true,
		}
		sources := func(number int) *pb.Sources {
			return testutil.TestSourcesWithChangelistNumbers(number)
		}
		withSources := func(number int) map[string]any {
			return map[string]any{
				"Sources": spanutil.Compress(pbutil.MustMarshal(sources(number))),
			}
		}

		Convey(`a -> []`, func() {
			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						Realm: insert.TestRealm,
					},
				},
				Sources: make(map[SourceHash]*pb.Sources),
			}
			Convey(`Root has no sources`, func() {
				testutil.MustApply(ctx, node("a", nil)...)
				Convey(`Simple`, func() {
					So(mustRead("a"), ShouldResembleReachable, expected)
				})
				Convey(`Batched`, func() {
					So(mustReadBatched("a"), ShouldResembleReachable, expected)
				})
			})
			Convey(`Root has inherit sources`, func() {
				testutil.MustApply(ctx, node("a", withInheritSources)...)
				Convey(`Simple`, func() {
					So(mustRead("a"), ShouldResembleReachable, expected)
				})
				Convey(`Batched`, func() {
					So(mustReadBatched("a"), ShouldResembleReachable, expected)
				})
			})
			Convey(`Root has concrete sources`, func() {
				testutil.MustApply(ctx, node("a", withSources(1))...)

				expected.Invocations["a"] = ReachableInvocation{
					Realm:      insert.TestRealm,
					SourceHash: hashSources(sources(1)),
				}
				expected.Sources[hashSources(sources(1))] = sources(1)

				Convey(`Simple`, func() {
					So(mustRead("a"), ShouldResembleReachable, expected)
				})
				Convey(`Batched`, func() {
					So(mustReadBatched("a"), ShouldResembleReachable, expected)
				})
			})
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", withSources(1), "b", "c"),
				node("b", withInheritSources),
				node("c", nil),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults("c", "Z", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestExonerations("c", "Z", nil, pb.ExonerationReason_NOT_CRITICAL),
			)...)

			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						HasTestExonerations: true,
						Realm:               insert.TestRealm,
						SourceHash:          hashSources(sources(1)),
					},
					"b": {
						Realm:      insert.TestRealm,
						SourceHash: hashSources(sources(1)),
					},
					"c": {
						HasTestResults:      true,
						HasTestExonerations: true,
						Realm:               insert.TestRealm,
					},
				},
				Sources: map[SourceHash]*pb.Sources{
					hashSources(sources(1)): sources(1),
				},
			}

			Convey(`Simple`, func() {
				So(mustRead("a"), ShouldResembleReachable, expected)
			})
			Convey(`Batched`, func() {
				So(mustReadBatched("a"), ShouldResembleReachable, expected)
			})
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", withSources(1), "b"),
				node("b", withInheritSources, "c"),
				node("c", withInheritSources),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults("c", "Z", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestExonerations("c", "Z", nil, pb.ExonerationReason_NOT_CRITICAL),
			)...)
			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						HasTestExonerations: true,
						Realm:               insert.TestRealm,
						SourceHash:          hashSources(sources(1)),
					},
					"b": {
						Realm:      insert.TestRealm,
						SourceHash: hashSources(sources(1)),
					},
					"c": {
						HasTestResults:      true,
						HasTestExonerations: true,
						Realm:               insert.TestRealm,
						SourceHash:          hashSources(sources(1)),
					},
				},
				Sources: map[SourceHash]*pb.Sources{
					hashSources(sources(1)): sources(1),
				},
			}

			Convey(`Simple`, func() {
				So(mustRead("a"), ShouldResembleReachable, expected)
			})
			Convey(`Batched`, func() {
				So(mustReadBatched("a"), ShouldResembleReachable, expected)
			})
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
				node("a", withSources(1), "b1", "c", "d"),
				node("b1", withInheritSources, "b2"),
				node("b2", withInheritSources, "e"),
				node("c", withSources(2), "e"),
				node("d", withSources(3), "e"),
				node("e", withInheritSources),
			)...)

			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						Realm:      insert.TestRealm,
						SourceHash: hashSources(sources(1)),
					},
					"b1": {
						Realm:      insert.TestRealm,
						SourceHash: hashSources(sources(1)),
					},
					"b2": {
						Realm:      insert.TestRealm,
						SourceHash: hashSources(sources(1)),
					},
					"c": {
						Realm:      insert.TestRealm,
						SourceHash: hashSources(sources(2)),
					},
					"d": {
						Realm:      insert.TestRealm,
						SourceHash: hashSources(sources(3)),
					},
					"e": {
						Realm:      insert.TestRealm,
						SourceHash: hashSources(sources(2)),
					},
				},
				Sources: map[SourceHash]*pb.Sources{
					hashSources(sources(1)): sources(1),
					hashSources(sources(2)): sources(2),
					hashSources(sources(3)): sources(3),
				},
			}

			Convey(`Simple`, func() {
				So(mustRead("a"), ShouldResembleReachable, expected)
			})
			Convey(`Batched`, func() {
				So(mustReadBatched("a"), ShouldResembleReachable, expected)
			})
		})
		Convey(`a -> b -> a`, func() {
			// Test a graph with cycles to make sure
			// source resolution always terminates.
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", withInheritSources, "b"),
				node("b", withInheritSources, "a"),
			)...)

			expected := ReachableInvocations{
				Invocations: map[invocations.ID]ReachableInvocation{
					"a": {
						Realm: insert.TestRealm,
					},
					"b": {
						Realm: insert.TestRealm,
					},
				},
				Sources: map[SourceHash]*pb.Sources{},
			}

			Convey(`Simple`, func() {
				So(mustRead("a"), ShouldResembleReachable, expected)
			})
			Convey(`Batched`, func() {
				So(mustReadBatched("a"), ShouldResembleReachable, expected)
			})
		})

		Convey(`a -> [100 invocations]`, func() {
			nodes := [][]*spanner.Mutation{}
			nodeSet := []invocations.ID{}
			for i := 0; i < 100; i++ {
				name := invocations.ID("b" + strconv.FormatInt(int64(i), 10))
				nodes = append(nodes, node(name, nil))
				nodes = append(nodes, insert.TestResults(string(name), "testID", nil, pb.TestStatus_SKIP))
				nodes = append(nodes, insert.TestExonerations(name, "testID", nil, pb.ExonerationReason_NOT_CRITICAL))
				nodeSet = append(nodeSet, name)
			}
			nodes = append(nodes, node("a", nil, nodeSet...))
			testutil.MustApply(ctx, testutil.CombineMutations(
				nodes...,
			)...)
			expectedInvs := NewReachableInvocations()
			expectedInvs.Invocations["a"] = ReachableInvocation{
				Realm: insert.TestRealm,
			}
			for _, id := range nodeSet {
				expectedInvs.Invocations[id] = ReachableInvocation{
					HasTestResults:      true,
					HasTestExonerations: true,
					Realm:               insert.TestRealm,
				}
			}
			Convey(`Single`, func() {
				So(mustRead("a"), ShouldResembleReachable, expectedInvs)
			})
			Convey(`Batched`, func() {
				So(mustReadBatched("a"), ShouldResembleReachable, expectedInvs)
			})
		})
		Convey(`errors passed through`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", nil, "b", "c"),
				node("b", nil),
				node("c", nil),
			)...)
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			process := func(ctx context.Context, batch ReachableInvocations) error {
				return fmt.Errorf("expected error")
			}
			err := BatchedReachable(ctx, invocations.NewIDSet("a"), process)
			So(err, ShouldNotEqual, nil)
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
		invs.Sources[hashSources(source1)] = source1

		invs.Invocations["inv"] = ReachableInvocation{
			HasTestResults:      true,
			HasTestExonerations: true,
			Realm:               insert.TestRealm,
		}
		invs.Invocations["a"] = ReachableInvocation{
			HasTestResults: true,
			Realm:          insert.TestRealm,
			SourceHash:     hashSources(source1),
		}
		invs.Invocations["b"] = ReachableInvocation{
			HasTestExonerations: true,
			Realm:               insert.TestRealm,
		}

		Convey(`Read`, func() {
			var err error
			conn.reply, err = invs.marshal()
			So(err, ShouldBeNil)
			actual, err := cache.Read(ctx)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, invs)
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
			So(actual, ShouldResemble, invs)
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

	if msg := ShouldResemble(a.Invocations, e.Invocations); msg != "" {
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

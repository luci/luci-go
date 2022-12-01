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
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/gomodule/redigo/redis"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/span"
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

		Convey(`a -> []`, func() {
			testutil.MustApply(ctx, node("a")...)
			expected := ReachableInvocations{
				"a": ReachableInvocation{},
			}
			So(mustRead("a"), ShouldResemble, expected)
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", "b", "c"),
				node("b"),
				node("c"),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults("c", "Z", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestExonerations("c", "Z", nil, pb.ExonerationReason_NOT_CRITICAL),
			)...)
			expected := ReachableInvocations{
				"a": ReachableInvocation{
					HasTestExonerations: true,
				},
				"b": ReachableInvocation{},
				"c": ReachableInvocation{
					HasTestResults:      true,
					HasTestExonerations: true,
				},
			}
			So(mustRead("a"), ShouldResemble, expected)
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", "b"),
				node("b", "c"),
				node("c"),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults("c", "Z", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestExonerations("c", "Z", nil, pb.ExonerationReason_NOT_CRITICAL),
			)...)
			expected := ReachableInvocations{
				"a": ReachableInvocation{
					HasTestExonerations: true,
				},
				"b": ReachableInvocation{},
				"c": ReachableInvocation{
					HasTestResults:      true,
					HasTestExonerations: true,
				},
			}
			So(mustRead("a"), ShouldResemble, expected)
		})
	})
}

func node(id invocations.ID, included ...invocations.ID) []*spanner.Mutation {
	return insert.InvocationWithInclusions(id, pb.Invocation_ACTIVE, nil, included...)
}

func TestBatchedReachable(t *testing.T) {
	Convey(`Reachable`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		node := func(id invocations.ID, included ...invocations.ID) []*spanner.Mutation {
			return insert.InvocationWithInclusions(id, pb.Invocation_ACTIVE, nil, included...)
		}

		read := func(roots ...invocations.ID) (ReachableInvocations, error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			invs := NewReachableInvocations()
			process := func(ctx context.Context, batch ReachableInvocations) error {
				invs.Union(batch)
				return nil
			}
			if err := BatchedReachable(ctx, invocations.NewIDSet(roots...), process); err != nil {
				return nil, err
			}
			return invs, nil
		}

		mustRead := func(roots ...invocations.ID) ReachableInvocations {
			invs, err := read(roots...)
			So(err, ShouldBeNil)
			return invs
		}

		Convey(`a -> []`, func() {
			testutil.MustApply(ctx, insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil)...)
			expected := ReachableInvocations{
				"a": ReachableInvocation{},
			}
			So(mustRead("a"), ShouldResemble, expected)
		})

		Convey(`a -> [b, c]`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", "b", "c"),
				node("b"),
				node("c"),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults("c", "Z", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestExonerations("c", "Z", nil, pb.ExonerationReason_NOT_CRITICAL),
			)...)
			expected := ReachableInvocations{
				"a": ReachableInvocation{
					HasTestExonerations: true,
				},
				"b": ReachableInvocation{},
				"c": ReachableInvocation{
					HasTestResults:      true,
					HasTestExonerations: true,
				},
			}
			So(mustRead("a"), ShouldResemble, expected)
		})

		Convey(`a -> b -> c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", "b"),
				node("b", "c"),
				node("c"),
				insert.TestExonerations("a", "Z", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestResults("c", "Z", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestExonerations("c", "Z", nil, pb.ExonerationReason_NOT_CRITICAL),
			)...)
			expected := ReachableInvocations{
				"a": ReachableInvocation{
					HasTestExonerations: true,
				},
				"b": ReachableInvocation{},
				"c": ReachableInvocation{
					HasTestResults:      true,
					HasTestExonerations: true,
				},
			}
			So(mustRead("a"), ShouldResemble, expected)
		})
		Convey(`a -> [100 invocations]`, func() {
			nodes := [][]*spanner.Mutation{}
			nodeSet := []invocations.ID{}
			for i := 0; i < 100; i++ {
				name := invocations.ID("b" + strconv.FormatInt(int64(i), 10))
				nodes = append(nodes, node(name))
				nodes = append(nodes, insert.TestResults(string(name), "testID", nil, pb.TestStatus_SKIP))
				nodes = append(nodes, insert.TestExonerations(name, "testID", nil, pb.ExonerationReason_NOT_CRITICAL))
				nodeSet = append(nodeSet, name)
			}
			nodes = append(nodes, node("a", nodeSet...))
			testutil.MustApply(ctx, testutil.CombineMutations(
				nodes...,
			)...)
			expectedInvs := NewReachableInvocations()
			expectedInvs["a"] = ReachableInvocation{}
			for _, id := range nodeSet {
				expectedInvs[id] = ReachableInvocation{
					HasTestResults:      true,
					HasTestExonerations: true,
				}
			}
			So(mustRead("a"), ShouldResemble, expectedInvs)
		})
		Convey(`errors passed through`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				node("a", "b", "c"),
				node("b"),
				node("c"),
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
		ms = append(ms, node(id, included...)...)
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
	reply    interface{}
	replyErr error
	received [][]interface{}
}

func (c *redisConn) Send(cmd string, args ...interface{}) error {
	c.received = append(c.received, append([]interface{}{cmd}, args...))
	return nil
}

func (c *redisConn) Do(cmd string, args ...interface{}) (reply interface{}, err error) {
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
		invs["inv"] = ReachableInvocation{
			HasTestResults:      true,
			HasTestExonerations: true,
		}
		invs["a"] = ReachableInvocation{
			HasTestResults: true,
		}
		invs["b"] = ReachableInvocation{
			HasTestExonerations: true,
		}

		Convey(`Read`, func() {
			var err error
			conn.reply, err = invs.marshal()
			So(err, ShouldBeNil)
			actual, err := cache.Read(ctx)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, invs)
			So(conn.received, ShouldResemble, [][]interface{}{
				{"GET", "reach2:inv"},
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

			So(conn.received, ShouldResemble, [][]interface{}{
				{"SET", "reach2:inv", conn.received[0][2]},
				{"EXPIRE", "reach2:inv", 2592000},
			})
			actual, err := unmarshalReachableInvocations(conn.received[0][2].([]byte))
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, invs)
		})
	})
}

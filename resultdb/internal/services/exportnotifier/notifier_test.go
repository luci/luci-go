// Copyright 2024 The LUCI Authors.
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

package exportnotifier

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/exportroots"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const testHostname = "test.resultdb.deployment.luci.app"

func TestPropagate(t *testing.T) {
	ftt.Run(`Propagate`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)

		sources := testutil.TestSourcesWithChangelistNumbers(1)
		rootCreateTime := timestamppb.New(time.Unix(1000, 0))
		_, err := span.Apply(ctx, testutil.CombineMutations(
			insert.InvocationWithInclusions("inv-root-a", pb.Invocation_ACTIVE, map[string]any{
				"Realm":        "projecta:root",
				"IsExportRoot": true,
				"Sources":      spanutil.Compressed(pbutil.MustMarshal(sources)),
				"CreateTime":   rootCreateTime,
			}, "inv-a"),
			insert.InvocationWithInclusions("inv-a", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}, "inv-a1", "inv-a2"),
			insert.InvocationWithInclusions("inv-a1", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}),
			insert.InvocationWithInclusions("inv-a2", pb.Invocation_ACTIVE, map[string]any{}),
		))
		assert.Loosely(t, err, should.BeNil)

		t.Run(`If there are no roots, does nothing`, func(t *ftt.Test) {
			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-a",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-a")))
			assert.Loosely(t, notifications, should.BeEmpty)

			roots, err := exportroots.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, roots, should.BeEmpty)
		})
		t.Run(`If there is a root, propogates roots to children (recursively)`, func(t *ftt.Test) {
			// Create the root for inv-root-a.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			assert.Loosely(t, err, should.BeNil)

			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("inv-root-a").WithoutInheritedSources().WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-a1").WithRootInvocation("inv-root-a").WithoutInheritedSources().WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-a2").WithRootInvocation("inv-root-a").WithoutInheritedSources().WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
			}

			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-a",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-a", "inv-a", "inv-a1", "inv-a2")))
			assert.Loosely(t, sentMessages(sched), should.BeEmpty)
			assert.Loosely(t, notifications, should.BeEmpty)

			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.Match(expectedRoots))

			t.Run(`Repeated propagation stops early`, func(t *ftt.Test) {
				visited, notifications, err := propagateRecursive(ctx, sched, task)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-a")))
				assert.Loosely(t, notifications, should.BeEmpty)

				got, err := exportroots.ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match(expectedRoots))
			})
			t.Run(`Setting sources as final, inherited sources are propagated and notifications are sent`, func(t *ftt.Test) {
				_, err := span.Apply(ctx, []*spanner.Mutation{
					spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-root-a"), "IsSourceSpecFinal": true}),
					spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-a"), "State": pb.Invocation_FINALIZING}),
					spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-a1"), "State": pb.Invocation_FINALIZED}),
				})
				assert.Loosely(t, err, should.BeNil)

				task := &taskspb.RunExportNotifications{
					InvocationId: "inv-root-a",
				}

				visited, notifications, err := propagateRecursive(ctx, sched, task)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-a", "inv-a", "inv-a1", "inv-a2")))
				assert.Loosely(t, notifications, should.Match([]*pb.InvocationReadyForExportNotification{
					{
						ResultdbHost:        testHostname,
						RootInvocation:      "invocations/inv-root-a",
						RootInvocationRealm: "projecta:root",
						Invocation:          "invocations/inv-a",
						InvocationRealm:     "testproject:testrealm",
						Sources:             sources,
						RootCreateTime:      rootCreateTime,
					},
					{
						ResultdbHost:        testHostname,
						RootInvocation:      "invocations/inv-root-a",
						RootInvocationRealm: "projecta:root",
						Invocation:          "invocations/inv-a1",
						InvocationRealm:     "testproject:testrealm",
						Sources:             sources,
						RootCreateTime:      rootCreateTime,
					},
				}))

				got, err := exportroots.ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.HaveLength(4))

				// Accept notified time determined by the implementation, as it is a function of commit time.
				expectedRoots := []exportroots.ExportRoot{
					exportroots.NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithNotified(got[0].NotifiedTime).Build(),
					exportroots.NewBuilder(1).WithInvocation("inv-a1").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithNotified(got[1].NotifiedTime).Build(),
					exportroots.NewBuilder(1).WithInvocation("inv-a2").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithoutNotified().Build(),
					exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
				}
				assert.Loosely(t, got, should.Match(expectedRoots))

				t.Run(`Repeated propagation does nothing`, func(t *ftt.Test) {
					visited, notifications, err := propagateRecursive(ctx, sched, task)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-a")))
					assert.Loosely(t, notifications, should.BeEmpty)

					got, err := exportroots.ReadAllForTesting(span.Single(ctx))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Match(expectedRoots))
				})
				t.Run(`Repeated propagation of notified invocation does nothing`, func(t *ftt.Test) {
					task := &taskspb.RunExportNotifications{
						InvocationId: "inv-a",
					}

					visited, notifications, err := propagateRecursive(ctx, sched, task)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-a")))
					assert.Loosely(t, notifications, should.BeEmpty)

					got, err := exportroots.ReadAllForTesting(span.Single(ctx))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Match(expectedRoots))
				})
			})
		})
		t.Run(`If there is a root and sources are final already, roots are propogated with sources and notifications are sent immediately`, func(t *ftt.Test) {
			// Mark sources on inv-root-a final, and mark inv-a and inv-a1 final.
			// Sources should propogate through inv-a to inv-a1 and inv-a2, and
			// pub/sub notification should be sent for inv-a and inv-a1.
			_, err := span.Apply(ctx, []*spanner.Mutation{
				spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-root-a"), "IsSourceSpecFinal": true}),
				spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-a"), "State": pb.Invocation_FINALIZING}),
				spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-a1"), "State": pb.Invocation_FINALIZED}),
			})
			assert.Loosely(t, err, should.BeNil)

			// Create the root for inv-root-a.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			assert.Loosely(t, err, should.BeNil)

			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-a",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-a", "inv-a", "inv-a1", "inv-a2")))
			assert.Loosely(t, notifications, should.Match([]*pb.InvocationReadyForExportNotification{
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-a",
					RootInvocationRealm: "projecta:root",
					Invocation:          "invocations/inv-a",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
					RootCreateTime:      rootCreateTime,
				},
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-a",
					RootInvocationRealm: "projecta:root",
					Invocation:          "invocations/inv-a1",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
					RootCreateTime:      rootCreateTime,
				},
			}))

			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.HaveLength(4))

			// Accept notified time determined by the implementation, as it is a function of commit time.
			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithNotified(got[0].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-a1").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithNotified(got[1].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-a2").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			assert.Loosely(t, got, should.Match(expectedRoots))
		})
		t.Run(`Graph cycles do not result in an infinite propagation loop`, func(t *ftt.Test) {
			_, err := span.Apply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("inv-root-c", pb.Invocation_ACTIVE, map[string]any{
					"IsExportRoot":      true,
					"IsSourceSpecFinal": true,
					"Sources":           spanutil.Compressed(pbutil.MustMarshal(sources)),
				}, "inv-c1"),
				insert.InvocationWithInclusions("inv-c1", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true, "IsSourceSpecFinal": true}, "inv-c2"),
				insert.InvocationWithInclusions("inv-c2", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true, "IsSourceSpecFinal": true}, "inv-c1"),
			))
			assert.Loosely(t, err, should.BeNil)

			// Create the root for inv-root-c.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-c").WithRootInvocation("inv-root-c").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			assert.Loosely(t, err, should.BeNil)

			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-c",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-c", "inv-c1", "inv-c2")))
			assert.Loosely(t, notifications, should.BeEmpty)

			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-c1").WithRootInvocation("inv-root-c").WithInheritedSources(sources).WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-c2").WithRootInvocation("inv-root-c").WithInheritedSources(sources).WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-c").WithRootInvocation("inv-root-c").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.Match(expectedRoots))

			t.Run(`Repeated propagation stops early`, func(t *ftt.Test) {
				visited, notifications, err := propagateRecursive(ctx, sched, task)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-c")))
				assert.Loosely(t, notifications, should.BeEmpty)
			})
		})
		t.Run(`Directed propagation works`, func(t *ftt.Test) {
			_, err := span.Apply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("inv-root-d", pb.Invocation_FINALIZING, map[string]any{
					"IsExportRoot": true,
					"Sources":      spanutil.Compressed(pbutil.MustMarshal(sources)),
					"CreateTime":   rootCreateTime,
				}, "inv-d1", "inv-d2"),
				insert.InvocationWithInclusions("inv-d1", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}, "inv-d1a"),
				insert.InvocationWithInclusions("inv-d1a", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}),
				insert.InvocationWithInclusions("inv-d2", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}),
			))
			assert.Loosely(t, err, should.BeNil)

			// Create the root for inv-root-d.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-d").WithRootInvocation("inv-root-d").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			assert.Loosely(t, err, should.BeNil)

			// Perform directed propagation for inv-root-d towards inv-d1.
			// This might be expected if we just included inv-d1 inside inv-root-d.
			task := &taskspb.RunExportNotifications{
				InvocationId:          "inv-root-d",
				IncludedInvocationIds: []string{"inv-d1"},
			}

			// Expect propagation to inv-d1, inv-d1a but not to inv-d2.
			visited, notifications, err := propagateRecursive(ctx, sched, task)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-d", "inv-d1", "inv-d1a")))
			assert.Loosely(t, notifications, should.Match([]*pb.InvocationReadyForExportNotification{
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-d",
					RootInvocationRealm: "testproject:testrealm",
					Invocation:          "invocations/inv-root-d",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
					RootCreateTime:      rootCreateTime,
				},
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-d",
					RootInvocationRealm: "testproject:testrealm",
					Invocation:          "invocations/inv-d1",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
					RootCreateTime:      rootCreateTime,
				},
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-d",
					RootInvocationRealm: "testproject:testrealm",
					Invocation:          "invocations/inv-d1a",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
					RootCreateTime:      rootCreateTime,
				},
			}))

			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.HaveLength(3))

			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-d1").WithRootInvocation("inv-root-d").WithInheritedSources(sources).WithNotified(got[0].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-d1a").WithRootInvocation("inv-root-d").WithInheritedSources(sources).WithNotified(got[1].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-d").WithRootInvocation("inv-root-d").WithInheritedSources(nil).WithNotified(got[2].NotifiedTime).Build(),
			}
			assert.Loosely(t, got, should.Match(expectedRoots))
		})
		t.Run(`Invocation with explicit sources`, func(t *ftt.Test) {
			_, err := span.Apply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("inv-root-e", pb.Invocation_ACTIVE, map[string]any{
					"IsExportRoot": true,
					"Realm":        "projecte:root",
					"CreateTime":   rootCreateTime,
				}, "inv-e"),
				insert.InvocationWithInclusions("inv-e", pb.Invocation_FINALIZING, map[string]any{
					"Sources": spanutil.Compressed(pbutil.MustMarshal(sources)),
					"Realm":   "projecte:included",
				}),
			))
			assert.Loosely(t, err, should.BeNil)

			// Create the root for inv-root-e.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-e").WithRootInvocation("inv-root-e").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			assert.Loosely(t, err, should.BeNil)

			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-e",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, visited, should.Match(invocations.NewIDSet("inv-root-e", "inv-e")))
			assert.Loosely(t, notifications, should.Match([]*pb.InvocationReadyForExportNotification{
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-e",
					RootInvocationRealm: "projecte:root",
					Invocation:          "invocations/inv-e",
					InvocationRealm:     "projecte:included",
					Sources:             sources,
					RootCreateTime:      rootCreateTime,
				},
			}))

			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.HaveLength(2))

			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-e").WithRootInvocation("inv-root-e").WithoutInheritedSources().WithNotified(got[0].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-e").WithRootInvocation("inv-root-e").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			assert.Loosely(t, got, should.Match(expectedRoots))
		})
	})
}

func propagateRecursive(ctx context.Context, sched *tqtesting.Scheduler, task *taskspb.RunExportNotifications) (visited invocations.IDSet, sentMessages []*pb.InvocationReadyForExportNotification, err error) {
	index := len(sched.Tasks().Payloads())

	options := Options{
		ResultDBHostname: testHostname,
	}
	err = propagate(ctx, task, options)
	if err != nil {
		return nil, nil, errors.Fmt("invocation %s: %w", task.InvocationId, err)
	}

	visited = invocations.NewIDSet(invocations.ID(task.InvocationId))
	toDo := sched.Tasks().Payloads()[index:]
	for len(toDo) > 0 {
		payload := toDo[0]
		task, ok := payload.(*taskspb.RunExportNotifications)
		if ok {
			invID := invocations.ID(task.InvocationId)
			if visited.Has(invID) {
				// Avoid getting stuck in infite loops by erroring out if we try
				// to visti the same invocation more than once.
				return nil, nil, errors.Fmt("visited %v twice", invID)
			}
			visited.Add(invID)

			err := propagate(ctx, task, options)
			if err != nil {
				return nil, nil, errors.Fmt("invocation %s: %w", task.InvocationId, err)
			}
		}

		notification, ok := payload.(*taskspb.NotificationInvocationReadyForExport)
		if ok {
			sentMessages = append(sentMessages, notification.Message)
		}

		index++
		toDo = sched.Tasks().Payloads()[index:]
	}

	return visited, sentMessages, nil
}

func sentMessages(sched *tqtesting.Scheduler) []*pb.InvocationReadyForExportNotification {
	var result []*pb.InvocationReadyForExportNotification
	for _, payload := range sched.Tasks().Payloads() {
		task, ok := payload.(*taskspb.NotificationInvocationReadyForExport)
		if ok {
			result = append(result, task.Message)
		}
	}
	return result
}

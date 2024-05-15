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

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const testHostname = "test.resultdb.deployment.luci.app"

func TestPropagate(t *testing.T) {
	Convey(`Propagate`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)

		sources := testutil.TestSourcesWithChangelistNumbers(1)

		_, err := span.Apply(ctx, testutil.CombineMutations(
			insert.InvocationWithInclusions("inv-root-a", pb.Invocation_ACTIVE, map[string]any{
				"Realm":        "projecta:root",
				"IsExportRoot": true,
				"Sources":      spanutil.Compressed(pbutil.MustMarshal(sources)),
			}, "inv-a"),
			insert.InvocationWithInclusions("inv-a", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}, "inv-a1", "inv-a2"),
			insert.InvocationWithInclusions("inv-a1", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}),
			insert.InvocationWithInclusions("inv-a2", pb.Invocation_ACTIVE, map[string]any{}),
		))
		So(err, ShouldBeNil)

		Convey(`If there are no roots, does nothing`, func() {
			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-a",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			So(err, ShouldBeNil)
			So(visited, ShouldResemble, invocations.NewIDSet("inv-root-a"))
			So(notifications, ShouldBeEmpty)

			roots, err := exportroots.ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)
			So(roots, ShouldBeEmpty)
		})
		Convey(`If there is a root, propogates roots to children (recursively)`, func() {
			// Create the root for inv-root-a.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			So(err, ShouldBeNil)

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
			So(err, ShouldBeNil)
			So(visited, ShouldResemble, invocations.NewIDSet("inv-root-a", "inv-a", "inv-a1", "inv-a2"))
			So(sentMessages(sched), ShouldBeEmpty)
			So(notifications, ShouldBeEmpty)

			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)
			So(got, ShouldResembleProto, expectedRoots)

			Convey(`Repeated propagation stops early`, func() {
				visited, notifications, err := propagateRecursive(ctx, sched, task)
				So(err, ShouldBeNil)
				So(visited, ShouldResemble, invocations.NewIDSet("inv-root-a"))
				So(notifications, ShouldBeEmpty)

				got, err := exportroots.ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, expectedRoots)
			})
			Convey(`Setting sources as final, inherited sources are propagated and notifications are sent`, func() {
				_, err := span.Apply(ctx, []*spanner.Mutation{
					spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-root-a"), "IsSourceSpecFinal": true}),
					spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-a"), "State": pb.Invocation_FINALIZING}),
					spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-a1"), "State": pb.Invocation_FINALIZED}),
				})
				So(err, ShouldBeNil)

				task := &taskspb.RunExportNotifications{
					InvocationId: "inv-root-a",
				}

				visited, notifications, err := propagateRecursive(ctx, sched, task)
				So(err, ShouldBeNil)
				So(visited, ShouldResemble, invocations.NewIDSet("inv-root-a", "inv-a", "inv-a1", "inv-a2"))
				So(notifications, ShouldResembleProto, []*pb.InvocationReadyForExportNotification{
					{
						ResultdbHost:        testHostname,
						RootInvocation:      "invocations/inv-root-a",
						RootInvocationRealm: "projecta:root",
						Invocation:          "invocations/inv-a",
						InvocationRealm:     "testproject:testrealm",
						Sources:             sources,
					},
					{
						ResultdbHost:        testHostname,
						RootInvocation:      "invocations/inv-root-a",
						RootInvocationRealm: "projecta:root",
						Invocation:          "invocations/inv-a1",
						InvocationRealm:     "testproject:testrealm",
						Sources:             sources,
					},
				})

				got, err := exportroots.ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(got, ShouldHaveLength, 4)

				// Accept notified time determined by the implementation, as it is a function of commit time.
				expectedRoots := []exportroots.ExportRoot{
					exportroots.NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithNotified(got[0].NotifiedTime).Build(),
					exportroots.NewBuilder(1).WithInvocation("inv-a1").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithNotified(got[1].NotifiedTime).Build(),
					exportroots.NewBuilder(1).WithInvocation("inv-a2").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithoutNotified().Build(),
					exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
				}
				So(got, ShouldResembleProto, expectedRoots)

				Convey(`Repeated propagation does nothing`, func() {
					visited, notifications, err := propagateRecursive(ctx, sched, task)
					So(err, ShouldBeNil)
					So(visited, ShouldResemble, invocations.NewIDSet("inv-root-a"))
					So(notifications, ShouldBeEmpty)

					got, err := exportroots.ReadAllForTesting(span.Single(ctx))
					So(err, ShouldBeNil)
					So(got, ShouldResembleProto, expectedRoots)
				})
				Convey(`Repeated propagation of notified invocation does nothing`, func() {
					task := &taskspb.RunExportNotifications{
						InvocationId: "inv-a",
					}

					visited, notifications, err := propagateRecursive(ctx, sched, task)
					So(err, ShouldBeNil)
					So(visited, ShouldResemble, invocations.NewIDSet("inv-a"))
					So(notifications, ShouldBeEmpty)

					got, err := exportroots.ReadAllForTesting(span.Single(ctx))
					So(err, ShouldBeNil)
					So(got, ShouldResembleProto, expectedRoots)
				})
			})
		})
		Convey(`If there is a root and sources are final already, roots are propogated with sources and notifications are sent immediately`, func() {
			// Mark sources on inv-root-a final, and mark inv-a and inv-a1 final.
			// Sources should propogate through inv-a to inv-a1 and inv-a2, and
			// pub/sub notification should be sent for inv-a and inv-a1.
			_, err := span.Apply(ctx, []*spanner.Mutation{
				spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-root-a"), "IsSourceSpecFinal": true}),
				spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-a"), "State": pb.Invocation_FINALIZING}),
				spanutil.UpdateMap("Invocations", map[string]any{"InvocationId": invocations.ID("inv-a1"), "State": pb.Invocation_FINALIZED}),
			})
			So(err, ShouldBeNil)

			// Create the root for inv-root-a.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			So(err, ShouldBeNil)

			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-a",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			So(err, ShouldBeNil)
			So(visited, ShouldResemble, invocations.NewIDSet("inv-root-a", "inv-a", "inv-a1", "inv-a2"))
			So(notifications, ShouldResembleProto, []*pb.InvocationReadyForExportNotification{
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-a",
					RootInvocationRealm: "projecta:root",
					Invocation:          "invocations/inv-a",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
				},
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-a",
					RootInvocationRealm: "projecta:root",
					Invocation:          "invocations/inv-a1",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
				},
			})

			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)
			So(got, ShouldHaveLength, 4)

			// Accept notified time determined by the implementation, as it is a function of commit time.
			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithNotified(got[0].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-a1").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithNotified(got[1].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-a2").WithRootInvocation("inv-root-a").WithInheritedSources(sources).WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-a").WithRootInvocation("inv-root-a").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			So(got, ShouldResembleProto, expectedRoots)
		})
		Convey(`Graph cycles do not result in an infinite propagation loop`, func() {
			_, err := span.Apply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("inv-root-c", pb.Invocation_ACTIVE, map[string]any{
					"IsExportRoot":      true,
					"IsSourceSpecFinal": true,
					"Sources":           spanutil.Compressed(pbutil.MustMarshal(sources)),
				}, "inv-c1"),
				insert.InvocationWithInclusions("inv-c1", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true, "IsSourceSpecFinal": true}, "inv-c2"),
				insert.InvocationWithInclusions("inv-c2", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true, "IsSourceSpecFinal": true}, "inv-c1"),
			))
			So(err, ShouldBeNil)

			// Create the root for inv-root-c.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-c").WithRootInvocation("inv-root-c").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			So(err, ShouldBeNil)

			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-c",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			So(err, ShouldBeNil)
			So(visited, ShouldResemble, invocations.NewIDSet("inv-root-c", "inv-c1", "inv-c2"))
			So(notifications, ShouldBeEmpty)

			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-c1").WithRootInvocation("inv-root-c").WithInheritedSources(sources).WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-c2").WithRootInvocation("inv-root-c").WithInheritedSources(sources).WithoutNotified().Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-c").WithRootInvocation("inv-root-c").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)
			So(got, ShouldResembleProto, expectedRoots)

			Convey(`Repeated propagation stops early`, func() {
				visited, notifications, err := propagateRecursive(ctx, sched, task)
				So(err, ShouldBeNil)
				So(visited, ShouldResemble, invocations.NewIDSet("inv-root-c"))
				So(notifications, ShouldBeEmpty)
			})
		})
		Convey(`Directed propagation works`, func() {
			_, err := span.Apply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("inv-root-d", pb.Invocation_FINALIZING, map[string]any{
					"IsExportRoot": true,
					"Sources":      spanutil.Compressed(pbutil.MustMarshal(sources)),
				}, "inv-d1", "inv-d2"),
				insert.InvocationWithInclusions("inv-d1", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}, "inv-d1a"),
				insert.InvocationWithInclusions("inv-d1a", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}),
				insert.InvocationWithInclusions("inv-d2", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}),
			))
			So(err, ShouldBeNil)

			// Create the root for inv-root-d.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-d").WithRootInvocation("inv-root-d").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			So(err, ShouldBeNil)

			// Perform directed propagation for inv-root-d towards inv-d1.
			// This might be expected if we just included inv-d1 inside inv-root-d.
			task := &taskspb.RunExportNotifications{
				InvocationId:          "inv-root-d",
				IncludedInvocationIds: []string{"inv-d1"},
			}

			// Expect propagation to inv-d1, inv-d1a but not to inv-d2.
			visited, notifications, err := propagateRecursive(ctx, sched, task)
			So(err, ShouldBeNil)
			So(visited, ShouldResemble, invocations.NewIDSet("inv-root-d", "inv-d1", "inv-d1a"))
			So(notifications, ShouldResembleProto, []*pb.InvocationReadyForExportNotification{
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-d",
					RootInvocationRealm: "testproject:testrealm",
					Invocation:          "invocations/inv-root-d",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
				},
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-d",
					RootInvocationRealm: "testproject:testrealm",
					Invocation:          "invocations/inv-d1",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
				},
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-d",
					RootInvocationRealm: "testproject:testrealm",
					Invocation:          "invocations/inv-d1a",
					InvocationRealm:     "testproject:testrealm",
					Sources:             sources,
				},
			})

			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)
			So(got, ShouldHaveLength, 3)

			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-d1").WithRootInvocation("inv-root-d").WithInheritedSources(sources).WithNotified(got[0].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-d1a").WithRootInvocation("inv-root-d").WithInheritedSources(sources).WithNotified(got[1].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-d").WithRootInvocation("inv-root-d").WithInheritedSources(nil).WithNotified(got[2].NotifiedTime).Build(),
			}
			So(got, ShouldResembleProto, expectedRoots)
		})
		Convey(`Invocation with explicit sources`, func() {
			_, err := span.Apply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("inv-root-e", pb.Invocation_ACTIVE, map[string]any{
					"IsExportRoot": true,
					"Realm":        "projecte:root",
				}, "inv-e"),
				insert.InvocationWithInclusions("inv-e", pb.Invocation_FINALIZING, map[string]any{
					"Sources": spanutil.Compressed(pbutil.MustMarshal(sources)),
					"Realm":   "projecte:included",
				}),
			))
			So(err, ShouldBeNil)

			// Create the root for inv-root-e.
			roots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-root-e").WithRootInvocation("inv-root-e").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			err = exportroots.SetForTesting(ctx, roots)
			So(err, ShouldBeNil)

			task := &taskspb.RunExportNotifications{
				InvocationId: "inv-root-e",
			}

			visited, notifications, err := propagateRecursive(ctx, sched, task)
			So(err, ShouldBeNil)
			So(visited, ShouldResemble, invocations.NewIDSet("inv-root-e", "inv-e"))
			So(notifications, ShouldResembleProto, []*pb.InvocationReadyForExportNotification{
				{
					ResultdbHost:        testHostname,
					RootInvocation:      "invocations/inv-root-e",
					RootInvocationRealm: "projecte:root",
					Invocation:          "invocations/inv-e",
					InvocationRealm:     "projecte:included",
					Sources:             sources,
				},
			})

			got, err := exportroots.ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)
			So(got, ShouldHaveLength, 2)

			expectedRoots := []exportroots.ExportRoot{
				exportroots.NewBuilder(1).WithInvocation("inv-e").WithRootInvocation("inv-root-e").WithoutInheritedSources().WithNotified(got[0].NotifiedTime).Build(),
				exportroots.NewBuilder(1).WithInvocation("inv-root-e").WithRootInvocation("inv-root-e").WithInheritedSources(nil).WithoutNotified().Build(),
			}
			So(got, ShouldResembleProto, expectedRoots)
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
		return nil, nil, errors.Annotate(err, "invocation %s", task.InvocationId).Err()
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
				return nil, nil, errors.Reason("visited %v twice", invID).Err()
			}
			visited.Add(invID)

			err := propagate(ctx, task, options)
			if err != nil {
				return nil, nil, errors.Annotate(err, "invocation %s", task.InvocationId).Err()
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

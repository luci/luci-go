// Copyright 2026 The LUCI Authors.
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

package pubsub

import (
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestHandleRootInvocationPublisher(t *testing.T) {
	ftt.Run("HandleRootInvocationPublisher", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		// Set up a placeholder service config.
		cfg := config.CreatePlaceholderServiceConfig()
		err := config.SetServiceConfigForTesting(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		// Setup TQ.
		ctx, sched := tq.TestingContext(ctx, nil)

		// Create a root invocation.
		ri := rootinvocations.NewBuilder("root-inv").
			WithRealm("test:realm").
			WithState(pb.RootInvocation_RUNNING).
			WithFinalizationState(pb.RootInvocation_FINALIZING).
			WithCreatedBy("user:test@example.com").
			WithUninterestingTestVerdictsExpirationTime(spanner.NullTime{Time: time.Now().Add(time.Hour), Valid: true}).
			Build()
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(ri)...)
		testutil.MustApply(ctx, t, rootinvocations.MarkFinalized("root-inv")...)

		p := &rootInvocationPublisher{
			task: &taskspb.PublishRootInvocationTask{
				RootInvocationId: "root-inv",
			},
			resultDBHostname: "resultdb.example.com",
		}

		t.Run("Success", func(t *ftt.Test) {
			err := p.handleRootInvocationPublisher(ctx)
			assert.Loosely(t, err, should.BeNil)

			// Verify that a task was enqueued.
			// The publisher enqueues a NotifyRootInvocationFinalized task.
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*taskspb.NotifyRootInvocationFinalized])
			task := sched.Tasks().Payloads()[0].(*taskspb.NotifyRootInvocationFinalized)

			// Read the root invocation back to get the exact state (including timestamps).
			rootInv, err := rootinvocations.Read(span.Single(ctx), "root-inv")
			assert.Loosely(t, err, should.BeNil)

			compiledCfg, err := config.Service(ctx)
			assert.Loosely(t, err, should.BeNil)

			expectedMsg := &pb.RootInvocationFinalizedNotification{
				RootInvocation: masking.RootInvocation(rootInv, compiledCfg),
				ResultdbHost:   "resultdb.example.com",
			}
			assert.Loosely(t, task.Message, should.Match(expectedMsg))
		})

		t.Run("Not Finalized", func(t *ftt.Test) {
			// Create an active root invocation.
			riActive := rootinvocations.NewBuilder("root-inv-active").
				WithRealm("test:realm").
				WithState(pb.RootInvocation_RUNNING).
				WithFinalizationState(pb.RootInvocation_ACTIVE).
				WithCreatedBy("user:test@example.com").
				WithUninterestingTestVerdictsExpirationTime(spanner.NullTime{Time: time.Now().Add(time.Hour), Valid: true}).
				Build()
			testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(riActive)...)

			p := &rootInvocationPublisher{
				task: &taskspb.PublishRootInvocationTask{
					RootInvocationId: "root-inv-active",
				},
				resultDBHostname: "resultdb.example.com",
			}

			err := p.handleRootInvocationPublisher(ctx)
			assert.Loosely(t, err, should.ErrLike("root invocation \"rootInvocations/root-inv-active\" is not finalized"))

			// Verify that NO task was enqueued.
			assert.Loosely(t, len(sched.Tasks()), should.BeZero)
		})
	})
}

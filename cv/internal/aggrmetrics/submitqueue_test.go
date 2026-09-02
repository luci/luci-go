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

package aggrmetrics

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
)

func TestSubmitQueueAggregator(t *testing.T) {
	t.Parallel()

	ftt.Run("works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const lProject = "test-proj"
		run := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("deaddead"))
		noopNotifyFn := func(ctx context.Context, runID common.RunID, eta time.Time) error {
			return nil
		}

		sentValue := func(project string) any {
			return ct.TSMonSentValue(ctx, metrics.Public.SubmitQueueLength, project)
		}
		sa := submitQueueAggregator{}

		assert.NoErr(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			_, err := submit.TryAcquire(ctx, noopNotifyFn, run, nil)
			return err
		}, nil))
		assert.NoErr(t, sa.report(ctx, []string{lProject}))
		assert.That(t, sentValue(lProject).(int64), should.Equal(int64(1)))

		assert.NoErr(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			return submit.Release(ctx, noopNotifyFn, run)
		}, nil))
		assert.NoErr(t, sa.report(ctx, []string{lProject}))
		assert.That(t, sentValue(lProject).(int64), should.Equal(int64(0)))
	})
}

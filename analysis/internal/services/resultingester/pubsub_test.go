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

package resultingester

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/gerrit"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestHandlePubsub(t *testing.T) {
	ftt.Run(`TestHandlePubsub`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx) // For config cache.

		clsByHost := gerritChangesByHostForTesting()
		ctx = gerrit.UseFakeClient(ctx, clsByHost)

		testIngestor := &testIngester{}

		o := &orchestrator{}
		o.sinks = []IngestionSink{
			testIngestor,
		}

		notification := testTestResultNotification()

		expectedInputs := RootInvocationInputs{
			Notification: notification,
			Sources:      resolvedSourcesForTesting(),
		}

		cfg := &configpb.Config{}
		err := config.SetTestConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`Baseline`, func(t *ftt.Test) {
			err := o.HandlePubSub(ctx, notification)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, testIngestor.called, should.BeTrue)
			assert.Loosely(t, testIngestor.gotRootInvocationInputs, should.Match(expectedInputs))
		})
		t.Run(`Without actual sources`, func(t *ftt.Test) {
			notification.RootInvocationMetadata.Sources = &rdbpb.Sources{IsDirty: true}
			expectedInputs.Sources = &analysispb.Sources{IsDirty: true}

			err := o.HandlePubSub(ctx, notification)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, testIngestor.called, should.BeTrue)
			assert.Loosely(t, testIngestor.gotRootInvocationInputs, should.Match(expectedInputs))
		})
		t.Run(`Project not allowlisted for ingestion`, func(t *ftt.Test) {
			cfg.Ingestion = &configpb.Ingestion{
				ProjectAllowlistEnabled: true,
				ProjectAllowlist:        []string{"other"},
			}
			err := config.SetTestConfig(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			err = o.HandlePubSub(ctx, notification)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, testIngestor.called, should.BeFalse)
		})
	})
}

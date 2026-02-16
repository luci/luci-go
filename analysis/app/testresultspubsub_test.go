// Copyright 2025 The LUCI Authors.
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

package app

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/pubsub"

	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/gerrit"
	"go.chromium.org/luci/analysis/internal/services/resultingester"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

type mockSink struct {
	called   bool
	gotInput resultingester.RootInvocationInputs
}

func (m *mockSink) Name() string {
	return "mock-sink"
}

func (m *mockSink) IngestLegacy(ctx context.Context, input resultingester.LegacyInputs) error {
	return nil
}

func (m *mockSink) IngestRootInvocation(ctx context.Context, input resultingester.RootInvocationInputs) error {
	if m.called {
		return errors.Fmt("mockSink.IngestRootInvocation called twice")
	}
	m.called = true
	m.gotInput = input
	return nil
}

func TestTestResultsPubSubHandler(t *testing.T) {
	ftt.Run("TestResultsPubSubHandler", t, func(t *ftt.Test) {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		ctx = memory.Use(ctx)
		ctx = gerrit.UseFakeClient(ctx, nil)

		cfg, err := config.CreatePlaceholderConfig()
		assert.Loosely(t, err, should.BeNil)
		err = config.SetTestConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		sink := &mockSink{}
		o := resultingester.NewOrchestrator(sink)
		h := NewTestResultsPubSubHandler(o)

		t.Run("Handle", func(t *ftt.Test) {
			notification := &rdbpb.TestResultsNotification{
				RootInvocationMetadata: &rdbpb.RootInvocationMetadata{
					RootInvocationId: "inv",
					Realm:            "project:realm",
					Sources:          &rdbpb.Sources{},
				},
			}
			expectedInputs := resultingester.RootInvocationInputs{
				Notification: notification,
				Sources:      &analysispb.Sources{},
			}

			message := pubsub.Message{}
			err := h.Handle(ctx, message, notification)
			assert.NoErr(t, err)

			assert.Loosely(t, sink.called, should.BeTrue)
			assert.Loosely(t, sink.gotInput, should.Match(expectedInputs))
		})
	})
}

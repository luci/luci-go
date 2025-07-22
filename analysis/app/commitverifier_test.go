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

package app

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	cvv1 "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/server/pubsub"

	_ "go.chromium.org/luci/analysis/internal/services/verdictingester" // Needed to ensure task class is registered.
)

func TestCVRunHandler(t *testing.T) {
	ftt.Run(`Test CVRunHandler`, t, func(t *ftt.Test) {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())

		rID := "id_full_run"
		fullRunID := fullRunID("cvproject", rID)

		h := &CVRunHandler{}
		t.Run(`Valid message`, func(t *ftt.Test) {
			message := &cvv1.PubSubRun{
				Id:       fullRunID,
				Status:   cvv1.Run_SUCCEEDED,
				Hostname: "cvhost",
			}

			called := false
			var processed bool
			h.handleCVRun = func(ctx context.Context, psRun *cvv1.PubSubRun) (project string, wasProcessed bool, err error) {
				assert.Loosely(t, called, should.BeFalse)
				assert.Loosely(t, psRun, should.Match(message))

				called = true
				return "cvproject", processed, nil
			}

			t.Run(`Processed`, func(t *ftt.Test) {
				processed = true

				err := h.Handle(ctx, pubsub.Message{}, message)
				assert.NoErr(t, err)
				assert.Loosely(t, cvRunCounter.Get(ctx, "cvproject", "success"), should.Equal(1))
			})
			t.Run(`Not processed`, func(t *ftt.Test) {
				processed = false

				err := h.Handle(ctx, pubsub.Message{}, message)
				assert.That(t, pubsub.Ignore.In(err), should.BeTrue)
				assert.Loosely(t, cvRunCounter.Get(ctx, "cvproject", "ignored"), should.Equal(1))
			})
		})
	})
}

func makeCVRunReq(message *cvv1.PubSubRun) pubsub.Message {
	blob, _ := protojson.Marshal(message)
	return pubsub.Message{Data: blob}
}

func fullRunID(project, runID string) string {
	return fmt.Sprintf("projects/%s/runs/%s", project, runID)
}

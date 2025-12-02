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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/tsmon"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/pubsub"
)

func TestTestResultsPubSubHandler(t *testing.T) {
	ftt.Run("TestResultsPubSubHandler", t, func(t *ftt.Test) {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		h := NewTestResultsPubSubHandler()

		t.Run("Handle", func(t *ftt.Test) {
			notification := &rdbpb.TestResultsNotification{}
			message := pubsub.Message{}
			err := h.Handle(ctx, message, notification)
			assert.NoErr(t, err)
		})
	})
}

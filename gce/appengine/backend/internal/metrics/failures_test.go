// Copyright 2019 The LUCI Authors.
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

package metrics

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

func TestFailures(t *testing.T) {
	t.Parallel()

	ftt.Run("UpdateFailures", t, func(t *ftt.Test) {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		s := tsmon.Store(c)

		vm := &model.VM{
			Attributes: config.VM{
				Project: "project",
				Zone:    "us-central1-b",
			},
			Hostname: "name-1",
			Prefix:   "prefix",
		}

		exhaustedFields := []any{200, "prefix", "project", "us-central1-b", "ZONE_RESOURCE_POOL_EXHAUSTED"}
		UpdateFailures(c, 200, "ZONE_RESOURCE_POOL_EXHAUSTED", vm)
		assert.Loosely(t, s.Get(c, creationFailures, exhaustedFields).(int64), should.Equal(1))

		UpdateFailures(c, 200, "ZONE_RESOURCE_POOL_EXHAUSTED", vm)
		assert.Loosely(t, s.Get(c, creationFailures, exhaustedFields).(int64), should.Equal(2))

		unknownFields := []any{400, "prefix", "project", "us-central1-b", DefaultFailureReason}
		UpdateFailures(c, 400, "", vm)
		assert.Loosely(t, s.Get(c, creationFailures, unknownFields).(int64), should.Equal(1))

		fallbackFields := []any{"prefix", "project", "us-central1-b", "us-central1-f", "ZONE_RESOURCE_POOL_EXHAUSTED"}
		UpdateZoneFallback(c, vm, "us-central1-b", "us-central1-f", "ZONE_RESOURCE_POOL_EXHAUSTED")
		assert.Loosely(t, s.Get(c, zoneFallbacks, fallbackFields).(int64), should.Equal(1))
	})
}

// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package backend

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
)

type fakeBQDataset struct {
	result map[string]any
}

func (f *fakeBQDataset) putToTable(c context.Context, table string, src []proto.Message) error {
	f.result[table] = src
	return nil
}

func TestDumper(t *testing.T) {
	t.Parallel()

	ftt.Run("dumpDatastore", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		datastore.GetTestable(c).Consistent(true)
		bq := &fakeBQDataset{result: make(map[string]any)}
		t.Run("none", func(t *ftt.Test) {
			assert.Loosely(t, uploadToBQ(c, bq), should.BeNil)
			assert.Loosely(t, bq.result, should.Resemble(map[string]any{}))
		})
		t.Run("one config", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.Config{
				ID: "id",
			}), should.BeNil)

			assert.Loosely(t, uploadToBQ(c, bq), should.BeNil)
			assert.Loosely(t, bq.result["config"], should.HaveLength(1))
			assert.Loosely(t, bq.result["instance_count"], should.BeNil)
		})
		t.Run("one config one instance_count", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, &model.Config{
				ID: "id",
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(c, &metrics.InstanceCount{ID: "id"}), should.BeNil)

			assert.Loosely(t, uploadToBQ(c, bq), should.BeNil)
			assert.Loosely(t, bq.result["config"], should.HaveLength(1))
			assert.Loosely(t, bq.result["instance_count"], should.HaveLength(1))
		})
	})
}

// Copyright 2018 The LUCI Authors.
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

package admin

import (
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/admin/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
)

func TestEnumeratePackages(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx, admin, sched := SetupTest()
		ctx = memlogger.Use(ctx)
		log := logging.Get(ctx).(*memlogger.MemLogger)

		assert.Loosely(t, datastore.Put(ctx, []*model.Package{
			{Name: "a/b"},
			{Name: "a/b/c"},
		}), should.BeNil)

		_, err := RunMapper(ctx, admin, sched, &api.JobConfig{
			Kind: api.MapperKind_ENUMERATE_PACKAGES,
		})
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "Found package: a/b"))
		assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "Found package: a/b/c"))
	})
}

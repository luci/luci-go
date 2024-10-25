// Copyright 2017 The LUCI Authors.
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

package gitiles

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
)

func TestLoadSave(t *testing.T) {
	t.Parallel()

	ftt.Run("storeState/loadState work", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		repo := "https://example.googlesource.com/what/ever.git"
		jobID := "job"

		loadNoError := func(repo string) map[string]string {
			r, err := loadState(c, jobID, repo)
			if err != nil {
				panic(err)
			}
			return r
		}

		t.Run("load first time ever", func(t *ftt.Test) {
			assert.Loosely(t, loadNoError(repo), should.Resemble(map[string]string{}))
			assert.Loosely(t, loadNoError(repo), should.NotBeNil)
		})

		t.Run("save/load/save/load", func(t *ftt.Test) {
			assert.Loosely(t, saveState(c, jobID, repo, map[string]string{"refs/heads/master": "beefcafe"}), should.BeNil)
			assert.Loosely(t, loadNoError(repo), should.Resemble(map[string]string{"refs/heads/master": "beefcafe"}))
			assert.Loosely(t, saveState(c, jobID, repo, map[string]string{"refs/tails/master": "efacfeeb"}), should.BeNil)
			assert.Loosely(t, loadNoError(repo), should.Resemble(map[string]string{"refs/tails/master": "efacfeeb"}))
		})

		t.Run("save/change repo name/load", func(t *ftt.Test) {
			assert.Loosely(t, saveState(c, jobID, repo, map[string]string{"refs/heads/master": "beefcafe"}), should.BeNil)
			assert.Loosely(t, loadNoError("https://some-other.googlesource.com/repo"), should.Resemble(map[string]string{}))
		})

		t.Run("save/load with deeply nested refs", func(t *ftt.Test) {
			nested := map[string]string{
				"refs/weirdo":                  "00",
				"refs/heads/master":            "11",
				"refs/heads/branch":            "22",
				"refs/heads/infra/config":      "33",
				"refs/heads/infra/deploy":      "44",
				"refs/heads/infra/configs/why": "55",
				"refs/heads/infra/configs/not": "66",
			}
			assert.Loosely(t, saveState(c, jobID, repo, nested), should.BeNil)
			assert.Loosely(t, loadNoError(repo), should.Resemble(nested))
		})
	})
}

func TestLoadSaveCompression(t *testing.T) {
	t.Parallel()

	ftt.Run("Compress lots of similar refs", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c, _, _ = tsmon.WithFakes(c)
		tsmon.GetState(c).SetStore(store.NewInMemory(&target.Task{}))

		repo := "https://example.googlesource.com/what/ever.git"
		jobID := "job"

		many16Ki := 16 * 1024
		tags := make(map[string]string, many16Ki)
		for i := 0; i < many16Ki; i++ {
			ref := "refs/tags/" + strconv.FormatInt(int64(i), 20)
			hsh := fmt.Sprintf("%x", sha256.Sum256([]byte(ref)))[:40]
			tags[ref] = hsh
		}

		assert.Loosely(t, saveState(c, jobID, repo, tags), should.BeNil)
		id, err := repositoryID(jobID, repo)
		assert.Loosely(t, err, should.BeNil)
		stored := Repository{ID: id}
		assert.Loosely(t, ds.Get(c, &stored), should.BeNil)
		// Given that SHA1 must have high entropy and hence shouldn't be
		// compressible. Thus, we can't go below 20 bytes (len of SHA1) per ref,
		// hence 20*many16Ki = 320 KiB.
		assert.Loosely(t, len(stored.CompressedState), should.BeGreaterThan(20*many16Ki))
		// But refs themselves should be quite compressible.
		assert.Loosely(t, len(stored.CompressedState), should.BeLessThan(20*many16Ki*5/4))

		assert.Loosely(t, getSentMetric(c, metricTaskGitilesStoredRefs, jobID), should.Equal(many16Ki))
		assert.Loosely(t, getSentMetric(c, metricTaskGitilesStoredSize, jobID), should.Equal(len(stored.CompressedState)))
	})
}

// getSentMetric returns sent value or nil if value wasn't sent.
func getSentMetric(c context.Context, m types.Metric, fieldVals ...any) any {
	return tsmon.GetState(c).Store().Get(c, m, fieldVals)
}

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
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadSave(t *testing.T) {
	t.Parallel()

	Convey("storeState/loadState work", t, func() {
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

		Convey("load first time ever", func() {
			So(loadNoError(repo), ShouldResemble, map[string]string{})
			So(loadNoError(repo), ShouldNotBeNil)
		})

		Convey("save/load/save/load", func() {
			So(saveState(c, jobID, repo, map[string]string{"refs/heads/master": "beefcafe"}), ShouldBeNil)
			So(loadNoError(repo), ShouldResemble, map[string]string{"refs/heads/master": "beefcafe"})
			So(saveState(c, jobID, repo, map[string]string{"refs/tails/master": "efacfeeb"}), ShouldBeNil)
			So(loadNoError(repo), ShouldResemble, map[string]string{"refs/tails/master": "efacfeeb"})
		})

		Convey("save/change repo name/load", func() {
			So(saveState(c, jobID, repo, map[string]string{"refs/heads/master": "beefcafe"}), ShouldBeNil)
			So(loadNoError("https://some-other.googlesource.com/repo"), ShouldResemble, map[string]string{})
		})

		Convey("save/load with deeply nested refs", func() {
			nested := map[string]string{
				"refs/weirdo":                  "00",
				"refs/heads/master":            "11",
				"refs/heads/branch":            "22",
				"refs/heads/infra/config":      "33",
				"refs/heads/infra/deploy":      "44",
				"refs/heads/infra/configs/why": "55",
				"refs/heads/infra/configs/not": "66",
			}
			So(saveState(c, jobID, repo, nested), ShouldBeNil)
			So(loadNoError(repo), ShouldResemble, nested)
		})

		Convey("wipeoutLegacyEntriesCrbug948900", func() {
			So(saveState(c, jobID, repo, map[string]string{"refs/heads/master": "beefcafe"}), ShouldBeNil)
			r := Repository{ID: jobID + ":" + repo, CompressedState: []byte("blah")}
			So(ds.Put(c, &r), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			wipeoutLegacyEntriesCrbug948900(c)

			So(loadNoError(repo), ShouldResemble, map[string]string{"refs/heads/master": "beefcafe"})
			So(ds.Get(c, &r), ShouldEqual, ds.ErrNoSuchEntity)
		})
	})
}

func TestLoadSaveCompression(t *testing.T) {
	t.Parallel()

	Convey("Compress lots of similar refs", t, func() {
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

		So(saveState(c, jobID, repo, tags), ShouldBeNil)
		id, err := repositoryID(jobID, repo)
		So(err, ShouldBeNil)
		stored := Repository{ID: id}
		So(ds.Get(c, &stored), ShouldBeNil)
		// Given that SHA1 must have high entropy and hence shouldn't be
		// compressible. Thus, we can't go below 20 bytes (len of SHA1) per ref,
		// hence 20*many16Ki = 320 KiB.
		So(len(stored.CompressedState), ShouldBeGreaterThan, 20*many16Ki)
		// But refs themselves should be quite compressible.
		So(len(stored.CompressedState), ShouldBeLessThan, 20*many16Ki*5/4)

		So(getSentMetric(c, metricTaskGitilesStoredRefs, jobID), ShouldEqual, many16Ki)
		So(getSentMetric(c, metricTaskGitilesStoredSize, jobID), ShouldEqual, len(stored.CompressedState))
	})
}

// getSentMetric returns sent value or nil if value wasn't sent.
func getSentMetric(c context.Context, m types.Metric, fieldVals ...interface{}) interface{} {
	return tsmon.GetState(c).Store().Get(c, m, time.Time{}, fieldVals)
}

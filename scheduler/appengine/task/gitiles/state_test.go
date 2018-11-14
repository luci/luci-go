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
	"net/url"
	"strconv"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
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
		u, err := url.Parse("https://repo/whatever.git")
		So(err, ShouldBeNil)
		jobID := "job"

		loadNoError := func() map[string]string {
			r, err := loadState(c, jobID, u)
			if err != nil {
				panic(err)
			}
			return r
		}

		Convey("load first time ever", func() {
			So(loadNoError(), ShouldResemble, map[string]string{})
			So(loadNoError(), ShouldNotBeNil)
		})

		Convey("save/load/save/load", func() {
			So(saveState(c, jobID, u, map[string]string{"refs/heads/master": "beefcafe"}), ShouldBeNil)
			So(loadNoError(), ShouldResemble, map[string]string{"refs/heads/master": "beefcafe"})
			So(saveState(c, jobID, u, map[string]string{"refs/tails/master": "efacfeeb"}), ShouldBeNil)
			So(loadNoError(), ShouldResemble, map[string]string{"refs/tails/master": "efacfeeb"})
		})

		Convey("save/change repo name/load", func() {
			So(saveState(c, jobID, u, map[string]string{"refs/heads/master": "beefcafe"}), ShouldBeNil)
			u.Host = "some-other.googlesource.com"
			So(loadNoError(), ShouldResemble, map[string]string{})
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
			So(saveState(c, jobID, u, nested), ShouldBeNil)
			So(loadNoError(), ShouldResemble, nested)
		})

		Convey("loadState old data and update it", func() {
			So(ds.Put(c, &Repository{
				ID:         repositoryID(jobID, u),
				References: []Reference{{Name: "refs/heads/master", Revision: "deadbeef"}},
			}), ShouldBeNil)

			So(loadNoError(), ShouldResemble, map[string]string{"refs/heads/master": "deadbeef"})
			So(saveState(c, jobID, u, map[string]string{"refs/heads/master2": "beefcafe"}), ShouldBeNil)
			So(loadNoError(), ShouldResemble, map[string]string{"refs/heads/master2": "beefcafe"})

			stored := Repository{ID: repositoryID(jobID, u)}
			So(ds.Get(c, &stored), ShouldBeNil)
			So(stored.References, ShouldBeNil)
		})
	})
}

func TestLoadSaveCompression(t *testing.T) {
	t.Parallel()

	Convey("Compress lots of similar refs", t, func() {
		c := memory.Use(context.Background())
		c, _, _ = tsmon.WithFakes(c)
		tsmon.GetState(c).SetStore(store.NewInMemory(&target.Task{}))

		u, err := url.Parse("https://repo/whatever.git")
		So(err, ShouldBeNil)
		jobID := "job"

		many16Ki := 16 * 1024
		tags := make(map[string]string, many16Ki)
		for i := 0; i < many16Ki; i++ {
			ref := "refs/tags/" + strconv.FormatInt(int64(i), 20)
			hsh := fmt.Sprintf("%x", sha256.Sum256([]byte(ref)))[:40]
			tags[ref] = hsh
		}

		So(saveState(c, jobID, u, tags), ShouldBeNil)
		stored := Repository{ID: repositoryID(jobID, u)}
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

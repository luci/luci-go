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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("dumpDatastore", t, func() {
		c := memory.Use(context.Background())
		datastore.GetTestable(c).Consistent(true)
		bq := &fakeBQDataset{result: make(map[string]any)}
		Convey("none", func() {
			So(uploadToBQ(c, bq), ShouldBeNil)
			So(bq.result, ShouldResemble, map[string]any{})
		})
		Convey("one config", func() {
			So(datastore.Put(c, &model.Config{
				ID: "id",
			}), ShouldBeNil)

			So(uploadToBQ(c, bq), ShouldBeNil)
			So(bq.result["config"], ShouldHaveLength, 1)
			So(bq.result["instance_count"], ShouldBeNil)
		})
		Convey("one config one instance_count", func() {
			So(datastore.Put(c, &model.Config{
				ID: "id",
			}), ShouldBeNil)
			So(datastore.Put(c, &metrics.InstanceCount{ID: "id"}), ShouldBeNil)

			So(uploadToBQ(c, bq), ShouldBeNil)
			So(bq.result["config"], ShouldHaveLength, 1)
			So(bq.result["instance_count"], ShouldHaveLength, 1)
		})
	})
}

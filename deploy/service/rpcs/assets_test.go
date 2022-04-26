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

package rpcs

import (
	"context"
	"fmt"
	"testing"
	"time"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
	"go.chromium.org/luci/deploy/service/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAssetsRPC(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		phonyStatus := func(x int) *statuspb.Status {
			return &statuspb.Status{
				Message: fmt.Sprintf("phony %d", x),
			}
		}

		phonyHistory := func(id int) *modelpb.AssetHistory {
			return &modelpb.AssetHistory{
				AssetId:   "apps/test",
				HistoryId: int64(id),
				Actuation: &modelpb.Actuation{
					Status: phonyStatus(id),
				},
			}
		}

		asset := &modelpb.Asset{
			Id:                  "apps/test",
			PostActuationStatus: phonyStatus(1000),
		}

		So(datastore.Put(ctx, &model.Asset{
			ID:            "apps/test",
			Asset:         asset,
			LastHistoryID: 9,
			HistoryEntry:  phonyHistory(10), // being recorded now
		}), ShouldBeNil)

		for i := 1; i < 10; i++ {
			So(datastore.Put(ctx, &model.AssetHistory{
				ID:     int64(i),
				Parent: datastore.NewKey(ctx, "Asset", "apps/test", 0, nil),
				Entry:  phonyHistory(i),
			}), ShouldBeNil)
		}

		srv := Assets{}

		Convey("Works", func() {
			resp, err := srv.ListAssetHistory(ctx, &rpcpb.ListAssetHistoryRequest{
				AssetId: "apps/test",
				Limit:   4,
			})
			So(err, ShouldBeNil)
			So(resp.Asset, ShouldResembleProto, asset)
			So(resp.Current, ShouldResembleProto, phonyHistory(10))
			So(resp.History, ShouldResembleProto, []*modelpb.AssetHistory{
				phonyHistory(9),
				phonyHistory(8),
				phonyHistory(7),
				phonyHistory(6),
			})

			resp, err = srv.ListAssetHistory(ctx, &rpcpb.ListAssetHistoryRequest{
				AssetId:         "apps/test",
				LatestHistoryId: 5,
				Limit:           4,
			})
			So(err, ShouldBeNil)
			So(resp.Asset, ShouldResembleProto, asset)
			So(resp.Current, ShouldResembleProto, phonyHistory(10))
			So(resp.History, ShouldResembleProto, []*modelpb.AssetHistory{
				phonyHistory(5),
				phonyHistory(4),
				phonyHistory(3),
				phonyHistory(2),
			})

			resp, err = srv.ListAssetHistory(ctx, &rpcpb.ListAssetHistoryRequest{
				AssetId:         "apps/test",
				LatestHistoryId: 1,
				Limit:           4,
			})
			So(err, ShouldBeNil)
			So(resp.Asset, ShouldResembleProto, asset)
			So(resp.Current, ShouldResembleProto, phonyHistory(10))
			So(resp.History, ShouldResembleProto, []*modelpb.AssetHistory{
				phonyHistory(1),
			})
		})
	})
}

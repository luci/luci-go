// Copyright 2021 The LUCI Authors.
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

package tree

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	tspb "go.chromium.org/luci/tree_status/proto/v1"
)

func TestTreeStatesClient(t *testing.T) {
	t.Parallel()

	ftt.Run("FetchLatest", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := tspb.NewMockTreeStatusClient(ctl)
		client := &treeStatusClientImpl{
			client: mc,
		}
		t.Run("Works", func(t *ftt.Test) {
			now := time.Date(2000, 01, 02, 03, 04, 05, 678910111, time.UTC)
			req := &tspb.GetStatusRequest{Name: "trees/mock/status/latest"}
			res := &tspb.Status{
				GeneralState: tspb.GeneralState_OPEN,
				Message:      "tree is open",
				CreateUser:   "abc@example.com",
				CreateTime:   timestamppb.New(now),
			}
			mc.EXPECT().GetStatus(gomock.Any(), proto.MatcherEqual(req),
				gomock.Any()).Return(res, nil)

			ts, err := client.FetchLatest(ctx, "mock")
			assert.NoErr(t, err)
			assert.Loosely(t, ts, should.Resemble(Status{
				State: Open,
				Since: now,
			}))
		})
		t.Run("Error if rpc call fails", func(t *ftt.Test) {
			mc.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, errors.New("rpc error"))
			_, err := client.FetchLatest(ctx, "mock")
			assert.Loosely(t, err, should.ErrLike("rpc error"))
		})
	})
}

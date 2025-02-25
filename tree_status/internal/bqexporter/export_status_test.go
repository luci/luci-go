// Copyright 2024 The LUCI Authors.
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

package bqexporter

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/tree_status/internal/status"
	"go.chromium.org/luci/tree_status/internal/testutil"
	bqpb "go.chromium.org/luci/tree_status/proto/bq"
	v1 "go.chromium.org/luci/tree_status/proto/v1"
)

func TestExportStatus(t *testing.T) {
	ftt.Run(`Export status`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		client := NewFakeClient()
		// This should not be exported, because it's earlier than the one in BQ.
		status.NewStatusBuilder().WithCreateTime(time.Unix(90, 0).UTC()).CreateInDB(ctx)
		// This should not be exported, because it's equal to the one in BQ.
		status.NewStatusBuilder().WithCreateTime(time.Unix(100, 0).UTC()).CreateInDB(ctx)
		status.NewStatusBuilder().WithCreateTime(time.Unix(200, 0).UTC()).CreateInDB(ctx)
		status.NewStatusBuilder().WithCreateTime(time.Unix(150, 0).UTC()).WithCreateUser("someone@google.com").CreateInDB(ctx)
		status.NewStatusBuilder().WithCreateTime(time.Unix(300, 0).UTC()).WithCreateUser("luci-notify@appspot.gserviceaccount.com").CreateInDB(ctx)
		status.NewStatusBuilder().WithCreateTime(time.Unix(400, 0).UTC()).WithClosingBuilderName("projects/chromium-m100/buckets/ci/builders/Linux 123").WithGeneralStatus(v1.GeneralState_CLOSED).WithCreateUser("").CreateInDB(ctx)
		err := export(ctx, client)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, client.Insertions, should.Match([]*bqpb.StatusRow{
			{
				TreeName:   "chromium",
				Status:     "open",
				Message:    "Tree is open!",
				CreateUser: "user",
				CreateTime: timestamppb.New(time.Unix(150, 0).UTC()),
			},
			{
				TreeName:   "chromium",
				Status:     "open",
				Message:    "Tree is open!",
				CreateUser: "user",
				CreateTime: timestamppb.New(time.Unix(200, 0).UTC()),
			},
			{
				TreeName:   "chromium",
				Status:     "open",
				Message:    "Tree is open!",
				CreateUser: "luci-notify@appspot.gserviceaccount.com",
				CreateTime: timestamppb.New(time.Unix(300, 0).UTC()),
			},
			{
				TreeName:   "chromium",
				Status:     "closed",
				Message:    "Tree is open!",
				CreateTime: timestamppb.New(time.Unix(400, 0).UTC()),
				ClosingBuilder: &bqpb.Builder{
					Project: "chromium-m100",
					Bucket:  "ci",
					Builder: "Linux 123",
				},
			},
		}))
	})
}

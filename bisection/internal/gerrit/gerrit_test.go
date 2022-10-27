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

package gerrit

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

const testGerritHost = "test-review.googlesource.com"

func TestGetChange(t *testing.T) {
	t.Parallel()

	Convey("GetChange", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		Convey("No change found", func() {
			res := &gerritpb.ListChangesResponse{}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, "abcdefgh")
			So(err, ShouldErrLike, "no change found")
			So(changeInfo, ShouldBeNil)
		})

		Convey("More than 1 change found", func() {
			res := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{
					{
						Number:  123456,
						Project: "chromium/test",
						Status:  gerritpb.ChangeStatus_MERGED,
					},
					{
						Number:  234567,
						Project: "chromium/test",
						Status:  gerritpb.ChangeStatus_MERGED,
					},
				},
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, "abcdefgh")
			So(err, ShouldErrLike, "multiple changes found")
			So(changeInfo, ShouldBeNil)
		})

		Convey("Exactly 1 change found", func() {
			expectedChange := &gerritpb.ChangeInfo{
				Number:  123456,
				Project: "chromium/test",
				Status:  gerritpb.ChangeStatus_MERGED,
			}
			res := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{expectedChange},
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, "abcdefgh")
			So(err, ShouldBeNil)
			So(changeInfo, ShouldResemble, expectedChange)
		})
	})
}

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

package swarmingimpl

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	googleapi "google.golang.org/api/googleapi"

	. "go.chromium.org/luci/common/testing/assertions"
	swarming "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestDeleteBotsParse_NoInput(t *testing.T) {
	t.Parallel()

	Convey(`Make sure that Parse handles when no bot ID is given.`, t, func() {
		b := deletebotsRun{}
		b.Init(&testAuthFlags{})

		err := b.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		So(err, ShouldBeNil)

		err = b.parse([]string{})
		So(err, ShouldErrLike, "must specify at least one")
	})
}

func TestDeleteBots(t *testing.T) {
	t.Parallel()

	Convey(`Test deleteBotsInList`, t, func() {
		ctx := context.Background()
		b := deletebotsRun{force: true}
		failbotID := "failingbotID"
		givenbotID := []string{}

		service := &testService{
			deleteBot: func(ctx context.Context, botID string) (*swarming.DeleteResponse, error) {
				givenbotID = append(givenbotID, botID)
				if botID == failbotID {
					return nil, &googleapi.Error{Code: 404}
				}
				if botID == "cannotdeletebotID" {
					return &swarming.DeleteResponse{
						Deleted: false,
					}, nil
				}
				return &swarming.DeleteResponse{
					Deleted: true,
				}, nil
			},
		}

		Convey(`Test deleting one bot`, func() {
			err := b.deleteBotsInList(ctx, []string{"testbot123"}, service)
			So(err, ShouldBeNil)
			So(givenbotID, ShouldResemble, []string{"testbot123"})
		})

		Convey(`Test deleting few bots`, func() {
			err := b.deleteBotsInList(ctx, []string{"testbot123", "testbot456", "testbot789"}, service)
			So(err, ShouldBeNil)
			So(givenbotID, ShouldResemble, []string{"testbot123", "testbot456", "testbot789"})
		})

		Convey(`Test when a bot can't be deleted`, func() {
			err := b.deleteBotsInList(ctx, []string{failbotID}, service)
			So(err, ShouldErrLike, "404")
			So(givenbotID, ShouldResemble, []string{failbotID})
		})

		Convey(`stop deleting bots immediately when encounter a bot that can't be deleted`, func() {
			err := b.deleteBotsInList(ctx, []string{"testbot123", "failingbotID", "testbot456", "testbot789"}, service)
			So(err, ShouldErrLike, "404")
			So(givenbotID, ShouldResemble, []string{"testbot123", "failingbotID"})
		})

		Convey(`Test when bot wasn't deleted`, func() {
			err := b.deleteBotsInList(ctx, []string{"testbot123", "cannotdeletebotID", "testbot456", "testbot789"}, service)
			So(err, ShouldErrLike, "not deleted")
			So(givenbotID, ShouldResemble, []string{"testbot123", "cannotdeletebotID"})
		})

	})
}

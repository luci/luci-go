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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarming "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDeleteBotsParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdDeleteBots,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Make sure that Parse handles when no bot ID is given.`, t, func() {
		expectErr(nil, "expecting at least 1 argument")
	})
}

func TestDeleteBots(t *testing.T) {
	t.Parallel()

	Convey(`Test delete bots`, t, func() {
		failbotID := "failingbotID"
		givenbotID := []string{}

		service := &swarmingtest.Client{
			DeleteBotMock: func(ctx context.Context, botID string) (*swarming.DeleteResponse, error) {
				givenbotID = append(givenbotID, botID)
				if botID == failbotID {
					return nil, status.Errorf(codes.NotFound, "no such bot")
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
			_, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", "testbot123"},
				nil, service,
			)
			So(code, ShouldEqual, 0)
			So(givenbotID, ShouldResemble, []string{"testbot123"})
		})

		Convey(`Test deleting few bots`, func() {
			_, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", "testbot123", "testbot456"},
				nil, service,
			)
			So(code, ShouldEqual, 0)
			So(givenbotID, ShouldResemble, []string{"testbot123", "testbot456"})
		})

		Convey(`Test when a bot can't be deleted`, func() {
			err, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", failbotID},
				nil, service,
			)
			So(code, ShouldEqual, 1)
			So(err, ShouldErrLike, "no such bot")
			So(givenbotID, ShouldResemble, []string{failbotID})
		})

		Convey(`stop deleting bots immediately when encounter a bot that can't be deleted`, func() {
			err, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", "testbot123", "failingbotID", "testbot456", "testbot789"},
				nil, service,
			)
			So(code, ShouldEqual, 1)
			So(err, ShouldErrLike, "no such bot")
			So(givenbotID, ShouldResemble, []string{"testbot123", "failingbotID"})
		})

		Convey(`Test when bot wasn't deleted`, func() {
			err, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", "testbot123", "cannotdeletebotID", "testbot456", "testbot789"},
				nil, service,
			)
			So(code, ShouldEqual, 1)
			So(err, ShouldErrLike, "not deleted")
			So(givenbotID, ShouldResemble, []string{"testbot123", "cannotdeletebotID"})
		})
	})
}

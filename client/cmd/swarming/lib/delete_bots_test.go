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

package lib

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	googleapi "google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDeleteBotsParse_NoInput(t *testing.T) {
	Convey(`Make sure that Parse handles when no bot ID is given.`, t, func() {
		b := deletebotsRun{}
		b.Init(&testAuthFlags{})
		err := b.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		err = b.Parse([]string{})
		So(err, ShouldErrLike, "must specify at least one")
	})
}

func TestDeleteBots(t *testing.T) {
	t.Parallel()

	c := context.Background()
	b := deletebotsRun{}
	called := 0

	service := &testService{
		deleteBots: func(c context.Context, botID string) (*swarming.SwarmingRpcsDeletedResponse, error) {
			called += 1
			if botID == "failingbotID" {
				return nil, &googleapi.Error{Code: 404}
			}
			return nil, nil
		},
	}

	Convey(`Test when a bot can't be deleted`, t, func() {
		called = 0
		err := b.deleteBotsInList(c, []string{"failingbotID"}, service)
		So(err, ShouldErrLike, "404")
		So(called, ShouldEqual, 1)
	})

	Convey(`Test deleting one bot`, t, func() {
		called = 0
		err := b.deleteBotsInList(c, []string{"testbot123"}, service)
		So(err, ShouldBeNil)
		So(called, ShouldEqual, 1)
	})

	Convey(`Test deleting few bots`, t, func() {
		called = 0
		err := b.deleteBotsInList(c, []string{"testbot123", "testbot456", "testbot789"}, service)
		So(err, ShouldBeNil)
		So(called, ShouldEqual, 3)
	})

	Convey(`stop deleting bots immediately when encounter a bot that can't be deleted`, t, func() {
		called = 0
		err := b.deleteBotsInList(c, []string{"testbot123", "failingbotID", "testbot456", "testbot789"}, service)
		So(err, ShouldErrLike, "404")
		So(called, ShouldEqual, 2)
	})
}

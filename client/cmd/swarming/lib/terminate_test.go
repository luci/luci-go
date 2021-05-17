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

func TestTerminateBotsParse_NoOrManyInputs(t *testing.T) {
	t.Parallel()

	Convey(`Test TerminateBotsParse when there's no input or too many inputs`, t, func() {

		t := terminateRun{}
		t.Init(&testAuthFlags{})

		err := t.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		So(err, ShouldBeNil)

		Convey(`Make sure that Parse handles when no bot ID is given.`, func() {
			err = t.parse([]string{})
			So(err, ShouldErrLike, "must specify a")
		})

		Convey(`Make sure that Parse handles when too many bot ID is given.`, func() {
			err = t.parse([]string{"a", "b"})
			So(err, ShouldErrLike, "specify only one")
		})
	})
}

func TestTerminateBots(t *testing.T) {
	t.Parallel()

	Convey(`Test terminate`, t, func() {
		ctx := context.Background()
		t := terminateRun{wait: false}
		failbotID := "failingbot123"
		//taskfailID := "taskfailed123"
		terminateTaskID := "terminatetask123"
		givenbotID := ""

		service := &testService{
			terminateBot: func(ctx context.Context, botID string) (*swarming.SwarmingRpcsTerminateResponse, error) {
				givenbotID = botID
				if botID == failbotID {
					return nil, &googleapi.Error{Code: 404}
				}
				return &swarming.SwarmingRpcsTerminateResponse{
					TaskId: terminateTaskID,
				}, nil
			},
		}

		Convey(`Test terminating bot`, func() {
			err := t.terminateBot(ctx, "testbot123", service)
			So(err, ShouldBeNil)
			So(givenbotID, ShouldResemble, "testbot123")
		})

		Convey(`Test when terminating bot fails`, func() {
			err := t.terminateBot(ctx, failbotID, service)
			So(err, ShouldErrLike, "404")
			So(givenbotID, ShouldResemble, failbotID)
		})

	})
}

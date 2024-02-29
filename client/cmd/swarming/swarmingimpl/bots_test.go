// Copyright 2018 The LUCI Authors.
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

	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBotsParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdBots,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Make sure that Parse fails with -count and -field.`, t, func() {
		expectErr([]string{"-count", "-field", "myField"}, "-field cannot")
	})
}

func TestFileOutput(t *testing.T) {
	t.Parallel()

	expectedDims := []*swarmingv2.StringPair{
		{
			Key:   "a",
			Value: "b",
		},
		{
			Key:   "c",
			Value: "d",
		},
	}
	expectedCount := &swarmingv2.BotsCount{
		Count: 10,
		Busy:  6,
		Dead:  4,
	}
	expectedBots := []*swarmingv2.BotInfo{
		{
			BotId: "bot1",
		},
		{
			BotId: "bot2",
		},
	}

	service := &swarmingtest.Client{
		CountBotsMock: func(ctx context.Context, dims []*swarmingv2.StringPair) (*swarmingv2.BotsCount, error) {
			for _, dim := range dims {
				So(dim, ShouldBeIn, expectedDims)
			}
			So(dims, ShouldHaveLength, len(expectedDims))
			return expectedCount, nil
		},
		ListBotsMock: func(ctx context.Context, dims []*swarmingv2.StringPair) ([]*swarmingv2.BotInfo, error) {
			// Sometimes the argument parser ends up creating these arguments in
			// different order. These two checks ensure that dims is equal to expected dims.
			for _, dim := range dims {
				So(dim, ShouldBeIn, expectedDims)
			}
			So(dims, ShouldHaveLength, len(expectedDims))
			return expectedBots, nil
		},
	}

	Convey(`Count is correctly written out when -count is specified`, t, func() {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdBots,
			[]string{"-server", "example.com", "-count", "-dimension", "a=b", "-dimension", "c=d"},
			nil, service,
		)
		So(code, ShouldEqual, 0)
		So(stdout, ShouldEqual, `{
 "count": 10,
 "dead": 4,
 "busy": 6
}
`)
	})

	Convey(`List bots is correctly outputted`, t, func() {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdBots,
			[]string{"-server", "example.com", "-dimension", "a=b", "-dimension", "c=d"},
			nil, service,
		)
		So(code, ShouldEqual, 0)
		So(stdout, ShouldEqual, `[
 {
  "bot_id": "bot1"
 },
 {
  "bot_id": "bot2"
 }
]
`)
	})

	Convey(`List bots is correctly outputted when -bare`, t, func() {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdBots,
			[]string{"-server", "example.com", "-dimension", "a=b", "-dimension", "c=d", "-bare"},
			nil, service,
		)
		So(code, ShouldEqual, 0)
		So(stdout, ShouldEqual, "bot1\nbot2\n")
	})
}

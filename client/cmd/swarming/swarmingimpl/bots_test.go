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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
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
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Make sure that Parse fails with -count and -field.`, t, func(t *ftt.Test) {
		expectErr([]string{"-count", "-field", "myField"}, "-field cannot")
	})
}

func TestFileOutput(t *testing.T) {
	t.Parallel()

	expectedDims := []*swarmingpb.StringPair{
		{
			Key:   "a",
			Value: "b",
		},
		{
			Key:   "c",
			Value: "d",
		},
	}
	expectedCount := &swarmingpb.BotsCount{
		Count: 10,
		Busy:  6,
		Dead:  4,
	}
	expectedBots := []*swarmingpb.BotInfo{
		{
			BotId: "bot1",
		},
		{
			BotId: "bot2",
		},
	}

	service := &swarmingtest.Client{
		CountBotsMock: func(ctx context.Context, dims []*swarmingpb.StringPair) (*swarmingpb.BotsCount, error) {
			for _, dim := range dims {
				assert.Loosely(t, expectedDims, should.ContainMatch(dim))
			}
			assert.Loosely(t, dims, should.HaveLength(len(expectedDims)))
			return expectedCount, nil
		},
		ListBotsMock: func(ctx context.Context, dims []*swarmingpb.StringPair) ([]*swarmingpb.BotInfo, error) {
			// Sometimes the argument parser ends up creating these arguments in
			// different order. These two checks ensure that dims is equal to expected dims.
			for _, dim := range dims {
				assert.Loosely(t, expectedDims, should.ContainMatch(dim))
			}
			assert.Loosely(t, dims, should.HaveLength(len(expectedDims)))
			return expectedBots, nil
		},
	}

	ftt.Run(`Count is correctly written out when -count is specified`, t, func(t *ftt.Test) {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdBots,
			[]string{"-server", "example.com", "-count", "-dimension", "a=b", "-dimension", "c=d"},
			nil, service,
		)
		assert.Loosely(t, code, should.BeZero)
		assert.Loosely(t, stdout, should.Equal(`{
 "count": 10,
 "dead": 4,
 "busy": 6
}
`))
	})

	ftt.Run(`List bots is correctly outputted`, t, func(t *ftt.Test) {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdBots,
			[]string{"-server", "example.com", "-dimension", "a=b", "-dimension", "c=d"},
			nil, service,
		)
		assert.Loosely(t, code, should.BeZero)
		assert.Loosely(t, stdout, should.Equal(`[
 {
  "bot_id": "bot1"
 },
 {
  "bot_id": "bot2"
 }
]
`))
	})

	ftt.Run(`List bots is correctly outputted when -bare`, t, func(t *ftt.Test) {
		_, code, stdout, _ := SubcommandTest(
			context.Background(),
			CmdBots,
			[]string{"-server", "example.com", "-dimension", "a=b", "-dimension", "c=d", "-bare"},
			nil, service,
		)
		assert.Loosely(t, code, should.BeZero)
		assert.Loosely(t, stdout, should.Equal("bot1\nbot2\n"))
	})
}

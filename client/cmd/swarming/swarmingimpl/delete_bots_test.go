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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarming "go.chromium.org/luci/swarming/proto/api_v2"
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
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Make sure that Parse handles when no bot ID is given.`, t, func(t *ftt.Test) {
		expectErr(nil, "expecting at least 1 argument")
	})
}

func TestDeleteBots(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test delete bots`, t, func(t *ftt.Test) {
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

		t.Run(`Test deleting one bot`, func(t *ftt.Test) {
			_, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", "testbot123"},
				nil, service,
			)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, givenbotID, should.Resemble([]string{"testbot123"}))
		})

		t.Run(`Test deleting few bots`, func(t *ftt.Test) {
			_, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", "testbot123", "testbot456"},
				nil, service,
			)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, givenbotID, should.Resemble([]string{"testbot123", "testbot456"}))
		})

		t.Run(`Test when a bot can't be deleted`, func(t *ftt.Test) {
			err, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", failbotID},
				nil, service,
			)
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, err, should.ErrLike("no such bot"))
			assert.Loosely(t, givenbotID, should.Resemble([]string{failbotID}))
		})

		t.Run(`stop deleting bots immediately when encounter a bot that can't be deleted`, func(t *ftt.Test) {
			err, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", "testbot123", "failingbotID", "testbot456", "testbot789"},
				nil, service,
			)
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, err, should.ErrLike("no such bot"))
			assert.Loosely(t, givenbotID, should.Resemble([]string{"testbot123", "failingbotID"}))
		})

		t.Run(`Test when bot wasn't deleted`, func(t *ftt.Test) {
			err, code, _, _ := SubcommandTest(
				context.Background(),
				CmdDeleteBots,
				[]string{"-server", "example.com", "-f", "testbot123", "cannotdeletebotID", "testbot456", "testbot789"},
				nil, service,
			)
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, err, should.ErrLike("not deleted"))
			assert.Loosely(t, givenbotID, should.Resemble([]string{"testbot123", "cannotdeletebotID"}))
		})
	})
}

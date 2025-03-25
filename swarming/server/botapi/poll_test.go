// Copyright 2025 The LUCI Authors.
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

package botapi

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"

	configpb "go.chromium.org/luci/swarming/proto/config"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botsession"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/model"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(pollRequest{}))
}

func TestProcessPoll(t *testing.T) {
	t.Parallel()

	const (
		testBotID       = "bot-id"
		testBotPool     = "test-pool"
		testSessionID   = "session-id"
		testBotVersion  = "test-version"
		testRequestUUID = "test-uuid"
	)

	var testTime = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)

	goodSession := &internalspb.Session{
		BotId:          testBotID,
		SessionId:      testSessionID,
		Expiry:         timestamppb.New(testTime.Add(time.Minute)),
		BotConfig:      &internalspb.BotConfig{},
		LastSeenConfig: timestamppb.New(testTime),
	}

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testTime)

		secret := hmactoken.NewStaticSecret(secrets.Secret{
			Active: []byte("secret"),
		})

		fakeSession := func(s *internalspb.Session) []byte {
			blob, err := botsession.Marshal(s, secret)
			if err != nil {
				panic(err)
			}
			return blob
		}

		conf := cfgtest.NewMockedConfigs()
		group := conf.MockBot(testBotID, testBotPool)
		group.Dimensions = append(group.Dimensions, "extra:2", "extra:1")

		srv := BotAPIServer{
			cfg:        cfgtest.MockConfigs(ctx, conf),
			hmacSecret: secret,
			authorizeBot: func(ctx context.Context, botID string, methods []*configpb.BotAuth) error {
				if botID != testBotID {
					return errors.New("boo")
				}
				return nil
			},
		}

		t.Run("OK", func(t *ftt.Test) {
			req, err := srv.processPoll(ctx, &PollRequest{
				Session: fakeSession(goodSession),
				Dimensions: map[string][]string{
					"id":          {testBotID},
					"pool":        {testBotPool, "will-be-ignored"},
					"more":        {"b", "a"},
					"quarantined": {"boom"},
				},
				State:       botstate.Dict{JSON: []byte(`{"key": "val"}`)},
				Version:     testBotVersion,
				RequestUUID: testRequestUUID,
			})
			assert.NoErr(t, err)

			assert.Loosely(t, req.conf, should.NotBeNil)
			req.conf = nil
			assert.Loosely(t, req.group.Pools(), should.Match([]string{testBotPool}))
			req.group = nil

			assert.That(t, req, should.Match(&pollRequest{
				botID:       testBotID,
				session:     goodSession,
				version:     testBotVersion,
				requestUUID: testRequestUUID,
				state:       botstate.Dict{JSON: []byte(`{"key": "val"}`)},
				dims: []string{
					"extra:1",
					"extra:2",
					"id:bot-id",
					"more:a",
					"more:b",
					"pool:test-pool",
					"quarantined:boom",
				},
				conf:        req.conf,
				group:       req.group,
				quarantined: []string{"boom"},
			}))
		})

		t.Run("Bad dimensions", func(t *ftt.Test) {
			req, err := srv.processPoll(ctx, &PollRequest{
				Session: fakeSession(goodSession),
				Dimensions: map[string][]string{
					"id":          {testBotID},
					"pool":        {testBotPool},
					"more":        {"a", "a"}, // dup
					"  bad key":   {"a"},
					"quarantined": {"boom"},
				},
				Version: testBotVersion,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, req.errs, should.HaveLength(2))
			assert.That(t, req.errs[0], should.ErrLike(`bad dimensions: key "  bad key"`))
			assert.That(t, req.errs[1], should.ErrLike(`bad dimensions: key "more"`))
			assert.Loosely(t, req.dims, should.BeNil)
			assert.That(t, req.quarantined, should.Match([]string{"boom"}))
		})

		t.Run("Missing ID dim", func(t *ftt.Test) {
			_, err := srv.processPoll(ctx, &PollRequest{
				Session: fakeSession(goodSession),
				Dimensions: map[string][]string{
					"pool": {testBotPool},
				},
				Version: testBotVersion,
			})
			assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("no `id` dimension reported"))
		})

		t.Run("Session bot ID != dimensions bot ID", func(t *ftt.Test) {
			req, err := srv.processPoll(ctx, &PollRequest{
				Session: fakeSession(goodSession),
				Dimensions: map[string][]string{
					"id":   {"wrong-bot-id"},
					"pool": {testBotPool},
				},
				Version: testBotVersion,
			})
			assert.NoErr(t, err)
			assert.That(t, req.botID, should.Equal(testBotID))
			assert.Loosely(t, req.errs, should.HaveLength(1))
			assert.That(t, req.errs[0], should.ErrLike(`"id" dimension "wrong-bot-id" doesn't match bot ID in the session "bot-id"`))
			assert.Loosely(t, req.dims, should.BeNil)
		})

		t.Run("Missing session", func(t *ftt.Test) {
			req, err := srv.processPoll(ctx, &PollRequest{
				Dimensions: map[string][]string{
					"id":   {testBotID},
					"pool": {testBotPool},
				},
				Version: testBotVersion,
			})
			assert.NoErr(t, err)
			assert.That(t, req.botID, should.Equal(testBotID))
			assert.Loosely(t, req.session, should.BeNil)
			assert.That(t, req.sessionBroken, should.BeFalse)
			assert.Loosely(t, req.errs, should.HaveLength(0))
		})

		t.Run("Expired session", func(t *ftt.Test) {
			req, err := srv.processPoll(ctx, &PollRequest{
				Session: fakeSession(&internalspb.Session{
					BotId:          testBotID,
					SessionId:      testSessionID,
					Expiry:         timestamppb.New(testTime.Add(-time.Minute)),
					BotConfig:      &internalspb.BotConfig{},
					LastSeenConfig: timestamppb.New(testTime.Add(-time.Minute)),
				}),
				Dimensions: map[string][]string{
					"id":   {testBotID},
					"pool": {testBotPool},
				},
				Version: testBotVersion,
			})
			assert.NoErr(t, err)
			assert.That(t, req.botID, should.Equal(testBotID))
			assert.Loosely(t, req.session, should.BeNil)
			assert.That(t, req.sessionBroken, should.BeTrue)
			assert.Loosely(t, req.errs, should.HaveLength(0))
		})

		t.Run("Broken state", func(t *ftt.Test) {
			req, err := srv.processPoll(ctx, &PollRequest{
				Session: fakeSession(goodSession),
				Dimensions: map[string][]string{
					"id":   {testBotID},
					"pool": {testBotPool},
				},
				State:   botstate.Dict{JSON: []byte(`not JSON`)},
				Version: testBotVersion,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, req.errs, should.HaveLength(1))
			assert.That(t, req.errs[0], should.ErrLike("bad state dict"))
			assert.That(t, req.state.IsEmpty(), should.BeTrue)
		})

		t.Run("Missing version", func(t *ftt.Test) {
			req, err := srv.processPoll(ctx, &PollRequest{
				Session: fakeSession(goodSession),
				Dimensions: map[string][]string{
					"id":   {testBotID},
					"pool": {testBotPool},
				},
			})
			assert.NoErr(t, err)
			assert.Loosely(t, req.errs, should.HaveLength(1))
			assert.That(t, req.errs[0], should.ErrLike("no `version` in the request"))
		})

		t.Run("Unauthorized", func(t *ftt.Test) {
			_, err := srv.processPoll(ctx, &PollRequest{
				Dimensions: map[string][]string{
					"id":   {"unknown-bot-id"},
					"pool": {testBotPool},
				},
				Version: testBotVersion,
			})
			assert.That(t, status.Code(err), should.Equal(codes.Unauthenticated))
			assert.That(t, err, should.ErrLike("the bot is not in bots.cfg or wrong credentials passed: boo"))
		})
	})
}

func TestPollResponse(t *testing.T) {
	t.Parallel()

	const (
		testBotID       = "bot-id"
		testBotPool     = "test-pool"
		testSessionID   = "session-id"
		testBotVersion  = "test-version"
		testRequestUUID = "test-uuid"
		testBotIdent    = "user:bot@example.com"
		testBotIP       = "192.0.2.1"
	)

	var testTime = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testTime)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       testBotIdent,
			PeerIPOverride: net.ParseIP(testBotIP),
		})

		var lastUpdate *model.BotInfoUpdate
		var updateErr error
		srv := BotAPIServer{
			submitUpdate: func(ctx context.Context, u *model.BotInfoUpdate) error {
				lastUpdate = u
				return updateErr
			},
		}

		fakePollRequest := func() *pollRequest {
			return &pollRequest{
				botID: testBotID,
				session: &internalspb.Session{
					BotId:          testBotID,
					SessionId:      testSessionID,
					Expiry:         timestamppb.New(testTime.Add(time.Minute)),
					BotConfig:      &internalspb.BotConfig{},
					LastSeenConfig: timestamppb.New(testTime),
				},
				version:     testBotVersion,
				requestUUID: testRequestUUID,
				state:       botstate.Dict{JSON: []byte(`{"rbe_idle": false}`)},
				dims: []string{
					"id:" + testBotID,
					"pool:" + testBotPool,
				},
				group: &cfg.BotGroup{
					Dimensions: map[string][]string{
						"pool": {testBotPool},
					},
				},
			}
		}

		t.Run("RBE idle", func(t *ftt.Test) {
			req := fakePollRequest()
			req.state = botstate.Dict{JSON: []byte(`{"rbe_idle": true}`)}

			resp, err := srv.pollResponse(ctx, req, &PollResponse{
				Cmd: PollRBE,
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(botsrv.Response(&PollResponse{Cmd: PollRBE})))

			assert.That(t, lastUpdate, should.Match(&model.BotInfoUpdate{
				BotID:         testBotID,
				EventType:     model.BotEventIdle,
				EventDedupKey: testRequestUUID,
				Dimensions:    req.dims,
				BotGroupDimensions: map[string][]string{
					"pool": {testBotPool},
				},
				State: &botstate.Dict{JSON: []byte(`{"rbe_idle": true}`)},
				CallInfo: &model.BotEventCallInfo{
					SessionID:       testSessionID,
					Version:         testBotVersion,
					ExternalIP:      testBotIP,
					AuthenticatedAs: testBotIdent,
				},
				HealthInfo: &model.BotHealthInfo{},
				TaskInfo:   &model.BotEventTaskInfo{},
			}))
		})

		t.Run("RBE polling", func(t *ftt.Test) {
			req := fakePollRequest()
			req.state = botstate.Dict{JSON: []byte(`{"rbe_idle": false}`)}

			_, err := srv.pollResponse(ctx, req, &PollResponse{Cmd: PollRBE})
			assert.NoErr(t, err)
			assert.That(t, lastUpdate.EventType, should.Equal(model.BotEventPolling))
		})

		t.Run("Quarantined via dims", func(t *ftt.Test) {
			req := fakePollRequest()
			req.state = botstate.Dict{}
			req.dims = append(req.dims, "quarantined:boom")
			req.quarantined = []string{"boom"}

			_, err := srv.pollResponse(ctx, req, &PollResponse{Cmd: PollSleep})
			assert.NoErr(t, err)

			assert.That(t, lastUpdate, should.Match(&model.BotInfoUpdate{
				BotID:         testBotID,
				EventType:     model.BotEventSleep,
				EventDedupKey: testRequestUUID,
				Dimensions:    req.dims,
				BotGroupDimensions: map[string][]string{
					"pool": {testBotPool},
				},
				State: &botstate.Dict{JSON: []byte(`{"quarantined": "boom"}`)},
				CallInfo: &model.BotEventCallInfo{
					SessionID:       testSessionID,
					Version:         testBotVersion,
					ExternalIP:      testBotIP,
					AuthenticatedAs: testBotIdent,
				},
				HealthInfo: &model.BotHealthInfo{
					Quarantined: "boom",
				},
				TaskInfo: &model.BotEventTaskInfo{},
			}))
		})

		t.Run("Quarantined via validation errors", func(t *ftt.Test) {
			req := fakePollRequest()
			req.state = botstate.Dict{}
			req.validationErr(ctx, errors.New("boom"))

			_, err := srv.pollResponse(ctx, req, &PollResponse{Cmd: PollSleep})
			assert.NoErr(t, err)

			assert.That(t, lastUpdate, should.Match(&model.BotInfoUpdate{
				BotID:         testBotID,
				EventType:     model.BotEventSleep,
				EventDedupKey: testRequestUUID,
				Dimensions:    req.dims,
				BotGroupDimensions: map[string][]string{
					"pool": {testBotPool},
				},
				State: &botstate.Dict{JSON: []byte(`{"quarantined": "boom"}`)},
				CallInfo: &model.BotEventCallInfo{
					SessionID:       testSessionID,
					Version:         testBotVersion,
					ExternalIP:      testBotIP,
					AuthenticatedAs: testBotIdent,
				},
				HealthInfo: &model.BotHealthInfo{
					Quarantined: "boom",
				},
				TaskInfo: &model.BotEventTaskInfo{},
			}))
		})

		t.Run("SubmitUpdate error", func(t *ftt.Test) {
			updateErr = errors.New("boom")

			_, err := srv.pollResponse(ctx, fakePollRequest(), &PollResponse{Cmd: PollRBE})
			assert.That(t, status.Code(err), should.Equal(codes.Internal))
			assert.That(t, err, should.ErrLike("failed to update bot info: boom"))
		})
	})
}

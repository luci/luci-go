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
	"math/rand"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"

	configpb "go.chromium.org/luci/swarming/proto/config"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/botsession"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/model"
)

func TestHandshake(t *testing.T) {
	t.Parallel()

	const (
		botID            = "good-bot"
		botIdent         = "user:bot@example.com"
		botIP            = "192.0.2.1"
		botPool          = "bot-pool"
		botHooksPy       = "custom_hooks.py"
		botHooksPyBody   = "custom hooks body"
		rbeInstance      = "some-instance"
		serverVer        = "server-version"
		conflictingBotID = "conflicting-bot"
	)

	var testTime = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))
		ctx, _ = testclock.UseTime(ctx, testTime)

		mockedCfg := cfgtest.NewMockedConfigs()
		mockedCfg.MockBotPackage("stable", map[string]string{"version": "stable"})
		mockedCfg.Scripts[botHooksPy] = botHooksPyBody

		poolCfg := mockedCfg.MockPool(botPool, "some:realm")
		poolCfg.RbeMigration = &configpb.Pool_RBEMigration{
			RbeInstance:    rbeInstance,
			RbeModePercent: 100,
			BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
				{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 100},
			},
		}

		botGroupCfg := mockedCfg.MockBot(botID, botPool, "extra:1", "extra:2")
		botGroupCfg.BotConfigScript = botHooksPy
		botGroupCfg.Owners = []string{"owner-1@example.com", "owner-2@example.com"}

		mockedCfg.MockBot(conflictingBotID, botPool, "pool:extra")
		mockedCfg.MockPool("extra", "some:realm").RbeMigration = &configpb.Pool_RBEMigration{
			RbeInstance:    "another-instance",
			RbeModePercent: 100,
			BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
				{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 100},
			},
		}

		conf := cfgtest.MockConfigs(ctx, mockedCfg)
		configRev := conf.Cached(ctx).VersionInfo.Revision
		botDigest := conf.Cached(ctx).VersionInfo.StableBot.Digest

		secret := hmactoken.NewStaticSecret(secrets.Secret{
			Active: []byte("secret"),
		})

		srv := NewBotAPIServer(conf, nil, secret, "test-project", serverVer)
		srv.authorizeBot = func(ctx context.Context, bot string, methods []*configpb.BotAuth) error {
			if bot == botID || bot == conflictingBotID {
				return nil
			}
			return errors.Reason("denied").Err()
		}
		var latestUpdate *botinfo.Update
		srv.submitUpdate = func(ctx context.Context, u *botinfo.Update) error {
			u.PanicIfInvalid(u.EventType, false)
			latestUpdate = u
			return nil
		}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       botIdent,
			PeerIPOverride: net.ParseIP(botIP),
		})

		call := func(req *HandshakeRequest) (*HandshakeResponse, error) {
			resp, err := srv.Handshake(ctx, req, &botsrv.Request{})
			if err != nil {
				return nil, err
			}
			return resp.(*HandshakeResponse), nil
		}

		t.Run("OK", func(t *ftt.Test) {
			resp, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{
					"id":      {botID},
					"ignored": {"stuff"},
				},
				State:     botstate.Dict{JSON: []byte(`{"state_key": "state_val"}`)},
				Version:   "reported-bot-version",
				SessionID: "reported-session-id",
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&HandshakeResponse{
				Session:       resp.Session,
				BotVersion:    botDigest,
				ServerVersion: serverVer,
				BotConfig:     botHooksPyBody,
				BotConfigName: botHooksPy,
				BotConfigRev:  configRev,
				BotGroupCfg: BotGroupCfg{
					Dimensions: map[string][]string{
						"pool":  {botPool},
						"extra": {"1", "2"},
					},
				},
				RBE: &BotRBEParams{
					Instance:  rbeInstance,
					PollToken: "legacy-unused-field",
				},
			}))

			session, err := botsession.Unmarshal(resp.Session, secret)
			assert.NoErr(t, err)
			assert.That(t, session, should.Match(&internalspb.Session{
				BotId:               botID,
				SessionId:           "reported-session-id",
				Expiry:              session.Expiry,
				DebugInfo:           session.DebugInfo,
				BotConfig:           session.BotConfig,
				HandshakeConfigHash: session.HandshakeConfigHash,
				LastSeenConfig:      timestamppb.New(testTime),
			}))

			assert.That(t, latestUpdate, should.Match(&botinfo.Update{
				BotID: botID,
				State: &botstate.Dict{
					JSON: []byte(`{
							"handshaking": true,
							"initial_dimensions": {
								"id": ["good-bot"],
								"ignored": ["stuff"]
							},
							"state_key": "state_val"
						}`),
				},
				EventType:     model.BotEventConnected,
				EventDedupKey: "reported-session-id",
				CallInfo: &botinfo.CallInfo{
					SessionID:       "reported-session-id",
					Version:         "reported-bot-version",
					ExternalIP:      botIP,
					AuthenticatedAs: botIdent,
				},
				BotGroupInfo: &botinfo.BotGroupInfo{
					Dimensions: map[string][]string{
						"pool":  {botPool},
						"extra": {"1", "2"},
					},
					Owners: []string{"owner-1@example.com", "owner-2@example.com"},
				},
				HealthInfo: &botinfo.HealthInfo{},
			}))
		})

		t.Run("OK: minimal + auto gen session ID", func(t *ftt.Test) {
			resp, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{
					"id": {botID},
				},
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&HandshakeResponse{
				Session:       resp.Session,
				BotVersion:    botDigest,
				ServerVersion: serverVer,
				BotConfig:     botHooksPyBody,
				BotConfigName: botHooksPy,
				BotConfigRev:  configRev,
				BotGroupCfg: BotGroupCfg{
					Dimensions: map[string][]string{
						"pool":  {botPool},
						"extra": {"1", "2"},
					},
				},
				RBE: &BotRBEParams{
					Instance:  rbeInstance,
					PollToken: "legacy-unused-field",
				},
			}))

			assert.That(t, latestUpdate, should.Match(&botinfo.Update{
				BotID: botID,
				State: &botstate.Dict{
					JSON: []byte(`{
							"handshaking": true,
							"initial_dimensions": {
								"id": ["good-bot"]
							}
						}`),
				},
				EventType:     model.BotEventConnected,
				EventDedupKey: "autogen-2338085100000-7828158075477027098",
				CallInfo: &botinfo.CallInfo{
					SessionID:       "autogen-2338085100000-7828158075477027098",
					ExternalIP:      botIP,
					AuthenticatedAs: botIdent,
				},
				BotGroupInfo: &botinfo.BotGroupInfo{
					Dimensions: map[string][]string{
						"pool":  {botPool},
						"extra": {"1", "2"},
					},
					Owners: []string{"owner-1@example.com", "owner-2@example.com"},
				},
				HealthInfo: &botinfo.HealthInfo{},
			}))
		})

		t.Run("OK: quarantine", func(t *ftt.Test) {
			_, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{
					"id":          {botID},
					"quarantined": {"Boom"},
				},
				State: botstate.Dict{
					JSON: []byte(`{
						"state_key": "state_val",
						"quarantined": true
					}`),
				},
				SessionID: "reported-session-id",
			})
			assert.NoErr(t, err)

			assert.That(t, latestUpdate, should.Match(&botinfo.Update{
				BotID: botID,
				State: &botstate.Dict{
					JSON: []byte(`{
							"handshaking": true,
							"initial_dimensions": {
								"id": ["good-bot"],
								"quarantined": ["Boom"]
							},
							"quarantined": "Boom",
							"state_key": "state_val"
						}`),
				},
				EventType:     model.BotEventConnected,
				EventDedupKey: "reported-session-id",
				CallInfo: &botinfo.CallInfo{
					SessionID:       "reported-session-id",
					ExternalIP:      botIP,
					AuthenticatedAs: botIdent,
				},
				BotGroupInfo: &botinfo.BotGroupInfo{
					Dimensions: map[string][]string{
						"pool":  {botPool},
						"extra": {"1", "2"},
					},
					Owners: []string{"owner-1@example.com", "owner-2@example.com"},
				},
				HealthInfo: &botinfo.HealthInfo{
					Quarantined: "Boom",
				},
			}))
		})

		t.Run("No ID", func(t *ftt.Test) {
			_, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{},
			})
			assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("no `id` dimension reported"))
		})

		t.Run("Bad ID", func(t *ftt.Test) {
			_, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{"id": {"  bad id  "}},
			})
			assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("bad bot `id`"))
		})

		t.Run("Unauthorized", func(t *ftt.Test) {
			_, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{"id": {"unknown-id"}},
			})
			assert.That(t, status.Code(err), should.Equal(codes.Unauthenticated))
			assert.That(t, err, should.ErrLike("the bot is not in bots.cfg"))
		})

		t.Run("Bad dims and state", func(t *ftt.Test) {
			_, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{
					"id":    {botID, "huh"},
					"bad_a": {" bad "},
					"bad_b": {"dup", "dup"},
				},
				State: botstate.Dict{JSON: []byte("not JSON")},
			})
			assert.NoErr(t, err)

			assert.That(t, latestUpdate.HealthInfo.Quarantined, should.Equal(
				`bad dimensions: key "bad_a": bad value " bad ": the value should have no leading or trailing spaces
bad dimensions: key "bad_b": duplicate value "dup"
bad dimensions: key "id": must have only one value
bad state dict: invalid character 'o' in literal null (expecting 'u')`))
		})

		t.Run("Bad dims + self-quarantine", func(t *ftt.Test) {
			_, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{
					"id": {botID, "huh"},
				},
				State: botstate.Dict{JSON: []byte(`{"quarantined": true}`)},
			})
			assert.NoErr(t, err)

			assert.That(t, latestUpdate.HealthInfo.Quarantined, should.Equal(
				"Bot self-quarantined.\nbad dimensions: key \"id\": must have only one value"))
		})

		t.Run("Conflicting RBE config", func(t *ftt.Test) {
			_, err := call(&HandshakeRequest{
				Dimensions: map[string][]string{
					"id": {conflictingBotID},
				},
				State: botstate.Dict{
					JSON: []byte(`{"state_key": "state_val"}`),
				},
				SessionID: "reported-session-id",
			})
			assert.NoErr(t, err)
			assert.That(t, latestUpdate.HealthInfo, should.Match(&botinfo.HealthInfo{
				Quarantined: `conflicting RBE config: bot pools are configured with conflicting RBE instances: ["another-instance" "some-instance"]`,
			}))
		})
	})
}

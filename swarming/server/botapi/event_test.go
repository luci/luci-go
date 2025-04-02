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
	"net"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
)

func TestEvent(t *testing.T) {
	t.Parallel()

	const (
		botID     = "bot-id"
		botIdent  = "user:bot@example.com"
		botIP     = "192.0.2.1"
		sessionID = "session-id"
	)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := context.Background()

		var lastUpdate *model.BotInfoUpdate
		srv := BotAPIServer{
			submitUpdate: func(ctx context.Context, u *model.BotInfoUpdate) error {
				u.PanicIfInvalid()
				lastUpdate = u
				return nil
			},
		}

		srvReq := &botsrv.Request{
			Session: &internalspb.Session{
				BotId:     botID,
				SessionId: sessionID,
			},
			Dimensions: []string{"id:" + botID},
		}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       botIdent,
			PeerIPOverride: net.ParseIP(botIP),
		})

		t.Run("OK minimal", func(t *ftt.Test) {
			_, err := srv.Event(ctx, &EventRequest{Event: model.BotEventRebooting}, srvReq)
			assert.NoErr(t, err)
			assert.That(t, lastUpdate, should.Match(&model.BotInfoUpdate{
				BotID:     botID,
				EventType: model.BotEventRebooting,
				CallInfo: &model.BotEventCallInfo{
					SessionID:       sessionID,
					ExternalIP:      botIP,
					AuthenticatedAs: botIdent,
				},
			}))
		})

		t.Run("OK full, no quarantine", func(t *ftt.Test) {
			_, err := srv.Event(ctx, &EventRequest{
				RequestUUID: "request-uuid",
				Event:       model.BotEventError,
				Message:     "Boom",
				State: botstate.Dict{JSON: []byte(`{
					"state": "abc"
				}`)},
				Version: "bot-version",
			}, srvReq)
			assert.NoErr(t, err)
			assert.That(t, lastUpdate, should.Match(&model.BotInfoUpdate{
				BotID: botID,
				State: &botstate.Dict{JSON: []byte(`{
					"state": "abc"
				}`)},
				EventType:     model.BotEventError,
				EventDedupKey: "request-uuid",
				EventMessage:  "Boom",
				CallInfo: &model.BotEventCallInfo{
					SessionID:       sessionID,
					ExternalIP:      botIP,
					AuthenticatedAs: botIdent,
					Version:         "bot-version",
				},
				HealthInfo: &model.BotHealthInfo{},
			}))
		})

		t.Run("OK, quarantine", func(t *ftt.Test) {
			_, err := srv.Event(ctx, &EventRequest{
				Event:   model.BotEventError,
				Message: "Boom",
				State: botstate.Dict{JSON: []byte(`{
					"quarantined": "quarantine msg",
					"maintenance": "maintenance msg"
				}`)},
			}, srvReq)
			assert.NoErr(t, err)
			assert.That(t, lastUpdate, should.Match(&model.BotInfoUpdate{
				BotID: botID,
				State: &botstate.Dict{JSON: []byte(`{
					"quarantined": "quarantine msg",
					"maintenance": "maintenance msg"
				}`)},
				EventType:    model.BotEventError,
				EventMessage: "Boom",
				CallInfo: &model.BotEventCallInfo{
					SessionID:       sessionID,
					ExternalIP:      botIP,
					AuthenticatedAs: botIdent,
				},
				HealthInfo: &model.BotHealthInfo{
					Quarantined: "quarantine msg",
					Maintenance: "maintenance msg",
				},
			}))
		})

		t.Run("Invalid: no event", func(t *ftt.Test) {
			_, err := srv.Event(ctx, &EventRequest{}, srvReq)
			assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("event type is required"))
		})

		t.Run("Invalid: not allowed event", func(t *ftt.Test) {
			_, err := srv.Event(ctx, &EventRequest{
				Event: model.BotEventTaskKilled,
			}, srvReq)
			assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("unsupported event type"))
		})

		t.Run("Invalid: bad request_uuid", func(t *ftt.Test) {
			_, err := srv.Event(ctx, &EventRequest{
				Event:       model.BotEventLog,
				RequestUUID: "    ",
			}, srvReq)
			assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("bad request_uuid"))
		})

		t.Run("Invalid: bad state JSON", func(t *ftt.Test) {
			_, err := srv.Event(ctx, &EventRequest{
				Event: model.BotEventLog,
				State: botstate.Dict{JSON: []byte("not JSON")},
			}, srvReq)
			assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("bad state JSON dict"))
		})
	})
}

// Copyright 2022 The LUCI Authors.
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

package botsrv

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/tokenserver/auth/machine"

	configpb "go.chromium.org/luci/swarming/proto/config"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/pyproxy"
)

type testRequest struct {
	Session      []byte
	Dimensions   map[string][]string
	PollToken    []byte
	SessionToken []byte
}

func (r *testRequest) ExtractSession() []byte                 { return r.Session }
func (r *testRequest) ExtractPollToken() []byte               { return r.PollToken }
func (r *testRequest) ExtractSessionToken() []byte            { return r.SessionToken }
func (r *testRequest) ExtractDimensions() map[string][]string { return r.Dimensions }
func (r *testRequest) ExtractDebugRequest() any               { return r }

func TestBotHandler(t *testing.T) {
	t.Parallel()

	ftt.Run("With server", t, func(t *ftt.Test) {
		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, now)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "bot:ignored",
			UserExtra: &machine.MachineTokenInfo{
				FQDN: "bot.fqdn",
			},
		})

		srv := &Server{
			router: router.New(),
			hmacSecret: hmactoken.NewStaticSecret(secrets.Secret{
				Active:  []byte("secret"),
				Passive: [][]byte{[]byte("also-secret")},
			}),
			cfg: cfgtest.MockConfigs(ctx, cfgtest.NewMockedConfigs()),
			knownBots: func(ctx context.Context, botID string) (*KnownBotInfo, error) {
				return nil, nil
			},
		}

		var lastBody *testRequest
		var lastRequest *Request
		var nextResponse Response
		var nextError error
		JSON(srv, "/test", func(_ context.Context, body *testRequest, r *Request) (Response, error) {
			lastBody = body
			lastRequest = r
			return nextResponse, nextError
		})

		callRaw := func(body []byte, ct string, mockedResp Response, mockedErr error) (b *testRequest, req *Request, status int, resp string) {
			lastRequest = nil
			nextResponse = mockedResp
			nextError = mockedErr
			rq := httptest.NewRequest("POST", "/test", bytes.NewReader(body)).WithContext(ctx)
			if ct != "" {
				rq.Header.Set("Content-Type", ct)
			}
			rw := httptest.NewRecorder()
			srv.router.ServeHTTP(rw, rq)
			res := rw.Result()
			if res.StatusCode == http.StatusOK {
				assert.Loosely(t, res.Header.Get("Content-Type"), should.Equal("application/json; charset=utf-8"))
			}
			respBody, _ := io.ReadAll(res.Body)
			return lastBody, lastRequest, res.StatusCode, string(respBody)
		}

		call := func(body testRequest, mockedResp Response, mockedErr error) (b *testRequest, req *Request, status int, resp string) {
			blob, err := json.Marshal(&body)
			assert.Loosely(t, err, should.BeNil)
			return callRaw(blob, "application/json; charset=utf-8", mockedResp, mockedErr)
		}

		makePollState := func(id string) *internalspb.PollState {
			return &internalspb.PollState{
				Id:          id,
				Expiry:      timestamppb.New(now.Add(5 * time.Minute)),
				RbeInstance: "some-rbe-instance",
				EnforcedDimensions: []*internalspb.PollState_Dimension{
					{Key: "id", Values: []string{"bot-id"}},
				},
				AuthMethod: &internalspb.PollState_LuciMachineTokenAuth{
					LuciMachineTokenAuth: &internalspb.PollState_LUCIMachineTokenAuth{
						MachineFqdn: "bot.fqdn",
					},
				},
			}
		}

		t.Run("Happy path with poll token", func(t *ftt.Test) {
			pollState := makePollState("poll-state-id")

			req := testRequest{
				Dimensions: map[string][]string{
					"id":   {"bot-id"},
					"pool": {"pool"},
				},
				PollToken: genToken(pollState, []byte("also-secret")),
			}

			body, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, status, should.Equal(http.StatusOK))
			assert.Loosely(t, resp, should.Equal("\"some-response\"\n"))
			assert.Loosely(t, body, should.Resemble(&req))
			assert.Loosely(t, seenReq.BotID, should.Equal("bot-id"))
			assert.Loosely(t, seenReq.SessionID, should.BeEmpty)
			assert.Loosely(t, seenReq.SessionTokenExpired, should.BeFalse)
			assert.Loosely(t, seenReq.PollState, should.Resemble(pollState))
		})

		t.Run("Happy path with session token", func(t *ftt.Test) {
			pollState := makePollState("poll-state-id")

			req := testRequest{
				Dimensions: map[string][]string{
					"id":   {"bot-id"},
					"pool": {"pool"},
				},
				SessionToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "bot-session-id",
					PollState:       pollState,
					Expiry:          timestamppb.New(now.Add(5 * time.Minute)),
				}, []byte("also-secret")),
			}

			body, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, status, should.Equal(http.StatusOK))
			assert.Loosely(t, resp, should.Equal("\"some-response\"\n"))
			assert.Loosely(t, body, should.Resemble(&req))
			assert.Loosely(t, seenReq.BotID, should.Equal("bot-id"))
			assert.Loosely(t, seenReq.SessionID, should.Equal("bot-session-id"))
			assert.Loosely(t, seenReq.SessionTokenExpired, should.BeFalse)
			assert.Loosely(t, seenReq.PollState, should.Resemble(pollState))
		})

		t.Run("Happy path with both tokens", func(t *ftt.Test) {
			pollStateInPollToken := makePollState("in-poll-token")
			pollStateInSessionToken := makePollState("in-session-token")

			req := testRequest{
				Dimensions: map[string][]string{
					"id":   {"bot-id"},
					"pool": {"pool"},
				},
				PollToken: genToken(pollStateInPollToken, []byte("also-secret")),
				SessionToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "bot-session-id",
					PollState:       pollStateInSessionToken,
					Expiry:          timestamppb.New(now.Add(5 * time.Minute)),
				}, []byte("also-secret")),
			}

			body, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, status, should.Equal(http.StatusOK))
			assert.Loosely(t, resp, should.Equal("\"some-response\"\n"))
			assert.Loosely(t, body, should.Resemble(&req))
			assert.Loosely(t, seenReq.BotID, should.Equal("bot-id"))
			assert.Loosely(t, seenReq.SessionID, should.Equal("bot-session-id"))
			assert.Loosely(t, seenReq.SessionTokenExpired, should.BeFalse)
			assert.Loosely(t, seenReq.PollState, should.Resemble(pollStateInPollToken))
		})

		t.Run("Happy path with session token and expired poll token", func(t *ftt.Test) {
			pollStateInPollToken := makePollState("in-poll-token")
			pollStateInPollToken.Expiry = timestamppb.New(now.Add(-5 * time.Minute))

			pollStateInSessionToken := makePollState("in-session-token")

			req := testRequest{
				Dimensions: map[string][]string{
					"id":   {"bot-id"},
					"pool": {"pool"},
				},
				PollToken: genToken(pollStateInPollToken, []byte("also-secret")),
				SessionToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "bot-session-id",
					PollState:       pollStateInSessionToken,
					Expiry:          timestamppb.New(now.Add(5 * time.Minute)),
				}, []byte("also-secret")),
			}

			body, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, status, should.Equal(http.StatusOK))
			assert.Loosely(t, resp, should.Equal("\"some-response\"\n"))
			assert.Loosely(t, body, should.Resemble(&req))
			assert.Loosely(t, seenReq.BotID, should.Equal("bot-id"))
			assert.Loosely(t, seenReq.SessionID, should.Equal("bot-session-id"))
			assert.Loosely(t, seenReq.SessionTokenExpired, should.BeFalse)

			// Used the session token.
			assert.Loosely(t, seenReq.PollState, should.Resemble(pollStateInSessionToken))
		})

		t.Run("Happy path with poll token and expired session token", func(t *ftt.Test) {
			pollStateInPollToken := makePollState("in-poll-token")

			pollStateInSessionToken := makePollState("in-session-token")
			pollStateInSessionToken.Expiry = timestamppb.New(now.Add(-5 * time.Minute))

			req := testRequest{
				Dimensions: map[string][]string{
					"id":   {"bot-id"},
					"pool": {"pool"},
				},
				PollToken: genToken(pollStateInPollToken, []byte("also-secret")),
				SessionToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "bot-session-id",
					PollState:       pollStateInSessionToken,
					Expiry:          pollStateInSessionToken.Expiry,
				}, []byte("also-secret")),
			}

			body, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, status, should.Equal(http.StatusOK))
			assert.Loosely(t, resp, should.Equal("\"some-response\"\n"))
			assert.Loosely(t, body, should.Resemble(&req))
			assert.Loosely(t, seenReq.BotID, should.Equal("bot-id"))
			assert.Loosely(t, seenReq.SessionID, should.BeEmpty)
			assert.Loosely(t, seenReq.SessionTokenExpired, should.BeTrue)

			// Used the poll token.
			assert.Loosely(t, seenReq.PollState, should.Resemble(pollStateInPollToken))
		})

		t.Run("Wrong bot credentials", func(t *ftt.Test) {
			pollState := &internalspb.PollState{
				Id:          "poll-state-id",
				Expiry:      timestamppb.New(now.Add(5 * time.Minute)),
				RbeInstance: "some-rbe-instance",
				EnforcedDimensions: []*internalspb.PollState_Dimension{
					{Key: "id", Values: []string{"bot-id"}},
				},
				AuthMethod: &internalspb.PollState_LuciMachineTokenAuth{
					LuciMachineTokenAuth: &internalspb.PollState_LUCIMachineTokenAuth{
						MachineFqdn: "another.fqdn",
					},
				},
			}

			req := testRequest{
				Dimensions: map[string][]string{
					"id": {"bot-id"},
				},
				PollToken: genToken(pollState, []byte("also-secret")),
			}

			_, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("bad bot credentials: wrong FQDN in the LUCI machine token"))
		})

		t.Run("Bad Content-Type", func(t *ftt.Test) {
			_, seenReq, status, resp := callRaw([]byte("ignored"), "application/x-www-form-urlencoded", nil, nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusBadRequest))
			assert.Loosely(t, resp, should.ContainSubstring("bad content type"))
		})

		t.Run("Not JSON", func(t *ftt.Test) {
			_, seenReq, status, resp := callRaw([]byte("what is this"), "application/json; charset=utf-8", nil, nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusBadRequest))
			assert.Loosely(t, resp, should.ContainSubstring("failed to deserialized"))
		})

		t.Run("Wrong poll token", func(t *ftt.Test) {
			req := testRequest{
				PollToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "not-a-poll-token",
					Expiry:          timestamppb.New(now.Add(5 * time.Minute)),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("failed to verify poll token: invalid payload type"))
		})

		t.Run("Wrong session token", func(t *ftt.Test) {
			req := testRequest{
				SessionToken: genToken(&internalspb.PollState{
					Id:     "not-a-session-token",
					Expiry: timestamppb.New(now.Add(5 * time.Minute)),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("failed to verify session token: invalid payload type"))
		})

		t.Run("Expired poll token", func(t *ftt.Test) {
			req := testRequest{
				PollToken: genToken(&internalspb.PollState{
					Id:     "poll-state-id",
					Expiry: timestamppb.New(now.Add(-5 * time.Minute)),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("no valid poll or state token"))
		})

		t.Run("Expired session token", func(t *ftt.Test) {
			req := testRequest{
				SessionToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "session-id",
					Expiry:          timestamppb.New(now.Add(-5 * time.Minute)),
					PollState:       makePollState("poll-state-id"),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("no valid poll or state token"))
		})

		t.Run("Expired session and poll tokens", func(t *ftt.Test) {
			req := testRequest{
				PollToken: genToken(&internalspb.PollState{
					Id:     "poll-state-id",
					Expiry: timestamppb.New(now.Add(-5 * time.Minute)),
				}, []byte("also-secret")),
				SessionToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "session-id",
					Expiry:          timestamppb.New(now.Add(-5 * time.Minute)),
					PollState:       makePollState("poll-state-id"),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("no valid poll or state token"))
		})

		t.Run("Session token with no session ID", func(t *ftt.Test) {
			req := testRequest{
				SessionToken: genToken(&internalspb.BotSession{
					Expiry:    timestamppb.New(now.Add(5 * time.Minute)),
					PollState: makePollState("poll-state-id"),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusBadRequest))
			assert.Loosely(t, resp, should.ContainSubstring("no session ID"))
		})

		t.Run("Poll state dimension overrides", func(t *ftt.Test) {
			pollState := &internalspb.PollState{
				Id:          "poll-state-id",
				Expiry:      timestamppb.New(now.Add(5 * time.Minute)),
				RbeInstance: "correct-rbe-instance",
				EnforcedDimensions: []*internalspb.PollState_Dimension{
					{Key: "id", Values: []string{"correct-bot-id"}},
					{Key: "keep", Values: []string{"a", "b"}},
					{Key: "override-1", Values: []string{"a"}},
					{Key: "override-2", Values: []string{"b", "a"}},
					{Key: "inject", Values: []string{"a"}},
				},
				AuthMethod: &internalspb.PollState_LuciMachineTokenAuth{
					LuciMachineTokenAuth: &internalspb.PollState_LUCIMachineTokenAuth{
						MachineFqdn: "bot.fqdn",
					},
				},
			}

			req := testRequest{
				Dimensions: map[string][]string{
					"id":         {"wrong-bot-id"},
					"pool":       {"pool"},
					"keep":       {"a", "b"},
					"override-1": {"a", "b"},
					"override-2": {"a", "b"},
					"keep-extra": {"a"},
				},
				PollToken: genToken(pollState, []byte("also-secret")),
			}

			body, seenReq, status, _ := call(req, nil, nil)
			assert.Loosely(t, status, should.Equal(http.StatusOK))
			assert.Loosely(t, body, should.Resemble(&testRequest{
				Dimensions: map[string][]string{
					"id":         {"correct-bot-id"},
					"pool":       {"pool"},
					"keep":       {"a", "b"},
					"override-1": {"a"},
					"override-2": {"b", "a"},
					"keep-extra": {"a"},
					"inject":     {"a"},
				},
				PollToken: req.PollToken,
			}))
			assert.Loosely(t, seenReq.BotID, should.Equal("correct-bot-id"))
		})
	})
}

func TestCheckCredentials(t *testing.T) {
	t.Parallel()

	ftt.Run("No creds", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})

		err := checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_GceAuth{
				GceAuth: &internalspb.PollState_GCEAuth{
					GceProject:  "some-project",
					GceInstance: "some-instance",
				},
			},
		})
		assert.Loosely(t, err, should.ErrLike("expecting GCE VM token auth"))

		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_ServiceAccountAuth_{
				ServiceAccountAuth: &internalspb.PollState_ServiceAccountAuth{
					ServiceAccount: "some-account@example.com",
				},
			},
		})
		assert.Loosely(t, err, should.ErrLike("expecting service account credentials"))

		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_LuciMachineTokenAuth{
				LuciMachineTokenAuth: &internalspb.PollState_LUCIMachineTokenAuth{
					MachineFqdn: "some.fqdn",
				},
			},
		})
		assert.Loosely(t, err, should.ErrLike("expecting LUCI machine token auth"))

		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod:  &internalspb.PollState_IpAllowlistAuth{},
			IpAllowlist: "some-ip-allowlist",
		})
		assert.Loosely(t, err, should.ErrLike("is not in the allowlist"))
	})

	ftt.Run("GCE auth", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "bot:ignored",
			UserExtra: &openid.GoogleComputeTokenInfo{
				Project:  "some-project",
				Instance: "some-instance",
			},
		})

		// OK.
		err := checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_GceAuth{
				GceAuth: &internalspb.PollState_GCEAuth{
					GceProject:  "some-project",
					GceInstance: "some-instance",
				},
			},
		})
		assert.Loosely(t, err, should.BeNil)

		// Wrong parameters #1.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_GceAuth{
				GceAuth: &internalspb.PollState_GCEAuth{
					GceProject:  "another-project",
					GceInstance: "some-instance",
				},
			},
		})
		assert.Loosely(t, err, should.ErrLike("wrong GCE VM token"))

		// Wrong parameters #2.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_GceAuth{
				GceAuth: &internalspb.PollState_GCEAuth{
					GceProject:  "some-project",
					GceInstance: "another-instance",
				},
			},
		})
		assert.Loosely(t, err, should.ErrLike("wrong GCE VM token"))
	})

	ftt.Run("Service account auth", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:some-account@example.com",
		})

		// OK.
		err := checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_ServiceAccountAuth_{
				ServiceAccountAuth: &internalspb.PollState_ServiceAccountAuth{
					ServiceAccount: "some-account@example.com",
				},
			},
		})
		assert.Loosely(t, err, should.BeNil)

		// Wrong email.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_ServiceAccountAuth_{
				ServiceAccountAuth: &internalspb.PollState_ServiceAccountAuth{
					ServiceAccount: "another-account@example.com",
				},
			},
		})
		assert.Loosely(t, err, should.ErrLike("wrong service account"))
	})

	ftt.Run("Machine token auth", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "bot:ignored",
			UserExtra: &machine.MachineTokenInfo{
				FQDN: "some.fqdn",
			},
		})

		// OK.
		err := checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_LuciMachineTokenAuth{
				LuciMachineTokenAuth: &internalspb.PollState_LUCIMachineTokenAuth{
					MachineFqdn: "some.fqdn",
				},
			},
		})
		assert.Loosely(t, err, should.BeNil)

		// Wrong FQDN.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_LuciMachineTokenAuth{
				LuciMachineTokenAuth: &internalspb.PollState_LUCIMachineTokenAuth{
					MachineFqdn: "another.fqdn",
				},
			},
		})
		assert.Loosely(t, err, should.ErrLike("wrong FQDN in the LUCI machine token"))
	})

	ftt.Run("IP allowlist", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			PeerIPOverride: net.ParseIP("127.1.1.1"),
			FakeDB: authtest.NewFakeDB(
				authtest.MockIPAllowlist("127.1.1.1", "good"),
				authtest.MockIPAllowlist("127.2.2.2", "bad"),
			),
		})

		// OK.
		err := checkCredentials(ctx, &internalspb.PollState{
			AuthMethod:  &internalspb.PollState_IpAllowlistAuth{},
			IpAllowlist: "good",
		})
		assert.Loosely(t, err, should.BeNil)

		// Wrong IP.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod:  &internalspb.PollState_IpAllowlistAuth{},
			IpAllowlist: "bad",
		})
		assert.Loosely(t, err, should.ErrLike("bot IP 127.1.1.1 is not in the allowlist"))
	})
}

func TestPythonProxying(t *testing.T) {
	t.Parallel()

	ftt.Run("With server", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(123)))

		var m sync.Mutex
		var calls []string

		recordCall := func(call string) {
			m.Lock()
			defer m.Unlock()
			calls = append(calls, call)
		}

		collectCalls := func() []string {
			m.Lock()
			defer m.Unlock()
			out := calls
			calls = nil
			return out
		}

		// A server representing Python.
		pySrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			recordCall("py:" + req.URL.RequestURI())
		}))
		defer pySrv.Close()

		cfg := cfgtest.MockConfigs(ctx, &cfgtest.MockedConfigs{
			Settings: &configpb.SettingsCfg{
				TrafficMigration: &configpb.TrafficMigration{
					Routes: []*configpb.TrafficMigration_Route{
						// Should be ignore: Go only route.
						{Name: "/swarming/api/v1/bot/rbe/something", RouteToGoPercent: 0},
						// Always routed to Py.
						{Name: "/swarming/api/v1/bot/python", RouteToGoPercent: 0},
						// Always routed to Go.
						{Name: "/swarming/api/v1/bot/go", RouteToGoPercent: 100},
						// Random chance.
						{Name: "/swarming/api/v1/bot/rand", RouteToGoPercent: 20},
					},
				},
			},
		})

		goSrv := router.New()
		middlewares := []router.Middleware{
			pythonProxyMiddleware(pyproxy.NewProxy(cfg, pySrv.URL)),
		}

		routes := []string{
			"/swarming/api/v1/bot/rbe/something",
			"/swarming/api/v1/bot/rbe/something-else",
			"/swarming/api/v1/bot/unconfigured",
			"/swarming/api/v1/bot/python",
			"/swarming/api/v1/bot/python/:Param",
			"/swarming/api/v1/bot/go",
			"/swarming/api/v1/bot/go/:Param",
			"/swarming/api/v1/bot/rand",
			"/swarming/api/v1/bot/rand/:Param",
		}
		for _, route := range routes {
			goSrv.POST(route, middlewares, func(ctx *router.Context) {
				recordCall("go:" + ctx.Request.URL.RequestURI())
			})
		}

		post := func(uri string) {
			req := httptest.NewRequest("POST", uri, strings.NewReader("{}"))
			rr := httptest.NewRecorder()
			goSrv.ServeHTTP(rr, req.WithContext(ctx))
			assert.That(t, rr.Code, should.Equal(200))
		}

		// Routes unconditionally routed to Go.
		post("/swarming/api/v1/bot/rbe/something")
		post("/swarming/api/v1/bot/rbe/something-else")
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"go:/swarming/api/v1/bot/rbe/something",
			"go:/swarming/api/v1/bot/rbe/something-else",
		}))

		// Routes not in the config are routed to Py.
		post("/swarming/api/v1/bot/unconfigured")
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"py:/swarming/api/v1/bot/unconfigured",
		}))

		// Routes configured to always route to Py.
		post("/swarming/api/v1/bot/python")
		post("/swarming/api/v1/bot/python/arg")
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"py:/swarming/api/v1/bot/python",
			"py:/swarming/api/v1/bot/python/arg",
		}))

		// Routes configured to always route to Go.
		post("/swarming/api/v1/bot/go")
		post("/swarming/api/v1/bot/go/arg")
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"go:/swarming/api/v1/bot/go",
			"go:/swarming/api/v1/bot/go/arg",
		}))

		// Routes with random splitting.
		for i := 0; i < 5; i++ {
			post("/swarming/api/v1/bot/rand")
			post("/swarming/api/v1/bot/rand/arg")
		}
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"py:/swarming/api/v1/bot/rand",
			"py:/swarming/api/v1/bot/rand/arg",
			"go:/swarming/api/v1/bot/rand",
			"py:/swarming/api/v1/bot/rand/arg",
			"py:/swarming/api/v1/bot/rand",
			"py:/swarming/api/v1/bot/rand/arg",
			"py:/swarming/api/v1/bot/rand",
			"go:/swarming/api/v1/bot/rand/arg",
			"py:/swarming/api/v1/bot/rand",
			"go:/swarming/api/v1/bot/rand/arg",
		}))
	})
}

func genToken(msg proto.Message, secret []byte) []byte {
	tok, err := hmactoken.NewStaticSecret(secrets.Secret{Active: secret}).GenerateToken(msg)
	if err != nil {
		panic(err)
	}
	return tok
}

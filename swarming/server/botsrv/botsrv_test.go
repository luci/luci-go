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
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/tokenserver/auth/machine"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/hmactoken"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testRequest struct {
	Dimensions   map[string][]string
	PollToken    []byte
	SessionToken []byte
}

func (r *testRequest) ExtractPollToken() []byte               { return r.PollToken }
func (r *testRequest) ExtractSessionToken() []byte            { return r.SessionToken }
func (r *testRequest) ExtractDimensions() map[string][]string { return r.Dimensions }
func (r *testRequest) ExtractDebugRequest() any               { return r }

func TestBotHandler(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx := context.Background()
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
		}

		var lastBody *testRequest
		var lastRequest *Request
		var nextResponse Response
		var nextError error
		InstallHandler(srv, "/test", func(_ context.Context, body *testRequest, r *Request) (Response, error) {
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
				So(res.Header.Get("Content-Type"), ShouldEqual, "application/json; charset=utf-8")
			}
			respBody, _ := io.ReadAll(res.Body)
			return lastBody, lastRequest, res.StatusCode, string(respBody)
		}

		call := func(body testRequest, mockedResp Response, mockedErr error) (b *testRequest, req *Request, status int, resp string) {
			blob, err := json.Marshal(&body)
			So(err, ShouldBeNil)
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

		Convey("Happy path with poll token", func() {
			pollState := makePollState("poll-state-id")

			req := testRequest{
				Dimensions: map[string][]string{
					"id":   {"bot-id"},
					"pool": {"pool"},
				},
				PollToken: genToken(pollState, []byte("also-secret")),
			}

			body, seenReq, status, resp := call(req, "some-response", nil)
			So(status, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\"some-response\"\n")
			So(body, ShouldResemble, &req)
			So(seenReq.BotID, ShouldEqual, "bot-id")
			So(seenReq.SessionID, ShouldEqual, "")
			So(seenReq.PollState, ShouldResembleProto, pollState)
		})

		Convey("Happy path with session token", func() {
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
			So(status, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\"some-response\"\n")
			So(body, ShouldResemble, &req)
			So(seenReq.BotID, ShouldEqual, "bot-id")
			So(seenReq.SessionID, ShouldEqual, "bot-session-id")
			So(seenReq.PollState, ShouldResembleProto, pollState)
		})

		Convey("Happy path with both tokens", func() {
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
			So(status, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\"some-response\"\n")
			So(body, ShouldResemble, &req)
			So(seenReq.BotID, ShouldEqual, "bot-id")
			So(seenReq.SessionID, ShouldEqual, "bot-session-id")
			So(seenReq.PollState, ShouldResembleProto, pollStateInPollToken)
		})

		Convey("Happy path with session token and expired poll token", func() {
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
			So(status, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\"some-response\"\n")
			So(body, ShouldResemble, &req)
			So(seenReq.BotID, ShouldEqual, "bot-id")
			So(seenReq.SessionID, ShouldEqual, "bot-session-id")

			// Used the session token.
			So(seenReq.PollState, ShouldResembleProto, pollStateInSessionToken)
		})

		Convey("Wrong bot credentials", func() {
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
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "bad bot credentials: wrong FQDN in the LUCI machine token")
		})

		Convey("Bad Content-Type", func() {
			_, seenReq, status, resp := callRaw([]byte("ignored"), "application/x-www-form-urlencoded", nil, nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldContainSubstring, "bad content type")
		})

		Convey("Not JSON", func() {
			_, seenReq, status, resp := callRaw([]byte("what is this"), "application/json; charset=utf-8", nil, nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldContainSubstring, "failed to deserialized")
		})

		Convey("Wrong poll token", func() {
			req := testRequest{
				PollToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "not-a-poll-token",
					Expiry:          timestamppb.New(now.Add(5 * time.Minute)),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "failed to verify poll token: invalid payload type")
		})

		Convey("Wrong session token", func() {
			req := testRequest{
				SessionToken: genToken(&internalspb.PollState{
					Id:     "not-a-session-token",
					Expiry: timestamppb.New(now.Add(5 * time.Minute)),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "failed to verify session token: invalid payload type")
		})

		Convey("Expired poll token", func() {
			req := testRequest{
				PollToken: genToken(&internalspb.PollState{
					Id:     "poll-state-id",
					Expiry: timestamppb.New(now.Add(-5 * time.Minute)),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "no valid poll or state token")
		})

		Convey("Expired session token", func() {
			req := testRequest{
				SessionToken: genToken(&internalspb.BotSession{
					RbeBotSessionId: "session-id",
					Expiry:          timestamppb.New(now.Add(-5 * time.Minute)),
					PollState:       makePollState("poll-state-id"),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "no valid poll or state token")
		})

		Convey("Expired session and poll tokens", func() {
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
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "no valid poll or state token")
		})

		Convey("Session token with no session ID", func() {
			req := testRequest{
				SessionToken: genToken(&internalspb.BotSession{
					Expiry:    timestamppb.New(now.Add(5 * time.Minute)),
					PollState: makePollState("poll-state-id"),
				}, []byte("also-secret")),
			}
			_, seenReq, status, resp := call(req, "some-response", nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldContainSubstring, "no session ID")
		})

		Convey("Poll state dimension overrides", func() {
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
			So(status, ShouldEqual, http.StatusOK)
			So(body, ShouldResemble, &testRequest{
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
			})
			So(seenReq.BotID, ShouldEqual, "correct-bot-id")
		})
	})
}

func TestCheckCredentials(t *testing.T) {
	t.Parallel()

	Convey("No creds", t, func() {
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
		So(err, ShouldErrLike, "expecting GCE VM token auth")

		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_ServiceAccountAuth_{
				ServiceAccountAuth: &internalspb.PollState_ServiceAccountAuth{
					ServiceAccount: "some-account@example.com",
				},
			},
		})
		So(err, ShouldErrLike, "expecting service account credentials")

		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_LuciMachineTokenAuth{
				LuciMachineTokenAuth: &internalspb.PollState_LUCIMachineTokenAuth{
					MachineFqdn: "some.fqdn",
				},
			},
		})
		So(err, ShouldErrLike, "expecting LUCI machine token auth")

		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod:  &internalspb.PollState_IpAllowlistAuth{},
			IpAllowlist: "some-ip-allowlist",
		})
		So(err, ShouldErrLike, "is not in the allowlist")
	})

	Convey("GCE auth", t, func() {
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
		So(err, ShouldBeNil)

		// Wrong parameters #1.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_GceAuth{
				GceAuth: &internalspb.PollState_GCEAuth{
					GceProject:  "another-project",
					GceInstance: "some-instance",
				},
			},
		})
		So(err, ShouldErrLike, "wrong GCE VM token")

		// Wrong parameters #2.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_GceAuth{
				GceAuth: &internalspb.PollState_GCEAuth{
					GceProject:  "some-project",
					GceInstance: "another-instance",
				},
			},
		})
		So(err, ShouldErrLike, "wrong GCE VM token")
	})

	Convey("Service account auth", t, func() {
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
		So(err, ShouldBeNil)

		// Wrong email.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_ServiceAccountAuth_{
				ServiceAccountAuth: &internalspb.PollState_ServiceAccountAuth{
					ServiceAccount: "another-account@example.com",
				},
			},
		})
		So(err, ShouldErrLike, "wrong service account")
	})

	Convey("Machine token auth", t, func() {
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
		So(err, ShouldBeNil)

		// Wrong FQDN.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod: &internalspb.PollState_LuciMachineTokenAuth{
				LuciMachineTokenAuth: &internalspb.PollState_LUCIMachineTokenAuth{
					MachineFqdn: "another.fqdn",
				},
			},
		})
		So(err, ShouldErrLike, "wrong FQDN in the LUCI machine token")
	})

	Convey("IP allowlist", t, func() {
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
		So(err, ShouldBeNil)

		// Wrong IP.
		err = checkCredentials(ctx, &internalspb.PollState{
			AuthMethod:  &internalspb.PollState_IpAllowlistAuth{},
			IpAllowlist: "bad",
		})
		So(err, ShouldErrLike, "bot IP 127.1.1.1 is not in the allowlist")
	})
}

func genToken(msg proto.Message, secret []byte) []byte {
	tok, err := hmactoken.NewStaticSecret(secrets.Secret{Active: secret}).GenerateToken(msg)
	if err != nil {
		panic(err)
	}
	return tok
}

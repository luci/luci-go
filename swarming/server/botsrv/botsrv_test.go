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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

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

		var hmacSecretKey atomic.Value
		hmacSecretKey.Store(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		srv := &Server{
			router:        router.New(),
			hmacSecretKey: hmacSecretKey,
		}

		var lastRequest *Request
		var nextResponse Response
		var nextError error
		srv.InstallHandler("/test", func(_ context.Context, r *Request) (Response, error) {
			lastRequest = r
			return nextResponse, nextError
		})

		callRaw := func(body []byte, ct string, mockedResp Response, mockedErr error) (req *Request, status int, resp string) {
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
			return lastRequest, res.StatusCode, string(respBody)
		}

		call := func(body RequestBody, mockedResp Response, mockedErr error) (req *Request, status int, resp string) {
			blob, err := json.Marshal(&body)
			So(err, ShouldBeNil)
			return callRaw(blob, "application/json; charset=utf-8", mockedResp, mockedErr)
		}

		Convey("Happy path", func() {
			pollToken := &internalspb.PollState{
				Id:          "poll-state-id",
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

			req := RequestBody{
				Dimensions: map[string][]string{
					"id": {"bot-id"},
				},
				State: map[string]interface{}{
					"state": "val",
				},
				Version: "some-bot-version",
				RBEState: RBEState{
					Instance:  "some-rbe-instance",
					PollToken: genPollToken(pollToken, internalspb.TaggedMessage_POLL_STATE, []byte("also-secret")),
				},
			}

			seenReq, status, resp := call(req, "some-response", nil)
			So(status, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\"some-response\"\n")
			So(seenReq.Body, ShouldResemble, &req)
			So(seenReq.PollState, ShouldResembleProto, pollToken)
		})

		Convey("Wrong bot credentials", func() {
			pollToken := &internalspb.PollState{
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

			req := RequestBody{
				Dimensions: map[string][]string{
					"id": {"bot-id"},
				},
				State: map[string]interface{}{
					"state": "val",
				},
				Version: "some-bot-version",
				RBEState: RBEState{
					Instance:  "some-rbe-instance",
					PollToken: genPollToken(pollToken, internalspb.TaggedMessage_POLL_STATE, []byte("also-secret")),
				},
			}

			seenReq, status, resp := call(req, "some-response", nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "bad bot credentials: wrong FQDN in the LUCI machine token")
		})

		Convey("Bad Content-Type", func() {
			seenReq, status, resp := callRaw([]byte("ignored"), "application/x-www-form-urlencoded", nil, nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldContainSubstring, "bad content type")
		})

		Convey("Not JSON", func() {
			seenReq, status, resp := callRaw([]byte("what is this"), "application/json; charset=utf-8", nil, nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldContainSubstring, "failed to deserialized")
		})

		Convey("Wrong poll token", func() {
			req := RequestBody{
				RBEState: RBEState{
					PollToken: genPollToken(&internalspb.PollState{
						Id:     "poll-state-id",
						Expiry: timestamppb.New(now.Add(5 * time.Minute)),
					}, 123, []byte("also-secret")),
				},
			}
			seenReq, status, resp := call(req, "some-response", nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "failed to verify poll token: invalid payload type")
		})

		Convey("Expired poll token", func() {
			req := RequestBody{
				RBEState: RBEState{
					PollToken: genPollToken(&internalspb.PollState{
						Id:     "poll-state-id",
						Expiry: timestamppb.New(now.Add(-5 * time.Minute)),
					}, internalspb.TaggedMessage_POLL_STATE, []byte("also-secret")),
				},
			}
			seenReq, status, resp := call(req, "some-response", nil)
			So(seenReq, ShouldBeNil)
			So(status, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldContainSubstring, "poll state token expired 5m0s ago")
		})

		Convey("Poll state token overrides", func() {
			pollToken := &internalspb.PollState{
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

			req := RequestBody{
				Dimensions: map[string][]string{
					"id":         {"wrong-bot-id"},
					"keep":       {"a", "b"},
					"override-1": {"a", "b"},
					"override-2": {"a", "b"},
					"keep-extra": {"a"},
				},
				RBEState: RBEState{
					Instance:  "wrong-rbe-instance",
					PollToken: genPollToken(pollToken, internalspb.TaggedMessage_POLL_STATE, []byte("also-secret")),
				},
			}

			seenReq, status, _ := call(req, nil, nil)
			So(status, ShouldEqual, http.StatusOK)
			So(seenReq.Body, ShouldResemble, &RequestBody{
				Dimensions: map[string][]string{
					"id":         {"correct-bot-id"},
					"keep":       {"a", "b"},
					"override-1": {"a"},
					"override-2": {"b", "a"},
					"keep-extra": {"a"},
					"inject":     {"a"},
				},
				RBEState: RBEState{
					Instance:  "correct-rbe-instance",
					PollToken: req.RBEState.PollToken,
				},
			})
		})
	})
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		var hmacSecretKey atomic.Value
		hmacSecretKey.Store(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		srv := &Server{hmacSecretKey: hmacSecretKey}

		Convey("Good token", func() {
			original := &internalspb.PollState{Id: "some-id"}

			extracted := &internalspb.PollState{}
			err := srv.validateToken(genPollToken(
				original,
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("secret"),
			), extracted)
			So(err, ShouldBeNil)
			So(extracted, ShouldResembleProto, original)

			// Non-active secret is also OK.
			extracted = &internalspb.PollState{}
			err = srv.validateToken(genPollToken(
				original,
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("also-secret"),
			), extracted)
			So(err, ShouldBeNil)
			So(extracted, ShouldResembleProto, original)
		})

		Convey("Bad TaggedMessage proto", func() {
			err := srv.validateToken([]byte("what is this"), &internalspb.PollState{})
			So(err, ShouldErrLike, "failed to deserialize TaggedMessage")
		})

		Convey("Wrong type", func() {
			err := srv.validateToken(genPollToken(
				&internalspb.PollState{Id: "some-id"},
				123,
				[]byte("secret"),
			), &internalspb.PollState{})
			So(err, ShouldErrLike, "invalid payload type")
		})

		Convey("Bad MAC", func() {
			err := srv.validateToken(genPollToken(
				&internalspb.PollState{Id: "some-id"},
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("some-other-secret"),
			), &internalspb.PollState{})
			So(err, ShouldErrLike, "bad token HMAC")
		})
	})
}

func TestGenerateToken(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		var hmacSecretKey atomic.Value
		hmacSecretKey.Store(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		srv := &Server{hmacSecretKey: hmacSecretKey}

		Convey("PollState", func() {
			original := &internalspb.PollState{Id: "testing"}
			tok, err := srv.generateToken(original)
			So(err, ShouldBeNil)

			decoded := &internalspb.PollState{}
			So(srv.validateToken(tok, decoded), ShouldBeNil)

			So(decoded, ShouldResembleProto, original)
		})

		Convey("BotSession", func() {
			original := &internalspb.BotSession{RbeBotSessionId: "testing"}
			tok, err := srv.generateToken(original)
			So(err, ShouldBeNil)

			decoded := &internalspb.BotSession{}
			So(srv.validateToken(tok, decoded), ShouldBeNil)

			So(decoded, ShouldResembleProto, original)
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
				authtest.MockIPWhitelist("127.1.1.1", "good"),
				authtest.MockIPWhitelist("127.2.2.2", "bad"),
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

func genPollToken(state *internalspb.PollState, typ internalspb.TaggedMessage_PayloadType, secret []byte) []byte {
	payload, err := proto.Marshal(state)
	if err != nil {
		panic(err)
	}

	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "%d\n", typ)
	_, _ = mac.Write(payload)
	digest := mac.Sum(nil)

	blob, err := proto.Marshal(&internalspb.TaggedMessage{
		PayloadType: typ,
		Payload:     payload,
		HmacSha256:  digest,
	})
	if err != nil {
		panic(err)
	}
	return blob
}

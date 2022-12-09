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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"

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

		var pollTokenKey atomic.Value
		pollTokenKey.Store(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		srv := &Server{
			router:       router.New(),
			pollTokenKey: pollTokenKey,
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

func TestValidatePollToken(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		var pollTokenKey atomic.Value
		pollTokenKey.Store(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		srv := &Server{pollTokenKey: pollTokenKey}

		Convey("Good token", func() {
			original := &internalspb.PollState{Id: "some-id"}

			extracted, err := srv.validatePollToken(genPollToken(
				original,
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("secret"),
			))
			So(err, ShouldBeNil)
			So(extracted, ShouldResembleProto, original)

			// Non-active secret is also OK.
			extracted, err = srv.validatePollToken(genPollToken(
				original,
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("also-secret"),
			))
			So(err, ShouldBeNil)
			So(extracted, ShouldResembleProto, original)
		})

		Convey("Bad TaggedMessage proto", func() {
			_, err := srv.validatePollToken([]byte("what is this"))
			So(err, ShouldErrLike, "failed to deserialize TaggedMessage")
		})

		Convey("Wrong type", func() {
			_, err := srv.validatePollToken(genPollToken(
				&internalspb.PollState{Id: "some-id"},
				123,
				[]byte("secret"),
			))
			So(err, ShouldErrLike, "invalid payload type")
		})

		Convey("Bad MAC", func() {
			_, err := srv.validatePollToken(genPollToken(
				&internalspb.PollState{Id: "some-id"},
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("some-other-secret"),
			))
			So(err, ShouldErrLike, "bad poll token HMAC")
		})
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

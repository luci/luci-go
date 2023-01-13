// Copyright 2023 The LUCI Authors.
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

package hmactoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/secrets"

	internalspb "go.chromium.org/luci/swarming/proto/internals"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateToken(t *testing.T) {
	t.Parallel()

	Convey("With secret", t, func() {
		s := NewStaticSecret(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		Convey("Good token", func() {
			original := &internalspb.PollState{Id: "some-id"}

			extracted := &internalspb.PollState{}
			err := s.ValidateToken(genPollToken(
				original,
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("secret"),
			), extracted)
			So(err, ShouldBeNil)
			So(extracted, ShouldResembleProto, original)

			// Non-active secret is also OK.
			extracted = &internalspb.PollState{}
			err = s.ValidateToken(genPollToken(
				original,
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("also-secret"),
			), extracted)
			So(err, ShouldBeNil)
			So(extracted, ShouldResembleProto, original)
		})

		Convey("Bad TaggedMessage proto", func() {
			err := s.ValidateToken([]byte("what is this"), &internalspb.PollState{})
			So(err, ShouldErrLike, "failed to deserialize TaggedMessage")
		})

		Convey("Wrong type", func() {
			err := s.ValidateToken(genPollToken(
				&internalspb.PollState{Id: "some-id"},
				123,
				[]byte("secret"),
			), &internalspb.PollState{})
			So(err, ShouldErrLike, "invalid payload type")
		})

		Convey("Bad MAC", func() {
			err := s.ValidateToken(genPollToken(
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

	Convey("With secret", t, func() {
		s := NewStaticSecret(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		Convey("PollState", func() {
			original := &internalspb.PollState{Id: "testing"}
			tok, err := s.GenerateToken(original)
			So(err, ShouldBeNil)

			decoded := &internalspb.PollState{}
			So(s.ValidateToken(tok, decoded), ShouldBeNil)

			So(decoded, ShouldResembleProto, original)
		})

		Convey("BotSession", func() {
			original := &internalspb.BotSession{RbeBotSessionId: "testing"}
			tok, err := s.GenerateToken(original)
			So(err, ShouldBeNil)

			decoded := &internalspb.BotSession{}
			So(s.ValidateToken(tok, decoded), ShouldBeNil)

			So(decoded, ShouldResembleProto, original)
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

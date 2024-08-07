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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/secrets"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
)

func TestValidateToken(t *testing.T) {
	t.Parallel()

	ftt.Run("With secret", t, func(t *ftt.Test) {
		s := NewStaticSecret(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		t.Run("Good token", func(t *ftt.Test) {
			original := &internalspb.PollState{Id: "some-id"}

			extracted := &internalspb.PollState{}
			err := s.ValidateToken(genPollToken(
				original,
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("secret"),
			), extracted)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, extracted, should.Resemble(original))

			// Non-active secret is also OK.
			extracted = &internalspb.PollState{}
			err = s.ValidateToken(genPollToken(
				original,
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("also-secret"),
			), extracted)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, extracted, should.Resemble(original))
		})

		t.Run("Bad TaggedMessage proto", func(t *ftt.Test) {
			err := s.ValidateToken([]byte("what is this"), &internalspb.PollState{})
			assert.Loosely(t, err, should.ErrLike("failed to deserialize TaggedMessage"))
		})

		t.Run("Wrong type", func(t *ftt.Test) {
			err := s.ValidateToken(genPollToken(
				&internalspb.PollState{Id: "some-id"},
				123,
				[]byte("secret"),
			), &internalspb.PollState{})
			assert.Loosely(t, err, should.ErrLike("invalid payload type"))
		})

		t.Run("Bad MAC", func(t *ftt.Test) {
			err := s.ValidateToken(genPollToken(
				&internalspb.PollState{Id: "some-id"},
				internalspb.TaggedMessage_POLL_STATE,
				[]byte("some-other-secret"),
			), &internalspb.PollState{})
			assert.Loosely(t, err, should.ErrLike("bad token HMAC"))
		})
	})
}

func TestGenerateToken(t *testing.T) {
	t.Parallel()

	ftt.Run("With secret", t, func(t *ftt.Test) {
		s := NewStaticSecret(secrets.Secret{
			Active:  []byte("secret"),
			Passive: [][]byte{[]byte("also-secret")},
		})

		t.Run("PollState", func(t *ftt.Test) {
			original := &internalspb.PollState{Id: "testing"}
			tok, err := s.GenerateToken(original)
			assert.Loosely(t, err, should.BeNil)

			decoded := &internalspb.PollState{}
			assert.Loosely(t, s.ValidateToken(tok, decoded), should.BeNil)

			assert.Loosely(t, decoded, should.Resemble(original))
		})

		t.Run("BotSession", func(t *ftt.Test) {
			original := &internalspb.BotSession{RbeBotSessionId: "testing"}
			tok, err := s.GenerateToken(original)
			assert.Loosely(t, err, should.BeNil)

			decoded := &internalspb.BotSession{}
			assert.Loosely(t, s.ValidateToken(tok, decoded), should.BeNil)

			assert.Loosely(t, decoded, should.Resemble(original))
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

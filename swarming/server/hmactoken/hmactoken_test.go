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

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/secrets"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
)

func TestTagVerify(t *testing.T) {
	t.Parallel()

	s1 := NewStaticSecret(secrets.Secret{Active: []byte("secret")})

	tag := s1.Tag([]byte("pfx"), []byte("body"))
	assert.That(t, s1.Verify([]byte("pfx"), []byte("body"), tag), should.BeTrue)
	assert.That(t, s1.Verify([]byte("pfx2"), []byte("body"), tag), should.BeFalse)
	assert.That(t, s1.Verify([]byte("pfx"), []byte("body2"), tag), should.BeFalse)

	s2 := NewStaticSecret(secrets.Secret{
		Active: []byte("newer-secret"),
		Passive: [][]byte{
			[]byte("ignored"),
			[]byte("secret"),
		},
	})
	assert.That(t, s2.Verify([]byte("pfx"), []byte("body"), tag), should.BeTrue)
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

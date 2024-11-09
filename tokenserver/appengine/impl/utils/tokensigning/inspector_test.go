// Copyright 2017 The LUCI Authors.
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

package tokensigning

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func TestInspectToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)

	signer := signingtest.NewSigner(&signing.ServiceInfo{
		ServiceAccountName: "service@example.com",
	})
	inspector := inspectorForTest(signer, "")

	original := &messages.Subtoken{
		DelegatedIdentity: "user:delegated@example.com",
		RequestorIdentity: "user:requestor@example.com",
		CreationTime:      clock.Now(ctx).Unix(),
		ValidityDuration:  3600,
		Audience:          []string{"*"},
		Services:          []string{"*"},
	}
	good, _ := signerForTest(signer, "").SignToken(ctx, original)

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		ins, err := inspector.InspectToken(ctx, good)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ins.Signed, should.BeTrue)
		assert.Loosely(t, ins.NonExpired, should.BeTrue)
		assert.Loosely(t, ins.InvalidityReason, should.BeEmpty)
		assert.Loosely(t, ins.Envelope, should.HaveType[*messages.DelegationToken])
		assert.Loosely(t, ins.Body, should.Resemble(original))
	})

	ftt.Run("Not base64", t, func(t *ftt.Test) {
		ins, err := inspector.InspectToken(ctx, "@@@@@@@@@@@@@")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ins, should.Resemble(&Inspection{
			InvalidityReason: "not base64 - illegal base64 data at input byte 0",
		}))
	})

	ftt.Run("Not valid envelope proto", t, func(t *ftt.Test) {
		ins, err := inspector.InspectToken(ctx, "zzzz")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ins.InvalidityReason, should.HavePrefix("can't unmarshal the envelope - proto"))
	})

	ftt.Run("Bad signature", t, func(t *ftt.Test) {
		env, _, _ := deserializeForTest(ctx, good, signer)
		env.Pkcs1Sha256Sig = []byte("lalala")
		blob, _ := proto.Marshal(env)
		tok := base64.RawURLEncoding.EncodeToString(blob)

		ins, err := inspector.InspectToken(ctx, tok)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ins.Signed, should.BeFalse)
		assert.Loosely(t, ins.NonExpired, should.BeTrue)
		assert.Loosely(t, ins.InvalidityReason, should.Equal("bad signature - crypto/rsa: verification error"))
		assert.Loosely(t, ins.Envelope, should.HaveType[*messages.DelegationToken])
		assert.Loosely(t, ins.Body, should.Resemble(original)) // recovered the token body nonetheless
	})

	ftt.Run("Wrong SigningContext", t, func(t *ftt.Test) {
		inspectorWithCtx := inspectorForTest(signer, "Some context")

		// Symptoms are same as in "Bad signature".
		ins, err := inspectorWithCtx.InspectToken(ctx, good)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ins.Signed, should.BeFalse)
		assert.Loosely(t, ins.NonExpired, should.BeTrue)
		assert.Loosely(t, ins.InvalidityReason, should.Equal("bad signature - crypto/rsa: verification error"))
		assert.Loosely(t, ins.Envelope, should.HaveType[*messages.DelegationToken])
		assert.Loosely(t, ins.Body, should.Resemble(original)) // recovered the token body nonetheless
	})

	ftt.Run("Expired", t, func(t *ftt.Test) {
		tc.Add(2 * time.Hour)

		ins, err := inspector.InspectToken(ctx, good)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ins.Signed, should.BeTrue)
		assert.Loosely(t, ins.NonExpired, should.BeFalse)
		assert.Loosely(t, ins.InvalidityReason, should.Equal("expired"))
		assert.Loosely(t, ins.Envelope, should.HaveType[*messages.DelegationToken])
		assert.Loosely(t, ins.Body, should.Resemble(original))
	})
}

func inspectorForTest(certs CertificatesSupplier, signingCtx string) *Inspector {
	return &Inspector{
		Certificates:   certs,
		SigningContext: signingCtx,
		Envelope:       func() proto.Message { return &messages.DelegationToken{} },
		Body:           func() proto.Message { return &messages.Subtoken{} },
		Unwrap: func(e proto.Message) Unwrapped {
			env := e.(*messages.DelegationToken)
			return Unwrapped{
				Body:         env.SerializedSubtoken,
				RsaSHA256Sig: env.Pkcs1Sha256Sig,
				KeyID:        env.SigningKeyId,
			}
		},
		Lifespan: func(b proto.Message) Lifespan {
			body := b.(*messages.Subtoken)
			return Lifespan{
				NotBefore: time.Unix(body.CreationTime, 0),
				NotAfter:  time.Unix(body.CreationTime+int64(body.ValidityDuration), 0),
			}
		},
	}
}

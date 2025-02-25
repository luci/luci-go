// Copyright 2016 The LUCI Authors.
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

package delegation

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

	admin "go.chromium.org/luci/tokenserver/api/admin/v1"
)

func TestInspectDelegationToken(t *testing.T) {
	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)

	signer := signingtest.NewSigner(&signing.ServiceInfo{
		ServiceAccountName: "service@example.com",
	})
	rpc := InspectDelegationTokenRPC{
		Signer: signer,
	}

	original := &messages.Subtoken{
		DelegatedIdentity: "user:delegated@example.com",
		RequestorIdentity: "user:requestor@example.com",
		CreationTime:      clock.Now(ctx).Unix(),
		ValidityDuration:  3600,
		Audience:          []string{"*"},
		Services:          []string{"*"},
	}

	tok, _ := SignToken(ctx, rpc.Signer, original)

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		resp, err := rpc.InspectDelegationToken(ctx, &admin.InspectDelegationTokenRequest{
			Token: tok,
		})
		assert.Loosely(t, err, should.BeNil)

		resp.Envelope.Pkcs1Sha256Sig = nil
		resp.Envelope.SerializedSubtoken = nil
		assert.Loosely(t, resp, should.Match(&admin.InspectDelegationTokenResponse{
			Valid:      true,
			Signed:     true,
			NonExpired: true,
			Envelope: &messages.DelegationToken{
				SignerId:     "user:service@example.com",
				SigningKeyId: signer.KeyNameForTest(),
			},
			Subtoken: original,
		}))
	})

	ftt.Run("Not base64", t, func(t *ftt.Test) {
		resp, err := rpc.InspectDelegationToken(ctx, &admin.InspectDelegationTokenRequest{
			Token: "@@@@@@@@@@@@@",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp, should.Match(&admin.InspectDelegationTokenResponse{
			InvalidityReason: "not base64 - illegal base64 data at input byte 0",
		}))
	})

	ftt.Run("Not valid envelope proto", t, func(t *ftt.Test) {
		resp, err := rpc.InspectDelegationToken(ctx, &admin.InspectDelegationTokenRequest{
			Token: "zzzz",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp.InvalidityReason, should.HavePrefix("can't unmarshal the envelope - proto"))
	})

	ftt.Run("Bad signature", t, func(t *ftt.Test) {
		env, _, _ := deserializeForTest(ctx, tok, rpc.Signer)
		env.Pkcs1Sha256Sig = []byte("lalala")
		blob, _ := proto.Marshal(env)
		tok := base64.RawURLEncoding.EncodeToString(blob)

		resp, err := rpc.InspectDelegationToken(ctx, &admin.InspectDelegationTokenRequest{
			Token: tok,
		})
		assert.Loosely(t, err, should.BeNil)

		resp.Envelope.Pkcs1Sha256Sig = nil
		resp.Envelope.SerializedSubtoken = nil
		assert.Loosely(t, resp, should.Match(&admin.InspectDelegationTokenResponse{
			Valid:            false,
			InvalidityReason: "bad signature - crypto/rsa: verification error",
			Signed:           false,
			NonExpired:       true,
			Envelope: &messages.DelegationToken{
				SignerId:     "user:service@example.com",
				SigningKeyId: signer.KeyNameForTest(),
			},
			Subtoken: original,
		}))
	})

	ftt.Run("Expired", t, func(t *ftt.Test) {
		tc.Add(2 * time.Hour)

		resp, err := rpc.InspectDelegationToken(ctx, &admin.InspectDelegationTokenRequest{
			Token: tok,
		})
		assert.Loosely(t, err, should.BeNil)

		resp.Envelope.Pkcs1Sha256Sig = nil
		resp.Envelope.SerializedSubtoken = nil
		assert.Loosely(t, resp, should.Match(&admin.InspectDelegationTokenResponse{
			Valid:            false,
			InvalidityReason: "expired",
			Signed:           true,
			NonExpired:       false,
			Envelope: &messages.DelegationToken{
				SignerId:     "user:service@example.com",
				SigningKeyId: signer.KeyNameForTest(),
			},
			Subtoken: original,
		}))
	})
}

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

package machine

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	tokenserverpb "go.chromium.org/luci/tokenserver/api"
)

func TestMachineTokenAuthMethod(t *testing.T) {
	t.Parallel()

	ftt.Run("with mock context", t, func(t *ftt.Test) {
		ctx := makeTestContext()
		log := logging.Get(ctx).(*memlogger.MemLogger)
		signer := signingtest.NewSigner(nil)
		method := MachineTokenAuthMethod{
			certsFetcher: func(ctx context.Context, email string) (*signing.PublicCertificates, error) {
				if email == "valid-signer@example.com" {
					return signer.Certificates(ctx)
				}
				return nil, fmt.Errorf("unknown signer")
			},
		}

		mint := func(tok *tokenserverpb.MachineTokenBody, sig []byte) string {
			body, _ := proto.Marshal(tok)
			keyID, validSig, _ := signer.SignBytes(ctx, body)
			if sig == nil {
				sig = validSig
			}
			envelope, _ := proto.Marshal(&tokenserverpb.MachineTokenEnvelope{
				TokenBody: body,
				KeyId:     keyID,
				RsaSha256: sig,
			})
			return base64.RawStdEncoding.EncodeToString(envelope)
		}

		call := func(tok string) (*auth.User, error) {
			r := authtest.NewFakeRequestMetadata()
			if tok != "" {
				r.FakeHeader.Set(MachineTokenHeader, tok)
			}
			u, _, err := method.Authenticate(ctx, r)
			return u, err
		}

		hasLog := func(msg string) bool {
			return log.HasFunc(func(m *memlogger.LogEntry) bool {
				return strings.Contains(m.Msg, msg)
			})
		}

		t.Run("valid token works", func(t *ftt.Test) {
			user, err := call(mint(&tokenserverpb.MachineTokenBody{
				MachineFqdn: "some-machine.location",
				CaId:        123,
				CertSn:      []byte{1, 2, 3},
				IssuedBy:    "valid-signer@example.com",
				IssuedAt:    uint64(clock.Now(ctx).Unix()),
				Lifetime:    3600,
			}, nil))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, user, should.Match(&auth.User{
				Identity: "bot:some-machine.location",
				Extra: &MachineTokenInfo{
					FQDN:   "some-machine.location",
					CA:     123,
					CertSN: []byte{1, 2, 3},
				},
			}))
		})

		t.Run("not header => not applicable", func(t *ftt.Test) {
			user, err := call("")
			assert.Loosely(t, user, should.BeNil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("not base64 envelope", func(t *ftt.Test) {
			_, err := call("not-a-valid-token")
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Failed to deserialize the token"), should.BeTrue)
		})

		t.Run("broken envelope", func(t *ftt.Test) {
			_, err := call("abcdef")
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Failed to deserialize the token"), should.BeTrue)
		})

		t.Run("broken body", func(t *ftt.Test) {
			envelope, _ := proto.Marshal(&tokenserverpb.MachineTokenEnvelope{
				TokenBody: []byte("bad body"),
				KeyId:     "123",
				RsaSha256: []byte("12345"),
			})
			tok := base64.RawStdEncoding.EncodeToString(envelope)
			_, err := call(tok)
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Failed to deserialize the token"), should.BeTrue)
		})

		t.Run("bad signer ID", func(t *ftt.Test) {
			_, err := call(mint(&tokenserverpb.MachineTokenBody{
				MachineFqdn: "some-machine.location",
				IssuedBy:    "not-a-email.com",
				IssuedAt:    uint64(clock.Now(ctx).Unix()),
				Lifetime:    3600,
			}, nil))
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Bad issued_by field"), should.BeTrue)
		})

		t.Run("unknown signer", func(t *ftt.Test) {
			_, err := call(mint(&tokenserverpb.MachineTokenBody{
				MachineFqdn: "some-machine.location",
				IssuedBy:    "unknown-signer@example.com",
				IssuedAt:    uint64(clock.Now(ctx).Unix()),
				Lifetime:    3600,
			}, nil))
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Unknown token issuer"), should.BeTrue)
		})

		t.Run("not yet valid", func(t *ftt.Test) {
			_, err := call(mint(&tokenserverpb.MachineTokenBody{
				MachineFqdn: "some-machine.location",
				IssuedBy:    "valid-signer@example.com",
				IssuedAt:    uint64(clock.Now(ctx).Unix()) + 60,
				Lifetime:    3600,
			}, nil))
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Token has expired or not yet valid"), should.BeTrue)
		})

		t.Run("expired", func(t *ftt.Test) {
			_, err := call(mint(&tokenserverpb.MachineTokenBody{
				MachineFqdn: "some-machine.location",
				IssuedBy:    "valid-signer@example.com",
				IssuedAt:    uint64(clock.Now(ctx).Unix()) - 3620,
				Lifetime:    3600,
			}, nil))
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Token has expired or not yet valid"), should.BeTrue)
		})

		t.Run("bad signature", func(t *ftt.Test) {
			_, err := call(mint(&tokenserverpb.MachineTokenBody{
				MachineFqdn: "some-machine.location",
				IssuedBy:    "valid-signer@example.com",
				IssuedAt:    uint64(clock.Now(ctx).Unix()),
				Lifetime:    3600,
			}, []byte("bad signature")))
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Bad signature"), should.BeTrue)
		})

		t.Run("bad machine_fqdn", func(t *ftt.Test) {
			_, err := call(mint(&tokenserverpb.MachineTokenBody{
				MachineFqdn: "not::valid::machine::id",
				IssuedBy:    "valid-signer@example.com",
				IssuedAt:    uint64(clock.Now(ctx).Unix()),
				Lifetime:    3600,
			}, nil))
			assert.Loosely(t, err, should.Equal(ErrBadToken))
			assert.Loosely(t, hasLog("Bad machine_fqdn"), should.BeTrue)
		})
	})
}

func makeTestContext() context.Context {
	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, time.Date(2015, time.February, 3, 4, 5, 6, 7, time.UTC))
	ctx = memlogger.Use(ctx)

	return authtest.NewFakeDB(
		authtest.MockMembership("user:valid-signer@example.com", TokenServersGroup),
	).Use(ctx)
}

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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func TestCheckToken(t *testing.T) {
	c := memlogger.Use(context.Background())
	c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
	minter := newFakeTokenMinter()

	defer func() {
		if t.Failed() {
			logging.Get(c).(*memlogger.MemLogger).Dump(os.Stderr)
		}
	}()

	ftt.Run("Basic use case", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		ident, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ident, should.Equal(identity.Identity("user:from@example.com")))
	})

	ftt.Run("Basic use case with group check", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "group:token-users"))

		groups := &fakeGroups{
			groups: map[string]string{
				"token-users": "user:to@example.com",
			},
		}

		// Pass.
		ident, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        groups,
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ident, should.Equal(identity.Identity("user:from@example.com")))

		// Fail.
		_, err = CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:NOT-to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        groups,
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrForbiddenDelegationToken))
	})

	ftt.Run("Not base64", t, func(t *ftt.Test) {
		_, err := CheckToken(c, CheckTokenParams{
			Token:                "(^*#%^&#%",
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrMalformedDelegationToken))
	})

	ftt.Run("Huge token is skipped", t, func(t *ftt.Test) {
		_, err := CheckToken(c, CheckTokenParams{
			Token:                strings.Repeat("aaaa", 10000),
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrMalformedDelegationToken))
	})

	ftt.Run("Untrusted signer", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		minter.signerID = "service:nah-i-renamed-myself"
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrUnsignedDelegationToken))
	})

	ftt.Run("Bad signature", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		// An offset in serialized token that points to Subtoken field. Replace one
		// byte there to "break" the signature.
		sigOffset := len(tok) - 10
		assert.Loosely(t, tok[sigOffset], should.NotEqual('A'))
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok[:sigOffset] + "A" + tok[sigOffset+1:],
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrUnsignedDelegationToken))
	})

	ftt.Run("Expired token", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))

		clock.Get(c).(testclock.TestClock).Add(2 * time.Hour)

		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrForbiddenDelegationToken))
	})

	ftt.Run("Wrong target service", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:NOT-a-service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrForbiddenDelegationToken))
	})

	ftt.Run("Wrong audience", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:NOT-to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrForbiddenDelegationToken))
	})

	ftt.Run("Unknown fields in envelope rejected", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		raw, err := base64.RawURLEncoding.DecodeString(tok)
		assert.Loosely(t, err, should.BeNil)

		// Append unknown field tag: field 99, length 4: [0x9a, 0x06, 0x04, 't', 'e', 's', 't']
		mutated := append(raw, 0x9a, 0x06, 0x04, 't', 'e', 's', 't')
		mutatedTok := base64.RawURLEncoding.EncodeToString(mutated)

		_, err = CheckToken(c, CheckTokenParams{
			Token:                mutatedTok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrMalformedDelegationToken))
	})

	ftt.Run("Non-strict base64 rejected", t, func(t *ftt.Test) {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		// Add trailing whitespace or invalid strict padding
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok + " ",
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrMalformedDelegationToken))
	})

	ftt.Run("Missing serialized_subtoken rejected", t, func(t *ftt.Test) {
		env := &messages.DelegationToken{
			SignerId:       "service:fake-signer",
			SigningKeyId:   "key",
			Pkcs1Sha256Sig: []byte("sig"),
		}
		raw, _ := proto.Marshal(env)
		tok := base64.RawURLEncoding.EncodeToString(raw)

		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		assert.Loosely(t, err, should.Equal(ErrMalformedDelegationToken))
	})

	ftt.Run("TokenFingerprint and EnvelopeFingerprint work", t, func(t *ftt.Test) {
		sub := subtoken(c, "user:from@example.com", "user:to@example.com")
		subBytes, err := proto.Marshal(sub)
		assert.Loosely(t, err, should.BeNil)
		digest := sha256.Sum256(subBytes)
		expectedFP := hex.EncodeToString(digest[:16])

		tok := minter.mintToken(c, sub)
		fp, err := TokenFingerprint(tok)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fp, should.Equal(expectedFP))

		// Unknown fields rejected by TokenFingerprint.
		raw, _ := base64.RawURLEncoding.DecodeString(tok)
		mutated := append(raw, 0x9a, 0x06, 0x04, 't', 'e', 's', 't')
		mutatedTok := base64.RawURLEncoding.EncodeToString(mutated)
		_, err = TokenFingerprint(mutatedTok)
		assert.Loosely(t, err, should.ErrLike("unknown fields"))

		// Non-strict base64 rejected by TokenFingerprint.
		_, err = TokenFingerprint(tok + "=")
		assert.Loosely(t, err, should.ErrLike("illegal base64"))
	})
}

// subtoken returns messages.Subtoken with some fields filled in.
func subtoken(ctx context.Context, delegatedID, audience string) *messages.Subtoken {
	return &messages.Subtoken{
		Kind:              messages.Subtoken_BEARER_DELEGATION_TOKEN,
		DelegatedIdentity: delegatedID,
		CreationTime:      clock.Now(ctx).Unix() - 300,
		ValidityDuration:  3600,
		Audience:          []string{audience},
		Services:          []string{"service:service-id"},
	}
}

// fakeTokenMinter knows how to generate tokens.
//
// It also implements CertificatesProvider protocol that is used when validating
// the tokens.
type fakeTokenMinter struct {
	signer   signing.Signer
	signerID string
}

func newFakeTokenMinter() *fakeTokenMinter {
	return &fakeTokenMinter{
		signer:   signingtest.NewSigner(nil),
		signerID: "service:fake-signer",
	}
}

func (f *fakeTokenMinter) GetCertificates(ctx context.Context, id identity.Identity) (*signing.PublicCertificates, error) {
	if string(id) != f.signerID {
		return nil, nil
	}
	return f.signer.Certificates(ctx)
}

func (f *fakeTokenMinter) mintToken(ctx context.Context, subtoken *messages.Subtoken) string {
	blob, err := proto.Marshal(subtoken)
	if err != nil {
		panic(err)
	}
	keyID, sig, err := f.signer.SignBytes(ctx, blob)
	if err != nil {
		panic(err)
	}
	tok, err := proto.Marshal(&messages.DelegationToken{
		SerializedSubtoken: blob,
		SignerId:           f.signerID,
		SigningKeyId:       keyID,
		Pkcs1Sha256Sig:     sig,
	})
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(tok)
}

// fakeGroups implements GroupsChecker.
type fakeGroups struct {
	groups map[string]string // if nil, IsMember always returns false
}

func (f *fakeGroups) IsMember(ctx context.Context, id identity.Identity, groups []string) (bool, error) {
	for _, group := range groups {
		if f.groups[group] == string(id) {
			return true, nil
		}
	}
	return false, nil
}

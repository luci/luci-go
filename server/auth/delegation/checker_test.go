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

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Basic use case", t, func() {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		ident, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		So(err, ShouldBeNil)
		So(ident, ShouldEqual, identity.Identity("user:from@example.com"))
	})

	Convey("Basic use case with group check", t, func() {
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
		So(err, ShouldBeNil)
		So(ident, ShouldEqual, identity.Identity("user:from@example.com"))

		// Fail.
		_, err = CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:NOT-to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        groups,
			OwnServiceIdentity:   "service:service-id",
		})
		So(err, ShouldEqual, ErrForbiddenDelegationToken)
	})

	Convey("Not base64", t, func() {
		_, err := CheckToken(c, CheckTokenParams{
			Token:                "(^*#%^&#%",
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		So(err, ShouldEqual, ErrMalformedDelegationToken)
	})

	Convey("Huge token is skipped", t, func() {
		_, err := CheckToken(c, CheckTokenParams{
			Token:                strings.Repeat("aaaa", 10000),
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		So(err, ShouldEqual, ErrMalformedDelegationToken)
	})

	Convey("Untrusted signer", t, func() {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		minter.signerID = "service:nah-i-renamed-myself"
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		So(err, ShouldEqual, ErrUnsignedDelegationToken)
	})

	Convey("Bad signature", t, func() {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		// An offset in serialized token that points to Subtoken field. Replace one
		// byte there to "break" the signature.
		sigOffset := len(tok) - 10
		So(tok[sigOffset], ShouldNotEqual, 'A')
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok[:sigOffset] + "A" + tok[sigOffset+1:],
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		So(err, ShouldEqual, ErrUnsignedDelegationToken)
	})

	Convey("Expired token", t, func() {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))

		clock.Get(c).(testclock.TestClock).Add(2 * time.Hour)

		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		So(err, ShouldEqual, ErrForbiddenDelegationToken)
	})

	Convey("Wrong target service", t, func() {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:NOT-a-service-id",
		})
		So(err, ShouldEqual, ErrForbiddenDelegationToken)
	})

	Convey("Wrong audience", t, func() {
		tok := minter.mintToken(c, subtoken(c, "user:from@example.com", "user:to@example.com"))
		_, err := CheckToken(c, CheckTokenParams{
			Token:                tok,
			PeerID:               "user:NOT-to@example.com",
			CertificatesProvider: minter,
			GroupsChecker:        &fakeGroups{},
			OwnServiceIdentity:   "service:service-id",
		})
		So(err, ShouldEqual, ErrForbiddenDelegationToken)
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

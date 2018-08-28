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

package serviceaccounts

import (
	"encoding/base64"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInspectOAuthTokenGrant(t *testing.T) {
	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
	signer := signingtest.NewSigner(nil)

	rpc := InspectOAuthTokenGrantRPC{
		Signer: signer,
		Rules: func(context.Context) (*Rules, error) {
			return loadConfig(ctx, `rules {
				name: "rule 1"
				service_account: "serviceaccount@robots.com"
				proxy: "user:proxy@example.com"
				end_user: "user:enduser@example.com"
				max_grant_validity_duration: 7200
			}`)
		},
	}

	original := &tokenserver.OAuthTokenGrantBody{
		TokenId:          123,
		ServiceAccount:   "serviceaccount@robots.com",
		Proxy:            "user:proxy@example.com",
		EndUser:          "user:enduser@example.com",
		IssuedAt:         google.NewTimestamp(clock.Now(ctx)),
		ValidityDuration: 3600,
	}

	matchingRule := &admin.ServiceAccountRule{
		Name:                     "rule 1",
		ServiceAccount:           []string{"serviceaccount@robots.com"},
		Proxy:                    []string{"user:proxy@example.com"},
		EndUser:                  []string{"user:enduser@example.com"},
		MaxGrantValidityDuration: 7200,
	}

	tok, _ := SignGrant(ctx, rpc.Signer, original)

	Convey("Happy path", t, func() {
		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: tok,
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResembleProto, &admin.InspectOAuthTokenGrantResponse{
			Valid:          true,
			Signed:         true,
			NonExpired:     true,
			SigningKeyId:   signer.KeyNameForTest(),
			TokenBody:      original,
			AllowedByRules: true,
			MatchingRule:   matchingRule,
		})
	})

	Convey("Not base64", t, func() {
		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: "@@@@@@@@@@@@@",
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResembleProto, &admin.InspectOAuthTokenGrantResponse{
			InvalidityReason: "not base64 - illegal base64 data at input byte 0",
		})
	})

	Convey("Not valid envelope proto", t, func() {
		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: "zzzz",
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResembleProto, &admin.InspectOAuthTokenGrantResponse{
			InvalidityReason: "can't unmarshal the envelope - proto: can't skip unknown wire type 7",
		})
	})

	Convey("Bad signature", t, func() {
		env, _, _ := deserializeForTest(ctx, tok, rpc.Signer)
		env.Pkcs1Sha256Sig = []byte("lalala")
		blob, _ := proto.Marshal(env)
		tok := base64.RawURLEncoding.EncodeToString(blob)

		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: tok,
		})
		So(err, ShouldBeNil)

		So(resp, ShouldResembleProto, &admin.InspectOAuthTokenGrantResponse{
			Valid:            false,
			Signed:           false,
			NonExpired:       true,
			InvalidityReason: "bad signature - crypto/rsa: verification error",
			SigningKeyId:     signer.KeyNameForTest(),
			TokenBody:        original,
			AllowedByRules:   true,
			MatchingRule:     matchingRule,
		})
	})

	Convey("Now allowed by rules", t, func() {
		another := *original
		another.ServiceAccount = "unknown@robots.com"
		tok, _ := SignGrant(ctx, rpc.Signer, &another)

		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: tok,
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResembleProto, &admin.InspectOAuthTokenGrantResponse{
			Valid:            false,
			Signed:           true,
			NonExpired:       true,
			InvalidityReason: "the service account is not specified in the rules (rev fake-revision)",
			SigningKeyId:     signer.KeyNameForTest(),
			TokenBody:        &another,
		})
	})

	Convey("Expired", t, func() {
		tc.Add(2 * time.Hour)

		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: tok,
		})
		So(err, ShouldBeNil)

		So(resp, ShouldResembleProto, &admin.InspectOAuthTokenGrantResponse{
			Valid:            false,
			Signed:           true,
			NonExpired:       false,
			InvalidityReason: "expired",
			SigningKeyId:     signer.KeyNameForTest(),
			TokenBody:        original,
			AllowedByRules:   true,
			MatchingRule:     matchingRule,
		})
	})
}

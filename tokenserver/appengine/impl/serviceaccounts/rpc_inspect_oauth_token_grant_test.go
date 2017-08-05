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
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/server/auth/signing/signingtest"

	"github.com/luci/luci-go/tokenserver/api"
	"github.com/luci/luci-go/tokenserver/api/admin/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInspectOAuthTokenGrant(t *testing.T) {
	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)

	rpc := InspectOAuthTokenGrantRPC{
		Signer: signingtest.NewSigner(0, nil),
	}

	original := &tokenserver.OAuthTokenGrantBody{
		TokenId:          123,
		ServiceAccount:   "serviceaccount@robots.com",
		Proxy:            "user:proxy@example.com",
		EndUser:          "user:enduser@example.com",
		IssuedAt:         google.NewTimestamp(clock.Now(ctx)),
		ValidityDuration: 3600,
	}

	tok, _ := SignGrant(ctx, rpc.Signer, original)

	Convey("Happy path", t, func() {
		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: tok,
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.InspectOAuthTokenGrantResponse{
			Valid:        true,
			Signed:       true,
			NonExpired:   true,
			SigningKeyId: "f9da5a0d0903bda58c6d664e3852a89c283d7fe9",
			TokenBody:    original,
		})
	})

	Convey("Not base64", t, func() {
		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: "@@@@@@@@@@@@@",
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.InspectOAuthTokenGrantResponse{
			InvalidityReason: "not base64 - illegal base64 data at input byte 0",
		})
	})

	Convey("Not valid envelope proto", t, func() {
		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: "zzzz",
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.InspectOAuthTokenGrantResponse{
			InvalidityReason: "can't unmarshal the envelope - proto: can't skip unknown wire type 7 for tokenserver.OAuthTokenGrantEnvelope",
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

		So(resp, ShouldResemble, &admin.InspectOAuthTokenGrantResponse{
			Valid:            false,
			Signed:           false,
			NonExpired:       true,
			InvalidityReason: "bad signature - crypto/rsa: verification error",
			SigningKeyId:     "f9da5a0d0903bda58c6d664e3852a89c283d7fe9",
			TokenBody:        original,
		})
	})

	Convey("Expired", t, func() {
		tc.Add(2 * time.Hour)

		resp, err := rpc.InspectOAuthTokenGrant(ctx, &admin.InspectOAuthTokenGrantRequest{
			Token: tok,
		})
		So(err, ShouldBeNil)

		So(resp, ShouldResemble, &admin.InspectOAuthTokenGrantResponse{
			Valid:            false,
			Signed:           true,
			NonExpired:       false,
			InvalidityReason: "expired",
			SigningKeyId:     "f9da5a0d0903bda58c6d664e3852a89c283d7fe9",
			TokenBody:        original,
		})
	})
}

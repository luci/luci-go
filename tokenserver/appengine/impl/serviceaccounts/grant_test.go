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

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/tokenserver/api"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSignGrant(t *testing.T) {
	Convey("Works", t, func() {
		ctx := context.Background()
		signer := signingtest.NewSigner(0, nil)

		original := &tokenserver.OAuthTokenGrantBody{
			TokenId:        123,
			ServiceAccount: "email@example.com",
			Proxy:          "user:someone@example.com",
			EndUser:        "user:someone-else@example.com",
		}

		tok, err := SignGrant(ctx, signer, original)
		So(err, ShouldBeNil)
		So(tok, ShouldEqual, `Ck4IexIRZW1haWxAZXhhbXBsZS5jb20aGHVzZXI6c29tZW9uZUB`+
			`leGFtcGxlLmNvbSIddXNlcjpzb21lb25lLWVsc2VAZXhhbXBsZS5jb20SKGY5ZGE1YTBkM`+
			`DkwM2JkYTU4YzZkNjY0ZTM4NTJhODljMjgzZDdmZTkaQIuW0EtCsdP3xNRgnQcWb5DkTvb`+
			`8Y6xwJLJAQ04PflFeCdBXBxvqVgHbGflYD9OZlNGhUeE40pFpGBPOt4KGxCI`)

		envelope, back, err := deserializeForTest(ctx, tok, signer)
		So(err, ShouldBeNil)
		So(back, ShouldResembleProto, original)
		So(envelope.KeyId, ShouldEqual, "f9da5a0d0903bda58c6d664e3852a89c283d7fe9")
	})
}

func deserializeForTest(c context.Context, tok string, signer signing.Signer) (*tokenserver.OAuthTokenGrantEnvelope, *tokenserver.OAuthTokenGrantBody, error) {
	blob, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return nil, nil, err
	}
	env := &tokenserver.OAuthTokenGrantEnvelope{}
	if err = proto.Unmarshal(blob, env); err != nil {
		return nil, nil, err
	}

	// See tokensigning.Signer. We prepend tokenSigningContext (+ \x00) before
	// a message to be signed.
	bytesToCheck := []byte(tokenSigningContext)
	bytesToCheck = append(bytesToCheck, 0)
	bytesToCheck = append(bytesToCheck, env.TokenBody...)

	certs, err := signer.Certificates(c)
	if err != nil {
		return nil, nil, err
	}
	if err = certs.CheckSignature(env.KeyId, bytesToCheck, env.Pkcs1Sha256Sig); err != nil {
		return nil, nil, err
	}

	body := &tokenserver.OAuthTokenGrantBody{}
	if err = proto.Unmarshal(env.TokenBody, body); err != nil {
		return nil, nil, err
	}
	return env, body, nil
}

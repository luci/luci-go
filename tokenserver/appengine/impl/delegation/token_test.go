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

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSignToken(t *testing.T) {
	Convey("Works", t, func() {
		ctx := context.Background()
		signer := signingtest.NewSigner(&signing.ServiceInfo{
			ServiceAccountName: "service@example.com",
		})

		original := &messages.Subtoken{
			DelegatedIdentity: "user:delegated@example.com",
			RequestorIdentity: "user:requestor@example.com",
			CreationTime:      1477624966,
			ValidityDuration:  3600,
			Audience:          []string{"*"},
			Services:          []string{"*"},
		}

		tok, err := SignToken(ctx, signer, original)
		So(err, ShouldBeNil)
		So(tok, ShouldHaveLength, 276)

		envelope, back, err := deserializeForTest(ctx, tok, signer)
		So(err, ShouldBeNil)
		So(back, ShouldResembleProto, original)

		envelope.Pkcs1Sha256Sig = nil
		envelope.SerializedSubtoken = nil
		So(envelope, ShouldResembleProto, &messages.DelegationToken{
			SignerId:     "user:service@example.com",
			SigningKeyId: signer.KeyNameForTest(),
		})
	})
}

func deserializeForTest(c context.Context, tok string, signer signing.Signer) (*messages.DelegationToken, *messages.Subtoken, error) {
	blob, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return nil, nil, err
	}
	env := &messages.DelegationToken{}
	if err = proto.Unmarshal(blob, env); err != nil {
		return nil, nil, err
	}
	certs, err := signer.Certificates(c)
	if err != nil {
		return nil, nil, err
	}
	if err = certs.CheckSignature(env.SigningKeyId, env.SerializedSubtoken, env.Pkcs1Sha256Sig); err != nil {
		return nil, nil, err
	}
	subtoken := &messages.Subtoken{}
	if err = proto.Unmarshal(env.SerializedSubtoken, subtoken); err != nil {
		return nil, nil, err
	}
	return env, subtoken, nil
}

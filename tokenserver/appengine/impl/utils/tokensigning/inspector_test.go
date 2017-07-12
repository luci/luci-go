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
	"encoding/base64"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/server/auth/signing"
	"github.com/luci/luci-go/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInspectToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)

	signer := signingtest.NewSigner(0, &signing.ServiceInfo{
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

	Convey("Happy path", t, func() {
		ins, err := inspector.InspectToken(ctx, good)
		So(err, ShouldBeNil)
		So(ins.Signed, ShouldBeTrue)
		So(ins.NonExpired, ShouldBeTrue)
		So(ins.InvalidityReason, ShouldEqual, "")
		So(ins.Envelope, ShouldHaveSameTypeAs, &messages.DelegationToken{})
		So(ins.Body, ShouldResemble, original)
	})

	Convey("Not base64", t, func() {
		ins, err := inspector.InspectToken(ctx, "@@@@@@@@@@@@@")
		So(err, ShouldBeNil)
		So(ins, ShouldResemble, &Inspection{
			InvalidityReason: "not base64 - illegal base64 data at input byte 0",
		})
	})

	Convey("Not valid envelope proto", t, func() {
		ins, err := inspector.InspectToken(ctx, "zzzz")
		So(err, ShouldBeNil)
		So(ins, ShouldResemble, &Inspection{
			InvalidityReason: "can't unmarshal the envelope - proto: can't skip unknown wire type 7 for messages.DelegationToken",
		})
	})

	Convey("Bad signature", t, func() {
		env, _, _ := deserializeForTest(ctx, good, signer)
		env.Pkcs1Sha256Sig = []byte("lalala")
		blob, _ := proto.Marshal(env)
		tok := base64.RawURLEncoding.EncodeToString(blob)

		ins, err := inspector.InspectToken(ctx, tok)
		So(err, ShouldBeNil)
		So(ins.Signed, ShouldBeFalse)
		So(ins.NonExpired, ShouldBeTrue)
		So(ins.InvalidityReason, ShouldEqual, "bad signature - crypto/rsa: verification error")
		So(ins.Envelope, ShouldHaveSameTypeAs, &messages.DelegationToken{})
		So(ins.Body, ShouldResemble, original) // recovered the token body nonetheless
	})

	Convey("Wrong SigningContext", t, func() {
		inspectorWithCtx := inspectorForTest(signer, "Some context")

		// Symptoms are same as in "Bad signature".
		ins, err := inspectorWithCtx.InspectToken(ctx, good)
		So(err, ShouldBeNil)
		So(ins.Signed, ShouldBeFalse)
		So(ins.NonExpired, ShouldBeTrue)
		So(ins.InvalidityReason, ShouldEqual, "bad signature - crypto/rsa: verification error")
		So(ins.Envelope, ShouldHaveSameTypeAs, &messages.DelegationToken{})
		So(ins.Body, ShouldResemble, original) // recovered the token body nonetheless
	})

	Convey("Expired", t, func() {
		tc.Add(2 * time.Hour)

		ins, err := inspector.InspectToken(ctx, good)
		So(err, ShouldBeNil)
		So(ins.Signed, ShouldBeTrue)
		So(ins.NonExpired, ShouldBeFalse)
		So(ins.InvalidityReason, ShouldEqual, "expired")
		So(ins.Envelope, ShouldHaveSameTypeAs, &messages.DelegationToken{})
		So(ins.Body, ShouldResemble, original)
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

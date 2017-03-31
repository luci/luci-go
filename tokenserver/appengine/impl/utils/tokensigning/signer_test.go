// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tokensigning

import (
	"encoding/base64"
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/server/auth/signing"
	"github.com/luci/luci-go/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSignToken(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := context.Background()
		signer := signingtest.NewSigner(0, &signing.ServiceInfo{
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

		tok, err := signForTest(ctx, signer, original)
		So(err, ShouldBeNil)
		So(tok, ShouldEqual, `Ehh1c2VyOnNlcnZpY2VAZXhhbXBsZS5jb20aKGY5ZGE1YTBkMDk`+
			`wM2JkYTU4YzZkNjY0ZTM4NTJhODljMjgzZDdmZTkiQG9oF6Zxi5yxVWdSjR_hKmxFqc51J`+
			`KfPeUeRmyUs3g79nOKLdg6b36WM9CB2BLhcumQSqv45e7rqNmkeFUfzCjsqRwoadXNlcjp`+
			`kZWxlZ2F0ZWRAZXhhbXBsZS5jb20QhonLwAUYkBwqASoyASo6GnVzZXI6cmVxdWVzdG9yQ`+
			`GV4YW1wbGUuY29t`)

		envelope, back, err := deserializeForTest(ctx, tok, signer)
		So(err, ShouldBeNil)
		So(back, ShouldResemble, original)

		envelope.Pkcs1Sha256Sig = nil
		envelope.SerializedSubtoken = nil
		So(envelope, ShouldResemble, &messages.DelegationToken{
			SignerId:     "user:service@example.com",
			SigningKeyId: "f9da5a0d0903bda58c6d664e3852a89c283d7fe9",
		})
	})
}

func signForTest(c context.Context, signer signing.Signer, tok *messages.Subtoken) (string, error) {
	s := Signer{
		Signer: signer,
		Wrap: func(t *Unwrapped) proto.Message {
			return &messages.DelegationToken{
				SignerId:           "user:" + t.SignerID,
				SigningKeyId:       t.KeyID,
				SerializedSubtoken: t.Body,
				Pkcs1Sha256Sig:     t.RsaSHA256Sig,
			}
		},
	}
	return s.SignToken(c, tok)
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

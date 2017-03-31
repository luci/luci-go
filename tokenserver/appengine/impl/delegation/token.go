// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/server/auth/signing"

	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/tokensigning"
)

// SignToken signs and serializes the delegation subtoken.
//
// It doesn't do any validation. Assumes the prepared subtoken is valid.
//
// Produces base64 URL-safe token or a transient error.
func SignToken(c context.Context, signer signing.Signer, subtok *messages.Subtoken) (string, error) {
	s := tokensigning.Signer{
		Signer: signer,
		Wrap: func(w *tokensigning.Unwrapped) proto.Message {
			return &messages.DelegationToken{
				SerializedSubtoken: w.Body,
				Pkcs1Sha256Sig:     w.RsaSHA256Sig,
				SignerId:           "user:" + w.SignerID,
				SigningKeyId:       w.KeyID,
			}
		},
	}
	return s.SignToken(c, subtok)
}

// InspectToken returns information about the delegation token.
//
// Inspection.Envelope is either nil or *messages.DelegationToken.
// Inspection.Body is either nil or *messages.Subtoken.
func InspectToken(c context.Context, certs tokensigning.CertificatesSupplier, tok string) (*tokensigning.Inspection, error) {
	i := tokensigning.Inspector{
		Certificates: certs,
		Envelope:     func() proto.Message { return &messages.DelegationToken{} },
		Body:         func() proto.Message { return &messages.Subtoken{} },
		Unwrap: func(e proto.Message) tokensigning.Unwrapped {
			env := e.(*messages.DelegationToken)
			return tokensigning.Unwrapped{
				Body:         env.SerializedSubtoken,
				RsaSHA256Sig: env.Pkcs1Sha256Sig,
				KeyID:        env.SigningKeyId,
			}
		},
		Lifespan: func(b proto.Message) tokensigning.Lifespan {
			body := b.(*messages.Subtoken)
			return tokensigning.Lifespan{
				NotBefore: time.Unix(body.CreationTime, 0),
				NotAfter:  time.Unix(body.CreationTime+int64(body.ValidityDuration), 0),
			}
		},
	}
	return i.InspectToken(c, tok)
}

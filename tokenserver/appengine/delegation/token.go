// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"encoding/base64"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/server/auth/signing"
)

// SignToken signs and serializes the delegation subtoken.
//
// It doesn't do any validation. Assumes the prepared subtoken is valid.
//
// Produces base64 URL-safe token or a transient error.
func SignToken(c context.Context, signer signing.Signer, subtok *messages.Subtoken) (string, error) {
	info, err := signer.ServiceInfo(c)
	if err != nil {
		return "", err
	}
	blob, err := proto.Marshal(subtok)
	if err != nil {
		return "", err
	}
	keyID, sig, err := signer.SignBytes(c, blob)
	if err != nil {
		return "", err
	}
	tok, err := proto.Marshal(&messages.DelegationToken{
		SignerId:           "user:" + info.ServiceAccountName,
		SigningKeyId:       keyID,
		Pkcs1Sha256Sig:     sig,
		SerializedSubtoken: blob,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(tok), nil
}
